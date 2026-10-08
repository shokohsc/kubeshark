package hub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// connectPost issues a Connect JSON unary request the way the front does.
func connectPost(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	h.ServeHTTP(rec, req)
	return rec
}

// TestConnectUnaryJSON pins unary responses to the shapes the front bundle
// (index-CIV6AiaK.js) reads:
//   - validateDisplayFilter: `l?.matched` (proto ValidateDisplayFilterResponse.matched)
//   - listSnapshots: destructures {snapshots, cloudEnabled, cloudProvider, cloudConnected}
//   - getBaseEntriesDatabaseInfo: reads `c.databaseInfo` (proto database_info)
//   - Capture/DisplayFilter: brief-pinned {"status":"accepted"} (not called by name in the bundle)
func TestConnectUnaryJSON(t *testing.T) {
	cases := []struct {
		path string
		body string
	}{
		{"/capture.UIEventService/ValidateDisplayFilter", `{"matched":true}`},
		{"/capture.UIEventService/RequestPayload", `{}`},
		{"/capture.UIEventService/RequestPcap", `{}`},
		{"/capture.Capture/CaptureFilter", `{"status":"accepted"}`},
		{"/capture.Capture/DisplayFilter", `{"status":"accepted"}`},
		{"/snapshot.SnapshotManagement/CreateSnapshot", `{}`},
		{"/snapshot.SnapshotManagement/GetSnapshot", `{}`},
		{"/snapshot.SnapshotManagement/DeleteSnapshot", `{}`},
		{"/snapshot.SnapshotManagement/RenameSnapshot", `{}`},
		{"/snapshot.SnapshotManagement/GetDataTimeBoundaries", `{}`},
		{"/snapshot.SnapshotManagement/GetL7DataTimeBoundaries", `{}`},
		{"/snapshot.SnapshotData/GetDataTimeBoundaries", `{}`},
		{"/base_entries_database.BaseEntriesDatabaseService/GetBaseEntriesDatabaseInfo", `{"databaseInfo":{}}`},
		{"/base_entries_database.BaseEntriesDatabaseService/FetchBaseEntry", `{}`},
		{"/base_entries_database.BaseEntriesDatabaseService/FetchBaseEntryHistory", `{}`},
	}
	h := NewServer(Config{}, NewStore(), nil).Handler()
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			rec := connectPost(t, h, tc.path)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Fatalf("Content-Type = %q, want application/json", ct)
			}
			if got := json.Valid(rec.Body.Bytes()); !got {
				t.Fatalf("body is not JSON: %s", rec.Body)
			}
			var want, got any
			if err := json.Unmarshal([]byte(tc.body), &want); err != nil {
				t.Fatalf("bad test fixture %q: %v", tc.body, err)
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("bad response JSON %s: %v", rec.Body, err)
			}
			w, _ := json.Marshal(want)
			g, _ := json.Marshal(got)
			if string(w) != string(g) {
				t.Fatalf("body = %s, want %s", g, w)
			}
		})
	}
}

func TestConnectSnapshotListEmpty(t *testing.T) {
	h := NewServer(Config{}, NewStore(), nil).Handler()
	rec := connectPost(t, h, "/snapshot.SnapshotManagement/ListSnapshots")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body)
	}
	var body struct {
		Snapshots json.RawMessage `json:"snapshots"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v; body = %s", err, rec.Body)
	}
	if string(body.Snapshots) != "[]" {
		t.Fatalf("snapshots = %s, want non-nil []", body.Snapshots)
	}
}

func TestConnectUnimplementedMethod(t *testing.T) {
	h := NewServer(Config{}, NewStore(), nil).Handler()
	rec := connectPost(t, h, "/capture.PayloadService/LoadPayload")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", rec.Code, rec.Body)
	}
	var body struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v; body = %s", err, rec.Body)
	}
	if body.Code != "unimplemented" {
		t.Fatalf("code = %q, want unimplemented", body.Code)
	}
	if want := "method /capture.PayloadService/LoadPayload not implemented"; body.Message != want {
		t.Fatalf("message = %q, want %q", body.Message, want)
	}
}

func TestConnectUnknownService(t *testing.T) {
	h := NewServer(Config{}, NewStore(), nil).Handler()
	rec := connectPost(t, h, "/foo.Bar/Baz")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", rec.Code, rec.Body)
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v; body = %s", err, rec.Body)
	}
	if body.Code != "unimplemented" {
		t.Fatalf("code = %q, want unimplemented", body.Code)
	}
}

func TestConnectAuthApplied(t *testing.T) {
	cfg := Config{
		AuthEnabled:     true,
		ServiceAccounts: []string{"ns:kubeshark-cli"},
		License:         "lic-1234",
	}
	h := NewServer(cfg, NewStore(), &stubVerifier{subject: "ns:kubeshark-cli"}).Handler()
	rec := connectPost(t, h, "/capture.UIEventService/ValidateDisplayFilter")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}
