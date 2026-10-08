package hub

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func mkPod(name, node, ip string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "kubeshark"},
		Spec:       corev1.PodSpec{NodeName: node},
		Status:     corev1.PodStatus{PodIP: ip},
	}
}

func TestHealthHubCountsFromStore(t *testing.T) {
	store := NewStore()
	for i := 0; i < 6; i++ {
		store.UpsertPod(mkPod(fmt.Sprintf("wk-%d", i), fmt.Sprintf("node-%d", i/2), fmt.Sprintf("10.0.0.%d", i+1)))
	}

	h := store.HealthHub("1.2.3")
	if len(h.Nodes) != 3 {
		t.Errorf("nodes length = %d, want 3", len(h.Nodes))
	}
	for _, n := range h.Nodes {
		if n.PodCount != 2 {
			t.Errorf("node %q podCount = %d, want 2", n.NodeName, n.PodCount)
		}
	}
	if len(h.Workers) != 3 {
		t.Errorf("workers length = %d, want 3", len(h.Workers))
	}

	rec := doRequest(t, NewServer(Config{Version: "1.2.3"}, store, nil).Handler(), "/health/hub")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /health/hub status = %d, want 200", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	storage, ok := body["storage"].(map[string]any)
	if !ok {
		t.Fatalf("storage = %v, want a JSON object", body["storage"])
	}
	if _, ok := storage["diskUsed"]; !ok {
		t.Errorf("storage keys = %v, want diskUsed", storage)
	}

	var resp HealthHub
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	if resp.Timestamp.IsZero() {
		t.Errorf("timestamp = %s, want a non-zero time", resp.Timestamp)
	}
}

func TestHealthHubEmpty(t *testing.T) {
	rec := doRequest(t, NewServer(Config{}, NewStore(), nil).Handler(), "/health/hub")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /health/hub status = %d, want 200", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	for _, key := range []string{"workers", "nodes"} {
		v, ok := body[key].([]any)
		if !ok {
			t.Errorf("%s = %v, want [] not null", key, body[key])
			continue
		}
		if len(v) != 0 {
			t.Errorf("%s length = %d, want 0", key, len(v))
		}
	}
}

func TestHealthWorkersFromStore(t *testing.T) {
	store := NewStore()
	store.UpsertPod(mkPod("wk-a", "node-1", "10.0.0.1"))
	store.UpsertPod(mkPod("wk-a2", "node-1", "10.0.0.3"))
	store.UpsertPod(mkPod("wk-b", "node-2", "10.0.0.2"))

	rec := doRequest(t, NewServer(Config{}, store, nil).Handler(), "/health/workers")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /health/workers status = %d, want 200", rec.Code)
	}
	var got WorkersHealth
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	if len(got.Workers) != 2 {
		t.Fatalf("workers length = %d, want 2", len(got.Workers))
	}
	for _, w := range got.Workers {
		if w.Addr == "" || w.PodName == "" || w.NodeName == "" || w.LastSeen.IsZero() {
			t.Errorf("worker %+v, want non-empty addr/podName/nodeName and lastSeen", w)
		}
	}

	rec = doRequest(t, NewServer(Config{}, NewStore(), nil).Handler(), "/health/workers")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /health/workers status = %d, want 200", rec.Code)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	if got.Workers == nil || len(got.Workers) != 0 {
		t.Errorf("empty-store workers = %#v, want [] not null", got.Workers)
	}
}

func TestHealthHubHasNoLimitFields(t *testing.T) {
	rec := doRequest(t, NewServer(Config{}, NewStore(), nil).Handler(), "/health/hub")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /health/hub status = %d, want 200", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	forbidden := regexp.MustCompile(`(?i)max|limit|quota|tier`)
	var walk func(v any)
	walk = func(v any) {
		switch obj := v.(type) {
		case map[string]any:
			for key, val := range obj {
				if forbidden.MatchString(key) {
					t.Errorf("forbidden key %q in health response", key)
				}
				walk(val)
			}
		case []any:
			for _, val := range obj {
				walk(val)
			}
		}
	}
	walk(body)
}

func TestEnsureBlockedPodNoOp(t *testing.T) {
	h := NewServer(Config{}, NewStore(), nil).Handler()

	rec := doPost(t, h, "/networkpolicies/ensure-blocked-pod", `{"metadata":{"name":"blocked","namespace":"kubeshark"},"spec":{"nodeName":"node-1"}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /networkpolicies/ensure-blocked-pod status = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); body != "{}\n" && body != "{}" {
		t.Errorf("body = %q, want {}", body)
	}

	rec = doRequest(t, h, "/health/workers")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /health/workers status = %d, want 200", rec.Code)
	}
	var got WorkersHealth
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	if len(got.Workers) != 0 {
		t.Errorf("workers = %+v, want no state recorded from the no-op", got.Workers)
	}
}
