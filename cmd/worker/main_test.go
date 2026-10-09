package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWorkerRegisterReportsPodToHub(t *testing.T) {
	var got struct {
		Path string
		Auth string
		Pod  workerPod
	}
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Path = r.URL.Path
		got.Auth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&got.Pod); err != nil {
			t.Errorf("decode pod: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer hub.Close()

	w := &worker{
		hubURL: hub.URL,
		token:  "tok123",
		pod: workerPod{
			Metadata: struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			}{Name: "kubeshark-worker-abc", Namespace: "default"},
			Spec: struct{ NodeName string `json:"nodeName"` }{NodeName: "node-1"},
		},
		client: hub.Client(),
	}

	if err := w.register(); err != nil {
		t.Fatalf("register: %v", err)
	}

	if got.Path != "/pods/worker" {
		t.Errorf("hub path = %q, want /pods/worker", got.Path)
	}
	if got.Auth != "Bearer tok123" {
		t.Errorf("auth header = %q, want bearer token", got.Auth)
	}
	if got.Pod.Metadata.Name != "kubeshark-worker-abc" {
		t.Errorf("pod name = %q, want kubeshark-worker-abc", got.Pod.Metadata.Name)
	}
	if got.Pod.Spec.NodeName != "node-1" {
		t.Errorf("node name = %q, want node-1", got.Pod.Spec.NodeName)
	}
}

func TestWorkerRegisterRejectsHubFailure(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer hub.Close()

	w := &worker{hubURL: hub.URL, client: hub.Client()}
	if err := w.register(); err == nil {
		t.Fatal("register accepted a non-200 response, want an error")
	}
}