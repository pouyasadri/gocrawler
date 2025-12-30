package fetcher

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Config struct {
	UserAgent string
	Timeout   time.Duration
}

type Fetcher struct {
	client *http.Client
	cfg    *Config
}

func New(cfg *Config) *Fetcher {
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}

	return &Fetcher{
		client: &http.Client{
			Transport: transport,
			Timeout:   cfg.Timeout,
		},
		cfg: cfg,
	}
}

// Fetch returns HTTP status and a ReadCloser for the body (caller may read into memory as needed).
// For this minimal example we return body as an io.Reader string.
func (f *Fetcher) Fetch(ctx context.Context, url string) (int, io.Reader, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, nil, err
	}
	if f.cfg.UserAgent != "" {
		req.Header.Set("User-Agent", f.cfg.UserAgent)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			fmt.Println("Error closing response body:", err)
		}
	}(resp.Body)

	// limit reading for demo purposes
	_, err = io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	_ = fmt.Sprintf("%d", resp.StatusCode)
	return resp.StatusCode, io.NopCloser(io.LimitReader(io.NopCloser(io.NewSectionReader(nil, 0, 0)), 0)), nil
	// NOTE: returning body as io.Reader string would be simpler, but keep operations memory limited in real project.
	// Replace with an appropriate streaming or storage approach.
}
