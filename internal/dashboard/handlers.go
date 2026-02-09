package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	stats := s.crawler.Stats()
	if err := s.tmpl.ExecuteTemplate(w, "index.html", stats); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	stats := s.crawler.Stats()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := s.crawler.Subscribe()

	// Send initial stats
	initialStats := s.crawler.Stats()
	if data, err := json.Marshal(initialStats); err == nil {
		fmt.Fprintf(w, "data: %s\n\n", data)
		w.(http.Flusher).Flush()
	}

	done := r.Context().Done()
	for {
		select {
		case <-done:
			s.crawler.Unsubscribe(ch)
			return
		case stats := <-ch:
			data, err := json.Marshal(stats)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			w.(http.Flusher).Flush()
		case <-time.After(30 * time.Second):
			// Keep-alive
			fmt.Fprintf(w, ": keepalive\n\n")
			w.(http.Flusher).Flush()
		}
	}
}

func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	seed := r.FormValue("seed")
	if seed == "" {
		http.Error(w, "Seed URL required", http.StatusBadRequest)
		return
	}

	if err := s.crawler.Start(seed); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.crawler.Stop()
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handlePause(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Toggle
	stats := s.crawler.Stats()
	if stats.State == "running" {
		s.crawler.Pause()
	} else if stats.State == "paused" {
		s.crawler.Resume()
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleQueue(w http.ResponseWriter, r *http.Request) {
	// Not implemented in this iteration, placeholder
	w.WriteHeader(http.StatusNotImplemented)
}
