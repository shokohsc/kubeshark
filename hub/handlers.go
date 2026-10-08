package hub

import (
	"encoding/json"
	"net/http"
	_ "net/http/pprof" // registers /debug/pprof/* on http.DefaultServeMux

	corev1 "k8s.io/api/core/v1"
)

func (s *Server) handlePostWorkerPod(w http.ResponseWriter, r *http.Request) {
	var pod corev1.Pod
	if err := json.NewDecoder(r.Body).Decode(&pod); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.store.UpsertPod(&pod)
	writeOK(w)
}

func (s *Server) handlePostLicense(w http.ResponseWriter, r *http.Request) {
	var req struct {
		License string `json:"license"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.store.SetLicense(req.License)
	writeOK(w)
}

// ponytail: /pcaps/merge is accept-and-discard — the feature is not implemented, and the CLI retries forever on any non-200.
func (s *Server) handlePcapsMerge(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleGetWorker(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.store.Pods())
}

func writeOK(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("{}"))
}
