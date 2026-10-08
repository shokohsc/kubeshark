package hub

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStreamClusterInfoConsumed(t *testing.T) {
	store := NewStore()
	h := NewServer(Config{}, store, nil).Handler()

	body := `{"type":"message","value":{"nodes":1}}
{"type":"message","value":{"nodes":2}}
{"type":"end"}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/capture.Capture/StreamClusterInfo", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/connect+json")
	req.Header.Set("Connect-Protocol-Version", "1")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "application/connect+json" && !strings.HasPrefix(ct, "application/connect") {
		t.Fatalf("Content-Type = %q", ct)
	}
	if !strings.Contains(rec.Body.String(), `{"type":"end"}`) {
		t.Fatalf("body = %q, want end", rec.Body.String())
	}
	if got := store.ClusterInfo(); string(got) != `{"nodes":2}` {
		t.Fatalf("clusterInfo = %s, want %s", got, `{"nodes":2}`)
	}
}

func TestCaptureBaseEntriesIngestsToRing(t *testing.T) {
	store := NewStore()
	h := NewServer(Config{}, store, nil).Handler()

	body := `{"type":"message","value":{"id":1,"ts":1}}
{"type":"message","value":{"id":2,"ts":2}}
{"type":"message","value":{"id":3,"ts":3}}
{"type":"end"}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/capture.Capture/CaptureBaseEntries", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/connect+json")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	snap := store.Ring().Snapshot()
	if len(snap) != 3 {
		t.Fatalf("len = %d, want 3", len(snap))
	}
	if string(snap[0]) != `{"id":1,"ts":1}` || string(snap[1]) != `{"id":2,"ts":2}` || string(snap[2]) != `{"id":3,"ts":3}` {
		t.Fatalf("snap = %s %s %s", snap[0], snap[1], snap[2])
	}
}

func TestRegisterClientHeartbeatAndRelay(t *testing.T) {
	store := NewStore()
	h := NewServer(Config{}, store, nil).Handler()

	srv := httptest.NewServer(h)
	defer srv.Close()

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/capture.UIEventService/RegisterClient", bytes.NewReader(nil))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/connect+json")
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	reader := bufio.NewScanner(resp.Body)
	reader.Buffer(make([]byte, 1024*1024), 1024*1024)
	reader.Split(bufio.ScanLines)

	if !reader.Scan() {
		t.Fatal("expected first line")
	}
	line := reader.Bytes()
	var env struct {
		Type  string          `json:"type"`
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(line, &env); err != nil {
		t.Fatalf("unmarshal: %v, line=%s", err, line)
	}
	if env.Type != "message" {
		t.Fatalf("type = %s", env.Type)
	}
	if !strings.Contains(string(env.Value), "heartbeat") {
		t.Fatalf("value = %s, want heartbeat", env.Value)
	}

	entry := json.RawMessage(`{"entry":123}`)
	store.Ring().Append(entry)
	if !reader.Scan() {
		t.Fatal("expected relay line")
	}
	relay := reader.Bytes()
	if err := json.Unmarshal(relay, &env); err != nil {
		t.Fatalf("unmarshal relay: %v", err)
	}
	if env.Type != "message" || string(env.Value) != string(entry) {
		_ = relay
		t.Fatalf("relay = %s %s", env.Type, string(env.Value))
	}

	resp.Body.Close()
}

func TestFrontStreamsEndImmediately(t *testing.T) {
	store := NewStore()
	h := NewServer(Config{}, store, nil).Handler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/snapshot.SnapshotData/GetFiles", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/connect+json")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `{"type":"end"}`) {
		t.Fatalf("body = %q", body)
	}
}

func TestUnimplementedStream(t *testing.T) {
	store := NewStore()
	h := NewServer(Config{}, store, nil).Handler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/unknown.Service/UnknownMethod", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/connect+json")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
	var b struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatal(err)
	}
	if b.Code != "unimplemented" {
		t.Fatalf("code = %q", b.Code)
	}
}
