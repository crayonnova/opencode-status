package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/nova/opencode-status/internal/poller"
	"github.com/nova/opencode-status/internal/storage"
)

type Server struct {
	Store  *storage.Store
	Window time.Duration
	srv    *http.Server
}

func New(store *storage.Store, addr string, window time.Duration) *Server {
	s := &Server{Store: store, Window: window}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.healthz)
	mux.HandleFunc("/api/snapshot", s.snapshot)
	mux.HandleFunc("/api/history", s.history)
	mux.HandleFunc("/api/stats", s.stats)
	s.srv = &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	return s
}

func (s *Server) Start() error {
	return s.srv.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	models, checks, err := s.Store.Stats()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"models":  models,
		"checks":  checks,
		"window":  s.Window.String(),
		"checked": time.Now().UTC(),
	})
}

func (s *Server) snapshot(w http.ResponseWriter, r *http.Request) {
	snap, err := poller.SnapshotFromStore(s.Store, s.Window)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (s *Server) history(w http.ResponseWriter, r *http.Request) {
	modelID := r.URL.Query().Get("model")
	if modelID == "" {
		writeErr(w, http.StatusBadRequest, errMissing("model query param required"))
		return
	}
	window := s.Window
	if v := r.URL.Query().Get("window"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			window = d
		}
	}
	buckets, _ := strconv.Atoi(r.URL.Query().Get("buckets"))
	if buckets <= 0 {
		buckets = 24
	}

	hist, err := s.Store.History(modelID, time.Now().Add(-window))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"model":   modelID,
		"window":  window.String(),
		"buckets": buckets,
		"points":  hist,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

type stringErr string

func (s stringErr) Error() string { return string(s) }
func errMissing(s string) error   { return stringErr(s) }
