package hub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func doRequest(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestRootServesServiceInfo(t *testing.T) {
	cfg := Config{Version: "1.2.3"}

	rec := doRequest(t, NewServer(cfg, NewStore(), nil).Handler(), "/")

	if rec.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want 200", rec.Code)
	}
	var got struct {
		Service string `json:"service"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	if got.Service != "kubeshark-hub" {
		t.Errorf("service = %q, want %q", got.Service, "kubeshark-hub")
	}
	if got.Version != cfg.Version {
		t.Errorf("version = %q, want %q", got.Version, cfg.Version)
	}
}

func TestEchoOK(t *testing.T) {
	rec := doRequest(t, NewServer(Config{}, NewStore(), nil).Handler(), "/echo")

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /echo status = %d, want 200", rec.Code)
	}
}

func TestRootAdvertisesHubURL(t *testing.T) {
	cfg := Config{Version: "1.2.3", HubFQDN: "kubeshark-hub.default.svc.cluster.local"}

	rec := doRequest(t, NewServer(cfg, NewStore(), nil).Handler(), "/")

	if rec.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want 200", rec.Code)
	}
	var got struct {
		HubURL string `json:"hubUrl"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	if got.HubURL != cfg.HubFQDN {
		t.Errorf("hubUrl = %q, want %q", got.HubURL, cfg.HubFQDN)
	}
}

func TestMetadataVersionAdvertisesHubURL(t *testing.T) {
	cfg := Config{Version: "1.2.3", HubFQDN: "kubeshark-hub.default.svc.cluster.local"}

	rec := doRequest(t, NewServer(cfg, NewStore(), nil).Handler(), "/metadata/version")

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /metadata/version status = %d, want 200", rec.Code)
	}
	var got struct {
		HubURL string `json:"hubUrl"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	if got.HubURL != cfg.HubFQDN {
		t.Errorf("hubUrl = %q, want %q", got.HubURL, cfg.HubFQDN)
	}
}

func TestUnknownRoute404(t *testing.T) {
	rec := doRequest(t, NewServer(Config{}, NewStore(), nil).Handler(), "/nope")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET /nope status = %d, want 404", rec.Code)
	}
}
