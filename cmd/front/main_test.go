package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestFrontProxiesAPIToHub(t *testing.T) {
	var gotPath string
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ver":"1.2.3"}`))
	}))
	defer hub.Close()

	hubBase, err := url.Parse(hub.URL)
	if err != nil {
		t.Fatalf("parse hub URL: %v", err)
	}
	rec := httptest.NewRecorder()
	newHandler(hubBase).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/metadata/version", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if gotPath != "/metadata/version" {
		t.Errorf("hub path = %q, want %q (prefix stripped)", gotPath, "/metadata/version")
	}
	if !strings.Contains(rec.Body.String(), "1.2.3") {
		t.Errorf("body = %q, want the hub response", rec.Body.String())
	}
}

func TestFrontServesSPAFallback(t *testing.T) {
	hubBase := &url.URL{Scheme: "http", Host: "127.0.0.1:1"}
	rec := httptest.NewRecorder()
	newHandler(hubBase).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/some/spa/route", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "kubeshark") {
		t.Errorf("body = %q, want the SPA shell", rec.Body.String())
	}
}