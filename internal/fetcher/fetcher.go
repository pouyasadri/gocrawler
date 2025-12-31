package fetcher

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Config controls fetcher behavior.
type Config struct {
	UserAgent     string
	Timeout       time.Duration // overall client timeout (optional)
	MaxBodySize   int64         //maximum bytes to read from response body (0 => default)
	MaxRedirects  int           // max redirect hops (0 => default)
	DrainOnClose  int64         // bytes to drain on Close to help connection reuse (0 => default 64KB)
	IdleConnLimit int           // MaxIdleConn'sPerHost (0 => default 10)
}

// Fetcher performs HTTP fetches with safe streaming.
type Fetcher struct {
	client *http.Client
	cfg    *Config
}

// New creates a configured Fetcher.
// cfg fields may be zero; sensible defaults are applied.
func New(cfg *Config) *Fetcher {
	// set defaults
	if cfg.MaxBodySize == 0 {
		cfg.MaxBodySize = 4 << 20 // 4 MB default limit
	}
	if cfg.DrainOnClose == 0 {
		cfg.DrainOnClose = 64 << 10 // 64 KB
	}
	if cfg.IdleConnLimit == 0 {
		cfg.IdleConnLimit = 10
	}
	// Transport tuned for reuse
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxConnsPerHost:     cfg.IdleConnLimit,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   cfg.Timeout,
	}

	// set redirect policy if MaxRedirects is set (otherwise default behavior)
	if cfg.MaxRedirects >= 0 {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if cfg.MaxRedirects == 0 {
				// follow default of net/http (which is 10), unless explicitly 0 => stop following
				return nil
			}
			if len(via) > cfg.MaxRedirects {
				// stop following and return error; caller can inspect last response if needed
				return http.ErrUseLastResponse
			}
			return nil
		}
	}

	return &Fetcher{
		client: client,
		cfg:    cfg,
	}
}

// drainReadCloser wraps the underlying response body.
// Reads are limited by an inner reader; Close will attempt to drain up to drainLimit bytes
// (helping connection reuse) and then close the underlying response body.
type drainReadCloser struct {
	inner      io.Reader // limited reader (not responsible for closing underlying)
	underlying io.ReadCloser
	drainLimit int64
	closed     bool
}

func (d *drainReadCloser) Read(p []byte) (n int, err error) {
	return d.inner.Read(p)
}

// Close drains up to drainLimit bytes from underlying (non-blocking beyond that due to LimitReader)
// then closes the underlying response body. Close is idempotent.
func (d *drainReadCloser) Close() error {
	if d.closed {
		return nil
	}
	d.closed = true
	// attempt to drain a small amount to increase chances of connection reuse
	_, _ = io.Copy(io.Discard, io.LimitReader(d.underlying, d.drainLimit))
	// finally close underlying body which releases resources/connections
	return d.underlying.Close()
}

// Fetch issues a GET request for the provided URL.
// It returns the response (so caller can inspect headers/status) and an io.ReadCloser for the body.
// The body reader is limited to MaxBodySize configured in Fetcher; caller MUST call body.Close() when done.
// If you want to stream directly to disk without holding body in memory, use FetchToFile.
func (f *Fetcher) Fetch(ctx context.Context, url string) (*http.Response, io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, err
	}
	if f.cfg.UserAgent != "" {
		req.Header.Set("User-Agent", f.cfg.UserAgent)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	// wrap body with a limit and a Close that drains up to drainLimit
	limited := io.LimitReader(resp.Body, f.cfg.MaxBodySize)
	rc := &drainReadCloser{
		inner:      limited,
		underlying: resp.Body,
		drainLimit: f.cfg.DrainOnClose,
	}
	return resp, rc, nil
}

// FetchToFile fetches the URL and streams the response body to the given file path.
// It writes up to MaxBodySize bytes. It returns the number of bytes written and the response status code.
// Caller receives an error if the response status is not 2xx (unless allowNon2xx is true).
// This helper closes the response body and the file.
func (f *Fetcher) FetchToFile(ctx context.Context, url string, filePath string, allowNon2xx bool) (int64, int, error) {
	resp, bodyRc, err := f.Fetch(ctx, url)
	if err != nil {
		return 0, 0, err
	}
	// ensure body is closed at the end
	defer func(bodyRc io.ReadCloser) {
		_ = bodyRc.Close()
	}(bodyRc)

	// prefer status >= 200 and < 300
	if !allowNon2xx && (resp.StatusCode < 200 || resp.StatusCode >= 300) {
		// consume and close body before returning
		_, _ = io.Copy(io.Discard, bodyRc)
		return 0, resp.StatusCode, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	fh, err := os.CreateTemp("", "fetch-*")
	if err != nil {
		// try to consume and close
		_, _ = io.Copy(io.Discard, bodyRc)
		return 0, resp.StatusCode, err
	}
	// attempt to remove temp file on error
	success := false
	defer func() {
		_ = fh.Close()
		if !success {
			_ = os.Remove(fh.Name())
		}
	}()

	n, err := io.Copy(fh, bodyRc)
	if err != nil {
		_ = fh.Close()
		return n, resp.StatusCode, err
	}
	_ = fh.Sync()
	_ = fh.Close()

	// ensure target directory exists
	destDir := filepath.Dir(filePath)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		_ = os.Remove(fh.Name())
		return n, resp.StatusCode, fmt.Errorf("creating target dir: %w", err)
	}

	// try atomic rename first
	if err := os.Rename(fh.Name(), filePath); err == nil {
		success = true
		return n, resp.StatusCode, nil
	}

	// fallback: copy temp -> destination with explicit error handling
	src, err := os.Open(fh.Name())
	if err != nil {
		_ = os.Remove(fh.Name())
		return n, resp.StatusCode, fmt.Errorf("opening temp file for copy: %w", err)
	}
	defer func() {
		_ = src.Close()
	}()

	dst, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		_ = os.Remove(fh.Name())
		return n, resp.StatusCode, fmt.Errorf("creating target file for copy: %w", err)
	}
	defer func() {
		_ = dst.Close()
	}()

	if _, err := io.Copy(dst, src); err != nil {
		_ = os.Remove(fh.Name())
		return n, resp.StatusCode, fmt.Errorf("copying temp to target file: %w", err)
	}
	_ = dst.Sync()
	_ = os.Remove(fh.Name())

	success = true
	return n, resp.StatusCode, nil
}

// ReadAllLimited reads whole body into memory but limited by Fetcher.MaxBodySize.
// Use only for small responses or debugging.
func (f *Fetcher) ReadAllLimited(ctx context.Context, url string) ([]byte, int, error) {
	resp, rc, err := f.Fetch(ctx, url)
	if err != nil {
		return nil, 0, err
	}
	defer func(rc io.ReadCloser) {
		err := rc.Close()
		if err != nil {
			fmt.Println(err)
		}
	}(rc)

	// io.ReadAll on limited reader is acceptable because reader enforces MaxBodySize
	b, err := io.ReadAll(rc)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return b, resp.StatusCode, nil
}

var ErrNon2xx = errors.New("non-2xx response")
