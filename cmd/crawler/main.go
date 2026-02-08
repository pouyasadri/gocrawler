package main

import (
	"bytes"
	"context"
	"encoding/json"
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
	"github.com/pouyasadri/gocrawler/internal/robotstxt"
	"github.com/pouyasadri/gocrawler/internal/visited"
)

func main() {
	seed := flag.String("seed", "https://example.com", "The seed URL to start crawling from")
	concurrency := flag.Int("c", 4, "concurrency level for crawling")
	maxDepth := flag.Int("depth", 2, "max crawling depth")
	delay := flag.Duration("delay", 500*time.Millisecond, "minimum delay between requests (global rate limit)")
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

	// 1. Fetch robots.txt
	robotsURL := fmt.Sprintf("%s://%s/robots.txt", seedURL.Scheme, seedURL.Host)
	fmt.Printf("Fetching robots.txt from %s...\n", robotsURL)
	var robotsChecker *robotstxt.RobotsChecker

	// Try to fetch robots.txt, if it fails, assume allowed (standard behavior on 404/error)
	rbCtx, rbCancel := context.WithTimeout(ctx, 10*time.Second)
	defer rbCancel()

	rbBytes, rbStatus, rbErr := f.ReadAllLimited(rbCtx, robotsURL)
	if rbErr != nil || rbStatus != 200 {
		fmt.Printf("Warning: could not fetch robots.txt (status=%d, err=%v). Assuming all allowed.\n", rbStatus, rbErr)
		// nil checker means everything allowed
	} else {
		fmt.Println("robots.txt fetched successfully. Parsing...")
		robotsChecker = robotstxt.New(bytes.NewReader(rbBytes), f.Config().UserAgent) // Accessing config needs care
	}

	//seed
	fr.Enqueue(*seed, 0)

	var wg sync.WaitGroup
	type crawlJob struct {
		url   string
		depth int
	}
	urls := make(chan crawlJob, 100)

	// Rate limiter: a token bucket with capacity 1, refilled every *delay
	rateLimiter := make(chan struct{}, 1)
	go func() {
		ticker := time.NewTicker(*delay)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				select {
				case rateLimiter <- struct{}{}:
					// Token added
				default:
					// Channel full, skip (burst already available)
				}
			}
		}
	}()
	// Seed one token immediately so first request doesn't wait
	rateLimiter <- struct{}{}

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

				// Wait for rate limiter token
				select {
				case <-ctx.Done():
					return
				case <-rateLimiter:
					// Got token, proceed
				}

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

				// Parse the page for links and metadata
				pageData, _ := parser.ParsePage(u, file)
				file.Close() // Ensure file is closed after parsing

				// Save metadata to JSON
				jsonPath := strings.TrimSuffix(outPath, ".html") + ".json"
				jsonData, _ := json.MarshalIndent(pageData, "", "  ")
				_ = os.WriteFile(jsonPath, jsonData, 0644)

				links := pageData.Links
				fmt.Printf("[worker %d] %s -> status=%d links=%d title=%q\n", id, u, status, len(links), pageData.Title)

				// If we haven't reached max depth, enqueue discovered links
				if depth < *maxDepth {
					for _, link := range links {
						// Domain restriction check
						if parsedLink, err := url.Parse(link); err == nil {
							// For simplicity, check if hostname matches exactly or is subdomain (optional)
							// Here we just check exact hostname match as "Domain Restriction"
							if parsedLink.Hostname() == allowHost {
								// Robots.txt check
								if robotsChecker != nil && !robotsChecker.Allowed(parsedLink.Path) {
									// Skip disallowed path
									continue
								}
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
