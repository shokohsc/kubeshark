package hub

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestNoCommunityLimits(t *testing.T) {
	store := NewStore()
	h := NewServer(Config{Version: "test"}, store, nil).Handler()

	// (a) POST 5000 distinct v1.Pods across 300 distinct nodes → every POST 200
	nodes := 300
	podsTotal := 5000
	for i := 0; i < podsTotal; i++ {
		name := "pod-" + strconv.Itoa(i)
		nodeName := "node-" + strconv.Itoa(i%nodes)
		ns := "default"
		podJSON := `{"metadata":{"name":"` + name + `","namespace":"` + ns + `"},"spec":{"nodeName":"` + nodeName + `"}}`
		rec := doPost(t, h, "/pods/worker", podJSON)
		if rec.Code != http.StatusOK {
			t.Fatalf("POST /pods/worker %d status = %d, want 200", i, rec.Code)
		}
	}

	// GET /health/hub → 200, nodes length 300, total podCount 5000
	rec := doRequest(t, h, "/health/hub")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /health/hub status = %d, want 200", rec.Code)
	}
	var healthHub struct {
		Nodes []HealthHubNode `json:"nodes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &healthHub); err != nil {
		t.Fatalf("decode /health/hub: %v", err)
	}
	if len(healthHub.Nodes) != nodes {
		t.Errorf("health/hub nodes length = %d, want %d", len(healthHub.Nodes), nodes)
	}
	total := 0
	for _, n := range healthHub.Nodes {
		total += n.PodCount
	}
	if total != podsTotal {
		t.Errorf("total podCount = %d, want %d", total, podsTotal)
	}

	// GET /health/workers → 200 with 300 entries
	rec = doRequest(t, h, "/health/workers")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /health/workers status = %d, want 200", rec.Code)
	}
	var workers WorkersHealth
	if err := json.Unmarshal(rec.Body.Bytes(), &workers); err != nil {
		t.Fatalf("decode /health/workers: %v", err)
	}
	if len(workers.Workers) != nodes {
		t.Errorf("health/workers length = %d, want %d", len(workers.Workers), nodes)
	}

	// GET /worker length 5000
	rec = doRequest(t, h, "/worker")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /worker status = %d, want 200", rec.Code)
	}
	var pods []*corev1.Pod
	if err := json.Unmarshal(rec.Body.Bytes(), &pods); err != nil {
		t.Fatalf("decode /worker: %v", err)
	}
	if len(pods) != podsTotal {
		t.Errorf("GET /worker length = %d, want %d", len(pods), podsTotal)
	}

	// (b) POST 100 distinct licenses sequentially → all 200, final GET /license-equivalent (store.License()) holds last
	for i := 0; i < 100; i++ {
		lic := "LICENSE-" + strconv.Itoa(i)
		rec := doPost(t, h, "/license", `{"license":"`+lic+`"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("POST /license %d status = %d, want 200", i, rec.Code)
		}
	}
	if got := store.License(); got != "LICENSE-99" {
		t.Errorf("final store.License() = %q, want %q", got, "LICENSE-99")
	}

	// (c) GET /whoami, /flows2, /mcp still 200 after the flood
	rec = doRequest(t, h, "/whoami")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /whoami status = %d, want 200", rec.Code)
	}
	rec = doRequest(t, h, "/flows2")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /flows2 status = %d, want 200", rec.Code)
	}
	rec = doRequest(t, h, "/mcp")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /mcp status = %d, want 200", rec.Code)
	}

	// (d) recursively scan every response body for keys matching (?i)max|quota|tier|limit → none (allowlist none)
	forbidden := regexp.MustCompile(`(?i)(max|quota|tier|limit)`)
	var walk func(v any)
	walk = func(v any) {
		switch obj := v.(type) {
		case map[string]any:
			for key, val := range obj {
				if forbidden.MatchString(key) {
					t.Errorf("forbidden key %q in response body", key)
				}
				walk(val)
			}
		case []any:
			for _, val := range obj {
				walk(val)
			}
		}
	}

	for _, path := range []string{"/health/hub", "/health/workers", "/worker", "/whoami", "/mcp", "/license", "/"} {
		rec = doRequest(t, h, path)
		if rec.Code == http.StatusOK {
			var body any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err == nil {
				walk(body)
			}
		}
	}
}
