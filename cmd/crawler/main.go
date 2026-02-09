package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pouyasadri/gocrawler/internal/crawler"
	"github.com/pouyasadri/gocrawler/internal/dashboard"
)

func main() {
	// Flags
	seed := flag.String("seed", "https://example.com", "Seed URL")
	concurrency := flag.Int("c", 4, "Concurrency")
	maxDepth := flag.Int("depth", 2, "Max depth")
	delay := flag.Duration("delay", 500*time.Millisecond, "Delay")
	dashboardAddr := flag.String("dashboard", "", "Dashboard address (e.g. :8080)")
	flag.Parse()

	// Config
	cfg := crawler.DefaultConfig()
	cfg.Concurrency = *concurrency
	cfg.MaxDepth = *maxDepth
	cfg.Delay = *delay

	// Crawler
	c := crawler.New(cfg)

	// Context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("Shutting down...")
		c.Stop()
		cancel()
	}()

	// Start Dashboard if requested
	if *dashboardAddr != "" {
		srv, err := dashboard.NewServer(c, *dashboardAddr)
		if err != nil {
			log.Fatalf("Failed to create dashboard: %v", err)
		}
		go func() {
			if err := srv.Run(ctx); err != nil {
				log.Printf("Dashboard error: %v", err)
			}
		}()
	}

	// Logic:
	// If dashboard is enabled, we wait for user to start via UI (unless we want to auto-start).
	// But CLI users might expect it to run if they didn't pass --dashboard?
	// If --dashboard is NOT passed, auto-start and wait.
	// If --dashboard IS passed, just wait for UI control (or auto-start if seed provided? the seed default is example.com, so safe to wait).

	if *dashboardAddr == "" {
		log.Printf("Starting crawler on %s...", *seed)
		if err := c.Start(*seed); err != nil {
			log.Fatalf("Failed to start crawler: %v", err)
		}
		c.Wait()
	} else {
		log.Printf("Dashboard enabled at http://localhost%s", *dashboardAddr)
		log.Println("Control the crawler via the dashboard.")
		// Block until context is done (signal)
		<-ctx.Done()
		// Wait for crawler if it was running
		c.Wait()
	}

	// Give some time for cleanup
	time.Sleep(500 * time.Millisecond)
}
