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
	cfg      Config
	store    *Store
	verifier TokenVerifier
}

// NewServer builds the hub server. verifier may be nil only when
// !cfg.AuthEnabled; with auth enabled a nil verifier fails closed (all
// requests 401).
func NewServer(cfg Config, store *Store, verifier TokenVerifier) *Server {
	return &Server{cfg: cfg, store: store, verifier: verifier}
}

// Handler returns the auth-wrapped mux; routes registered on the mux are
// covered by the auth middleware.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/{$}", s.handleRoot)
	mux.HandleFunc("/echo", s.handleEcho)
	mux.HandleFunc("POST /pods/worker", s.handlePostWorkerPod)
	mux.HandleFunc("POST /license", s.handlePostLicense)
	mux.HandleFunc("POST /pcaps/merge", s.handlePcapsMerge)
	mux.HandleFunc("GET /worker", s.handleGetWorker)
	mux.Handle("/debug/pprof/", http.DefaultServeMux)
	return s.requireAuth(mux)
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
