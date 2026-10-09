package hub

import (
	"encoding/json"
	"errors"
	"net/http"
	_ "net/http/pprof" // registers /debug/pprof/* on http.DefaultServeMux

	corev1 "k8s.io/api/core/v1"
)

// maxBodyBytes caps request bodies so a client cannot stream unbounded data
// into the hub; oversized requests trip with 413.
const maxBodyBytes = 1 << 20

// decodeJSON bounds the request body, then decodes it; a body over the cap is
// rejected with 413 rather than the decoder's generic 400. Returns false when
// a response has already been written.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return false
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

func (s *Server) handlePostWorkerPod(w http.ResponseWriter, r *http.Request) {
	var pod corev1.Pod
	if !decodeJSON(w, r, &pod) {
		return
	}
	s.store.UpsertPod(&pod)
	writeOK(w)
}

func (s *Server) handlePostLicense(w http.ResponseWriter, r *http.Request) {
	var req struct {
		License string `json:"license"`
	}
	if !decodeJSON(w, r, &req) {
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
