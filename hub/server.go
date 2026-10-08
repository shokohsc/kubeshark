package hub

import (
	"encoding/json"
	"net/http"
)

type Config struct {
	AuthEnabled     bool
	ServiceAccounts []string
	License         string
	RingSize        int
	LogLevel        string
	Version         string
}

type Server struct {
	cfg   Config
	store *Store
}

func NewServer(cfg Config, store *Store) *Server {
	return &Server{cfg: cfg, store: store}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/{$}", s.handleRoot)
	mux.HandleFunc("/echo", s.handleEcho)
	return mux
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Service string `json:"service"`
		Version string `json:"version"`
	}{Service: "kubeshark-hub", Version: s.cfg.Version})
}

func (s *Server) handleEcho(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
