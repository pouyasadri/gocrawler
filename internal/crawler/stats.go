package crawler

import (
	"time"
)

// CrawlerStats holds the current state and statistics of the crawler.
type CrawlerStats struct {
	State       string    `json:"state"`        // "idle", "running", "paused", "stopped"
	URLsFetched int64     `json:"urls_fetched"` // Atomic counter
	QueueSize   int       `json:"queue_size"`
	ErrorCount  int64     `json:"error_count"` // Atomic counter
	CurrentURLs []string  `json:"current_urls"`
	LastFetched []string  `json:"last_fetched"` // Recent URLs (last 10)
	StartedAt   time.Time `json:"started_at"`
	Duration    string    `json:"duration"` // Human readable duration
}
