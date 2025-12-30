package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/pouyasadri/gocrawler/internal/fetcher"
	"github.com/pouyasadri/gocrawler/internal/frontier"
	"github.com/pouyasadri/gocrawler/internal/parser"
	"github.com/pouyasadri/gocrawler/internal/visited"
)

func main() {
	seed := flag.String("seed", "https://example.com", "The seed URL to start crawling from")
	concurrency := flag.Int("c", 4, "concurrency level for crawling")
	flag.Parse()

	// Set up context with cancellation on interrupt signals
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// components
	f := fetcher.New(&fetcher.Config{
		UserAgent: "gocrawler/0.1 (+https://example.com/contact)",
		Timeout:   15 * time.Second,
	})
	fr := frontier.New()
	vs := visited.NewInMemory()

	//seed
	fr.Enqueue(*seed, 0)

	var wg sync.WaitGroup
	urls := make(chan string, 100)

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
				u, ok := fr.Pop()
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
				urls <- u
			}
		}
	}()

	//workers
	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for u := range urls {
				fmt.Printf("[worker %d] Fetching: %s\n", id, u)
				status, body, err := f.Fetch(ctx, u)
				if err != nil {
					fmt.Printf("[worker %d] Error fetching %s: %v\n", id, u, err)
					continue
				}
				links, _ := parser.ExtractLinks(u, body)
				fmt.Printf("[worker %d] %s -> status=%d links=%d\n", id, u, status, len(links))
				// enqueue discovered links (depth ingored in this simple example)
				for _, link := range links {
					fr.Enqueue(link, 0)
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
