package dashboard

import "net/http"

func (s *Server) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /", s.handleIndex)
	mux.HandleFunc("GET /api/stats", s.handleStats)
	mux.HandleFunc("GET /api/events", s.handleSSE)
	mux.HandleFunc("POST /api/start", s.handleStart)
	mux.HandleFunc("POST /api/stop", s.handleStop)
	mux.HandleFunc("POST /api/pause", s.handlePause)

	// Static files
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(content))))
}
