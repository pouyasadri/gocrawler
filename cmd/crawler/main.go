package main

import (
	"context"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pouyasadri/gocrawler/internal/fetcher"
	"github.com/pouyasadri/gocrawler/internal/frontier"
	"github.com/pouyasadri/gocrawler/internal/parser"
	"github.com/pouyasadri/gocrawler/internal/visited"
)

func main() {
	seed := flag.String("seed", "https://example.com", "The seed URL to start crawling from")
	concurrency := flag.Int("c", 4, "concurrency level for crawling")
	maxDepth := flag.Int("depth", 2, "max crawling depth")
	flag.Parse()

	// Set up context with cancellation on interrupt signals
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// components
	f := fetcher.New(&fetcher.Config{
		UserAgent:   "gocrawler/0.1 (+https://example.com/contact)",
		Timeout:     20 * time.Second,
		MaxBodySize: 5 << 20, // 5 MB
	})

	fr := frontier.New()
	vs := visited.NewInMemory()

	// Parse seed to get allowed host
	seedURL, err := url.Parse(*seed)
	if err != nil {
		fmt.Printf("Error parsing seed URL: %v\n", err)
		os.Exit(1)
	}
	allowHost := seedURL.Hostname()

	//seed
	fr.Enqueue(*seed, 0)

	var wg sync.WaitGroup
	type crawlJob struct {
		url   string
		depth int
	}
	urls := make(chan crawlJob, 100)

	//producer: pop from frontier and send to urls channel
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				close(urls)
				return
			default:
				u, d, ok := fr.Pop()
				if !ok {
					// nothing queued, small sleep to avoid busy loop
					time.Sleep(200 * time.Millisecond)
					continue
				}
				//dedup
				if vs.Seen(u) {
					continue
				}
				vs.Mark(u)
				urls <- crawlJob{url: u, depth: d}
			}
		}
	}()

	//workers
	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for job := range urls {
				u := job.url
				depth := job.depth
				fmt.Printf("[worker %d] Fetching (depth=%d): %s\n", id, depth, u)

				// derive host directory name from URL
				hostDir := "unknown"
				if parsed, err := url.Parse(u); err == nil {
					hostName := parsed.Hostname()
					hostName = strings.TrimPrefix(hostName, "www.")
					if hostName != "" {
						// sanitize any remaining path/port chars just in case
						hostName = strings.ReplaceAll(hostName, ":", "_")
						hostName = strings.ReplaceAll(hostName, "/", "_")
						hostDir = hostName
					}
				}

				outDir := filepath.Join("output", hostDir)
				if err := os.MkdirAll(outDir, 0o755); err != nil {
					fmt.Printf("[worker %d] Error creating dir %s: %v\n", id, outDir, err)
					continue
				}

				outPath := filepath.Join(outDir, fmt.Sprintf("output_worker%d_%d.html", id, time.Now().UnixNano()))
				n, status, err := f.FetchToFile(ctx, u, outPath, false)

				if err != nil {
					fmt.Printf("[worker %d] Error fetching %s: %v\n", id, u, err)
					continue
				}
				fmt.Printf("wrote %d bytes to %s (status=%d)\n", n, outPath, status)

				// Open the file we just wrote to parse it
				file, err := os.Open(outPath)
				if err != nil {
					fmt.Printf("[worker %d] Error opening file %s for parsing: %v\n", id, outPath, err)
					continue
				}

				links, _ := parser.ExtractLinks(u, file)
				file.Close() // Ensure file is closed after parsing
				fmt.Printf("[worker %d] %s -> status=%d links=%d\n", id, u, status, len(links))

				// If we haven't reached max depth, enqueue discovered links
				if depth < *maxDepth {
					for _, link := range links {
						// Domain restriction check
						if parsedLink, err := url.Parse(link); err == nil {
							// For simplicity, check if hostname matches exactly or is subdomain (optional)
							// Here we just check exact hostname match as "Domain Restriction"
							if parsedLink.Hostname() == allowHost {
								fr.Enqueue(link, depth+1)
							}
						}
					}
				}
			}
		}(i)
	}

	// wait for cancellation
	<-ctx.Done()
	fmt.Println("Shutting down, waiting for workers to finish...")
	// closing urls channel will let workers finish
	// in this simple design the producer closes urls on ctx Done
	wg.Wait()
	fmt.Println("done")
}
