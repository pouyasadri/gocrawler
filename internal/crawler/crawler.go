package crawler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pouyasadri/gocrawler/internal/fetcher"
	"github.com/pouyasadri/gocrawler/internal/frontier"
	"github.com/pouyasadri/gocrawler/internal/parser"
	"github.com/pouyasadri/gocrawler/internal/robotstxt"
	"github.com/pouyasadri/gocrawler/internal/visited"
)

// Config configures the crawler.
type Config struct {
	Concurrency   int
	MaxDepth      int
	Delay         time.Duration
	OutputDir     string
	UserAgent     string
	FetchTimeout  time.Duration
	MaxBodySize   int64
	IdleConnLimit int
}

// DefaultConfig returns a default configuration.
func DefaultConfig() *Config {
	return &Config{
		Concurrency:   4,
		MaxDepth:      2,
		Delay:         500 * time.Millisecond,
		OutputDir:     "output",
		UserAgent:     "gocrawler/0.1 (+https://example.com/contact)",
		FetchTimeout:  20 * time.Second,
		MaxBodySize:   5 << 20, // 5 MB
		IdleConnLimit: 10,
	}
}

// Crawler manages the crawling process.
type Crawler struct {
	cfg      *Config
	fetcher  *fetcher.Fetcher
	frontier *frontier.Frontier
	visited  *visited.InMemory

	// state
	mu        sync.RWMutex
	state     string // "idle", "running", "paused", "stopped"
	startedAt time.Time
	stats     CrawlerStats

	// control
	ctx        context.Context
	cancel     context.CancelFunc
	pauseCh    chan struct{}
	resumeCh   chan struct{}
	stopCh     chan struct{}
	activeJobs sync.WaitGroup
	crawlerWg  sync.WaitGroup // Waits for the main loop and workers

	// observability
	subscribersMu sync.Mutex
	subscribers   []chan CrawlerStats
}

// New creates a new Crawler instance.
func New(cfg *Config) *Crawler {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	// ensure output dir exists
	_ = os.MkdirAll(cfg.OutputDir, 0755)

	f := fetcher.New(&fetcher.Config{
		UserAgent:     cfg.UserAgent,
		Timeout:       cfg.FetchTimeout,
		MaxBodySize:   cfg.MaxBodySize,
		IdleConnLimit: cfg.IdleConnLimit,
	})

	return &Crawler{
		cfg:      cfg,
		fetcher:  f,
		frontier: frontier.New(),
		visited:  visited.NewInMemory(),
		state:    "idle",
		pauseCh:  make(chan struct{}),
		resumeCh: make(chan struct{}),
		stopCh:   make(chan struct{}),
		stats: CrawlerStats{
			State: "idle",
		},
	}
}

// Start begins the crawling process starting from the seed URL.
// It runs asynchronously. Use Wait() to block until completion if needed.
func (c *Crawler) Start(seed string) error {
	c.mu.Lock()
	if c.state == "running" || c.state == "paused" {
		c.mu.Unlock()
		return fmt.Errorf("crawler is already running or paused")
	}
	c.state = "running"
	c.startedAt = time.Now()
	// Reset stats for new run
	c.stats = CrawlerStats{
		State:       "running",
		StartedAt:   c.startedAt,
		CurrentURLs: []string{},
		LastFetched: []string{},
	}
	c.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	c.ctx = ctx
	c.cancel = cancel

	// Seed the frontier
	c.frontier.Enqueue(seed, 0)
	c.visited.Mark(seed)

	c.crawlerWg.Add(1)
	go c.run(seed)

	return nil
}

// Stop gracefully stops the crawler.
func (c *Crawler) Stop() {
	c.mu.Lock()
	if c.state == "stopped" || c.state == "idle" {
		c.mu.Unlock()
		return
	}
	c.state = "stopped"
	c.mu.Unlock()

	if c.cancel != nil {
		c.cancel()
	}
	c.broadcastStats()
}

// Pause pauses the crawler workers.
func (c *Crawler) Pause() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state == "running" {
		c.state = "paused"
		// Logic to signal pause is handled in the worker loop by checking state
		c.broadcastStats()
	}
}

// Resume resumes the crawler workers.
func (c *Crawler) Resume() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state == "paused" {
		c.state = "running"
		c.broadcastStats()
	}
}

// Stats returns the current statistics.
func (c *Crawler) Stats() CrawlerStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Create a copy to avoid races on slices
	s := c.stats
	s.QueueSize = c.frontier.Size()
	s.Duration = time.Since(c.startedAt).Round(time.Second).String()
	s.State = c.state

	// Copy slices
	current := make([]string, len(c.stats.CurrentURLs))
	copy(current, c.stats.CurrentURLs)
	s.CurrentURLs = current

	last := make([]string, len(c.stats.LastFetched))
	copy(last, c.stats.LastFetched)
	s.LastFetched = last

	return s
}

// Subscribe returns a channel that receives stats updates.
func (c *Crawler) Subscribe() chan CrawlerStats {
	c.subscribersMu.Lock()
	defer c.subscribersMu.Unlock()
	ch := make(chan CrawlerStats, 10)
	c.subscribers = append(c.subscribers, ch)
	return ch
}

// Unsubscribe removes a subscriber channel.
func (c *Crawler) Unsubscribe(ch chan CrawlerStats) {
	c.subscribersMu.Lock()
	defer c.subscribersMu.Unlock()

	for i, subscriber := range c.subscribers {
		if subscriber == ch {
			c.subscribers = append(c.subscribers[:i], c.subscribers[i+1:]...)
			close(subscriber)
			break
		}
	}
}

func (c *Crawler) broadcastStats() {
	stats := c.Stats()
	c.subscribersMu.Lock()
	defer c.subscribersMu.Unlock()

	for _, ch := range c.subscribers {
		select {
		case ch <- stats:
		default:
			// Skip if channel is full
		}
	}
}

func (c *Crawler) run(seed string) {
	defer c.crawlerWg.Done()
	defer func() {
		c.mu.Lock()
		c.state = "idle"
		c.mu.Unlock()
		c.broadcastStats()
	}()

	// Parse seed to get allowed host
	seedURL, err := url.Parse(seed)
	if err != nil {
		fmt.Printf("Error parsing seed URL: %v\n", err)
		return
	}
	allowHost := seedURL.Hostname()

	// Fetch robots.txt
	var robotsChecker *robotstxt.RobotsChecker
	robotsURL := fmt.Sprintf("%s://%s/robots.txt", seedURL.Scheme, seedURL.Host)
	rbCtx, rbCancel := context.WithTimeout(c.ctx, 10*time.Second)
	rbBytes, rbStatus, rbErr := c.fetcher.ReadAllLimited(rbCtx, robotsURL)
	rbCancel()

	if rbErr == nil && rbStatus == 200 {
		robotsChecker = robotstxt.New(bytes.NewReader(rbBytes), c.cfg.UserAgent)
	}

	type crawlJob struct {
		url   string
		depth int
	}
	urls := make(chan crawlJob, 100)

	// Producer
	c.activeJobs.Add(1)
	go func() {
		defer c.activeJobs.Done()
		defer close(urls)

		for {
			select {
			case <-c.ctx.Done():
				return
			default:
				// Pause check
				c.mu.RLock()
				paused := c.state == "paused"
				c.mu.RUnlock()
				if paused {
					time.Sleep(500 * time.Millisecond)
					continue
				}

				u, d, ok := c.frontier.Pop()
				if !ok {
					// Check if we are done: no active jobs and empty frontier
					// For simplicity in this loop, just sleep.
					// Real termination detection is harder with concurrent workers.
					// We'll rely on manual stop or exhaustion.
					time.Sleep(200 * time.Millisecond)
					// Verify if we should exit (if queue is empty and all workers are idle)
					// requires more complex coordination. Keeping it simple.
					continue
				}

				// Dedup check was already done on enqueue mostly, but check again just in case
				// Actually main.go checked on Pop.
				if c.visited.Seen(u) {
					continue
				}
				c.visited.Mark(u)

				select {
				case urls <- crawlJob{url: u, depth: d}:
				case <-c.ctx.Done():
					return
				}
			}
		}
	}()

	// Workers
	// Rate limiter
	rateLimiter := time.NewTicker(c.cfg.Delay)
	defer rateLimiter.Stop()

	for i := 0; i < c.cfg.Concurrency; i++ {
		c.activeJobs.Add(1)
		go func(id int) {
			defer c.activeJobs.Done()
			for job := range urls {
				// Wait for rate limit
				select {
				case <-c.ctx.Done():
					return
				case <-rateLimiter.C:
					// proceed
				}

				c.processURL(c.ctx, id, job.url, job.depth, allowHost, robotsChecker)
			}
		}(i)
	}

	// Stats ticker
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	go func() {
		for {
			select {
			case <-c.ctx.Done():
				return
			case <-ticker.C:
				c.broadcastStats()
			}
		}
	}()

	// Wait for context cancellation
	<-c.ctx.Done()
	c.activeJobs.Wait()
}

func (c *Crawler) processURL(ctx context.Context, workerID int, u string, depth int, allowHost string, robots *robotstxt.RobotsChecker) {
	// Update stats: current URL
	c.mu.Lock()
	c.stats.CurrentURLs = append(c.stats.CurrentURLs, u)
	c.mu.Unlock()
	c.broadcastStats()

	defer func() {
		// Remove from current URLs
		c.mu.Lock()
		for i, url := range c.stats.CurrentURLs {
			if url == u {
				c.stats.CurrentURLs = append(c.stats.CurrentURLs[:i], c.stats.CurrentURLs[i+1:]...)
				break
			}
		}
		c.mu.Unlock()
		c.broadcastStats()
	}()

	// Output path setup
	hostDir := "unknown"
	if parsed, err := url.Parse(u); err == nil {
		hostName := parsed.Hostname()
		hostName = strings.TrimPrefix(hostName, "www.")
		if hostName != "" {
			hostName = strings.ReplaceAll(hostName, ":", "_")
			hostName = strings.ReplaceAll(hostName, "/", "_")
			hostDir = hostName
		}
	}
	outDir := filepath.Join(c.cfg.OutputDir, hostDir)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		atomic.AddInt64(&c.stats.ErrorCount, 1)
		return
	}
	outPath := filepath.Join(outDir, fmt.Sprintf("output_worker%d_%d.html", workerID, time.Now().UnixNano()))

	// Fetch
	_, status, err := c.fetcher.FetchToFile(ctx, u, outPath, false)
	if err != nil {
		atomic.AddInt64(&c.stats.ErrorCount, 1)
		return
	}
	atomic.AddInt64(&c.stats.URLsFetched, 1)

	// Update last fetched
	c.mu.Lock()
	c.stats.LastFetched = append([]string{u}, c.stats.LastFetched...)
	if len(c.stats.LastFetched) > 10 {
		c.stats.LastFetched = c.stats.LastFetched[:10]
	}
	c.mu.Unlock()

	// Parse
	file, err := os.Open(outPath)
	if err != nil {
		return
	}
	pageData, _ := parser.ParsePage(u, file)
	file.Close()

	if pageData != nil {
		// Save JSON
		jsonPath := strings.TrimSuffix(outPath, ".html") + ".json"
		jsonData, _ := json.MarshalIndent(pageData, "", "  ")
		_ = os.WriteFile(jsonPath, jsonData, 0644)

		fmt.Printf("[worker %d] %s -> status=%d links=%d title=%q\n", workerID, u, status, len(pageData.Links), pageData.Title)

		// Enqueue links
		if depth < c.cfg.MaxDepth {
			for _, link := range pageData.Links {
				if parsedLink, err := url.Parse(link); err == nil {
					if parsedLink.Hostname() == allowHost {
						if robots != nil && !robots.Allowed(parsedLink.Path) {
							continue
						}
						// Avoid enqueuing if already seen (optimization)
						if !c.visited.Seen(link) {
							c.frontier.Enqueue(link, depth+1)
						}
					}
				}
			}
		}
	}
}

// Wait blocks until the crawler finishes.
func (c *Crawler) Wait() {
	c.crawlerWg.Wait()
}
