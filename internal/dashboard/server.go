package dashboard

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"github.com/pouyasadri/gocrawler/internal/crawler"
)

//go:embed static templates
var content embed.FS

// Server represents the dashboard HTTP server.
type Server struct {
	crawler *crawler.Crawler
	addr    string
	tmpl    *template.Template
}

// NewServer creates a new dashboard server.
func NewServer(c *crawler.Crawler, addr string) (*Server, error) {
	tmpl, err := template.ParseFS(content, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parsing templates: %w", err)
	}

	return &Server{
		crawler: c,
		addr:    addr,
		tmpl:    tmpl,
	}, nil
}

// Run starts the HTTP server.
func (s *Server) Run(ctx context.Context) error {
	mux := http.NewServeMux()
	s.registerRoutes(mux)

	srv := &http.Server{
		Addr:    s.addr,
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	fmt.Printf("Dashboard running at http://localhost%s\n", s.addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}
