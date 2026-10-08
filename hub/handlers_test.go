package hub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func doPost(t *testing.T, h http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
	return rec
}

func TestPostWorkerPodStores(t *testing.T) {
	store := NewStore()
	h := NewServer(Config{}, store, nil).Handler()

	rec := doPost(t, h, "/pods/worker", `{"metadata":{"name":"wk-0","namespace":"kubeshark"}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /pods/worker status = %d, want 200", rec.Code)
	}

	rec = doRequest(t, h, "/worker")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /worker status = %d, want 200", rec.Code)
	}
	var pods []*corev1.Pod
	if err := json.Unmarshal(rec.Body.Bytes(), &pods); err != nil {
		t.Fatalf("decode GET /worker body %q: %v", rec.Body.String(), err)
	}
	found := false
	for _, p := range pods {
		if p.Name == "wk-0" && p.Namespace == "kubeshark" {
			found = true
		}
	}
	if !found {
		t.Errorf("GET /worker = %v, want pod kubeshark/wk-0", rec.Body.String())
	}
}

func TestPostWorkerPodInvalidJSON(t *testing.T) {
	h := NewServer(Config{}, NewStore(), nil).Handler()

	rec := doPost(t, h, "/pods/worker", `{"metadata":`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST /pods/worker invalid JSON status = %d, want 400", rec.Code)
	}
}

func TestPostLicenseStores(t *testing.T) {
	store := NewStore()
	h := NewServer(Config{}, store, nil).Handler()

	rec := doPost(t, h, "/license", `{"license":"KEY-1"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /license status = %d, want 200", rec.Code)
	}
	if got := store.License(); got != "KEY-1" {
		t.Errorf("store.License() = %q, want %q", got, "KEY-1")
	}

	rec = doPost(t, h, "/license", `{"license":"KEY-2"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("second POST /license status = %d, want 200", rec.Code)
	}
	if got := store.License(); got != "KEY-2" {
		t.Errorf("store.License() after overwrite = %q, want %q", got, "KEY-2")
	}
}

func TestPcapsMergeAlwaysOK(t *testing.T) {
	h := NewServer(Config{}, NewStore(), nil).Handler()

	rec := doPost(t, h, "/pcaps/merge", `{"query":""}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /pcaps/merge status = %d, want 200", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("POST /pcaps/merge body = %q, want empty", rec.Body.String())
	}

	rec = doPost(t, h, "/pcaps/merge", `{}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /pcaps/merge {} status = %d, want 200", rec.Code)
	}
}

func TestPprofIndex(t *testing.T) {
	h := NewServer(Config{}, NewStore(), nil).Handler()

	rec := doRequest(t, h, "/debug/pprof/")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /debug/pprof/ status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "profile") {
		t.Errorf("GET /debug/pprof/ body missing %q: %q", "profile", rec.Body.String())
	}
}
