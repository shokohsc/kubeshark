package hub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// doFront issues an arbitrary-method request with the given body.
func doFront(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rec
}

func decodeFrontJSON(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	return got
}

func TestWhoamiShape(t *testing.T) {
	h := NewServer(Config{}, NewStore(), nil).Handler()

	rec := doRequest(t, h, "/whoami")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /whoami status = %d, want 200", rec.Code)
	}
	got := decodeFrontJSON(t, rec)
	if len(got) != 4 {
		t.Errorf("GET /whoami body = %v, want exactly 4 keys: authenticated, user, capabilities, authzNamespaces", got)
	}
	for _, key := range []string{"authenticated", "user", "capabilities", "authzNamespaces"} {
		if _, ok := got[key]; !ok {
			t.Errorf("GET /whoami missing key %q (body %v)", key, got)
		}
	}
	if got["authenticated"] != false {
		t.Errorf("authenticated = %v, want false with auth off", got["authenticated"])
	}
	if got["user"] != nil {
		t.Errorf("user = %v, want null with auth off", got["user"])
	}
	if got["capabilities"] != nil || got["authzNamespaces"] != nil {
		t.Errorf("capabilities/authzNamespaces = %v/%v, want null", got["capabilities"], got["authzNamespaces"])
	}
}

func TestWhoamiUserWithServiceAccountToken(t *testing.T) {
	cfg := Config{
		AuthEnabled:     true,
		ServiceAccounts: []string{"ns:kubeshark-cli"},
		License:         "lic-1234",
	}

	t.Run("service account subject", func(t *testing.T) {
		h := NewServer(cfg, NewStore(), &stubVerifier{subject: "ns:kubeshark-cli"}).Handler()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/whoami", nil)
		req.Header.Set("Authorization", "Bearer tok")
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("GET /whoami status = %d, want 200", rec.Code)
		}
		got := decodeFrontJSON(t, rec)
		if got["user"] != "ns:kubeshark-cli" {
			t.Errorf("user = %v, want verified subject %q", got["user"], "ns:kubeshark-cli")
		}
		if got["authenticated"] != true {
			t.Errorf("authenticated = %v, want true", got["authenticated"])
		}
	})

	t.Run("license key stays null", func(t *testing.T) {
		h := NewServer(cfg, NewStore(), &stubVerifier{subject: "ns:kubeshark-cli"}).Handler()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/whoami", nil)
		req.Header.Set("License-Key", "lic-1234")
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("GET /whoami status = %d, want 200", rec.Code)
		}
		got := decodeFrontJSON(t, rec)
		if got["user"] != nil {
			t.Errorf("user = %v, want null for License-Key auth", got["user"])
		}
		if got["authenticated"] != true {
			t.Errorf("authenticated = %v, want true (config-level, not per-request)", got["authenticated"])
		}
	})
}

func TestAuthSessionShape(t *testing.T) {
	cfg := Config{AuthEnabled: true, ServiceAccounts: []string{"ns:kubeshark-cli"}}
	h := NewServer(cfg, NewStore(), &stubVerifier{subject: "ns:kubeshark-cli"}).Handler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/auth/session", nil)
	req.Header.Set("Authorization", "Bearer tok")
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /auth/session status = %d, want 200", rec.Code)
	}
	got := decodeFrontJSON(t, rec)
	if len(got) != 2 || got["authenticated"] != true || got["user"] != nil {
		t.Errorf("GET /auth/session body = %v, want {authenticated:true user:null} only", got)
	}
}

func TestSettingsDissectionRoundTrip(t *testing.T) {
	h := NewServer(Config{}, NewStore(), nil).Handler()

	rec := doRequest(t, h, "/settings/status")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /settings/status status = %d, want 200", rec.Code)
	}
	if got := decodeFrontJSON(t, rec); got["dissection"] != true {
		t.Errorf("settings/status dissection = %v, want default true", got["dissection"])
	}

	rec = doRequest(t, h, "/settings/dissection")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /settings/dissection status = %d, want 200", rec.Code)
	}
	if got := decodeFrontJSON(t, rec); got["enabled"] != true {
		t.Errorf("settings/dissection enabled = %v, want default true", got["enabled"])
	}

	rec = doFront(t, h, http.MethodPost, "/settings/dissection", `{"enabled":false}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /settings/dissection status = %d, want 200", rec.Code)
	}
	if got := decodeFrontJSON(t, rec); got["enabled"] != false {
		t.Errorf("POST /settings/dissection body = %v, want {enabled:false}", got)
	}

	rec = doRequest(t, h, "/settings/dissection")
	if got := decodeFrontJSON(t, rec); got["enabled"] != false {
		t.Errorf("GET /settings/dissection after POST = %v, want {enabled:false}", got)
	}

	rec = doRequest(t, h, "/settings/status")
	if got := decodeFrontJSON(t, rec); got["dissection"] != false {
		t.Errorf("settings/status after POST = %v, want dissection false", got)
	}
}

func TestScriptsCRUD(t *testing.T) {
	h := NewServer(Config{}, NewStore(), nil).Handler()

	list := func(t *testing.T) []map[string]any {
		t.Helper()
		rec := doRequest(t, h, "/scripts")
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /scripts status = %d, want 200", rec.Code)
		}
		if !strings.HasPrefix(strings.TrimSpace(rec.Body.String()), "[") {
			t.Fatalf("GET /scripts body = %q, want JSON array (never null)", rec.Body.String())
		}
		var got []map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode GET /scripts body %q: %v", rec.Body.String(), err)
		}
		return got
	}

	rec := doFront(t, h, http.MethodPost, "/scripts", `{"id":"s1","name":"hello","language":"javascript","code":"1+1"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /scripts status = %d, want 200", rec.Code)
	}
	if got := decodeFrontJSON(t, rec); got["id"] != "s1" || got["name"] != "hello" {
		t.Errorf("POST /scripts body = %v, want stored script", got)
	}
	if scripts := list(t); len(scripts) != 1 || scripts[0]["id"] != "s1" {
		t.Errorf("GET /scripts after create = %v, want one script s1", scripts)
	}

	rec = doFront(t, h, http.MethodPost, "/scripts/exec", `{"id":"s1"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /scripts/exec status = %d, want 200", rec.Code)
	}
	if got := decodeFrontJSON(t, rec); got["status"] != "ok" {
		t.Errorf("POST /scripts/exec body = %v, want {status:ok}", got)
	}

	rec = doFront(t, h, http.MethodPut, "/scripts/s1", `{"name":"renamed","language":"javascript","code":"2+2"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT /scripts/s1 status = %d, want 200", rec.Code)
	}
	if scripts := list(t); len(scripts) != 1 || scripts[0]["name"] != "renamed" || scripts[0]["id"] != "s1" {
		t.Errorf("GET /scripts after PUT = %v, want one edited script s1", scripts)
	}

	rec = doFront(t, h, http.MethodPost, "/scripts/s1/activate", `{}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /scripts/s1/activate status = %d, want 200", rec.Code)
	}
	if scripts := list(t); len(scripts) != 1 || scripts[0]["active"] != true {
		t.Errorf("GET /scripts after activate = %v, want active true", scripts)
	}

	rec = doFront(t, h, http.MethodPost, "/scripts/s1/deactivate", `{}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /scripts/s1/deactivate status = %d, want 200", rec.Code)
	}
	if scripts := list(t); len(scripts) != 1 || scripts[0]["active"] != false {
		t.Errorf("GET /scripts after deactivate = %v, want active false", scripts)
	}

	rec = doFront(t, h, http.MethodDelete, "/scripts/s1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE /scripts/s1 status = %d, want 200", rec.Code)
	}
	if scripts := list(t); len(scripts) != 0 {
		t.Errorf("GET /scripts after DELETE = %v, want empty list", scripts)
	}

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodPut, "/scripts/nope"},
		{http.MethodDelete, "/scripts/nope"},
		{http.MethodPost, "/scripts/nope/activate"},
		{http.MethodPost, "/scripts/nope/deactivate"},
	} {
		rec := doFront(t, h, tc.method, tc.path, `{"id":"nope"}`)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s %s status = %d, want 404", tc.method, tc.path, rec.Code)
		}
	}
}

func TestQueryValidateAndFilters(t *testing.T) {
	h := NewServer(Config{}, NewStore(), nil).Handler()

	rec := doRequest(t, h, "/query/validate?q=name%20%3D%20%22api%22")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /query/validate status = %d, want 200", rec.Code)
	}
	if got := decodeFrontJSON(t, rec); got["valid"] != true {
		t.Errorf("query/validate body = %v, want {valid:true}", got)
	}

	rec = doRequest(t, h, "/query/filters")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /query/filters status = %d, want 200", rec.Code)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
		t.Errorf("query/filters body = %q, want []", got)
	}

	rec = doFront(t, h, http.MethodPost, "/query/filter", `{"filters":[]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /query/filter status = %d, want 200", rec.Code)
	}
	if got := decodeFrontJSON(t, rec); got["matched"] != float64(0) {
		t.Errorf("query/filter body = %v, want {matched:0}", got)
	}
}

func TestPodsTargetSettings(t *testing.T) {
	h := NewServer(Config{}, NewStore(), nil).Handler()

	rec := doRequest(t, h, "/pods/target/settings")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /pods/target/settings status = %d, want 200", rec.Code)
	}
	got := decodeFrontJSON(t, rec)
	targets, ok := got["targets"].([]any)
	if !ok || len(targets) != 0 {
		t.Errorf("pods/target/settings targets = %v, want empty array (never null)", got["targets"])
	}

	for i := 0; i < 2; i++ {
		rec = doFront(t, h, http.MethodPost, "/pods/target/a.default.svc", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("POST /pods/target/a.default.svc status = %d, want 200", rec.Code)
		}
	}

	rec = doRequest(t, h, "/pods/target/settings")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /pods/target/settings status = %d, want 200", rec.Code)
	}
	got = decodeFrontJSON(t, rec)
	targets, ok = got["targets"].([]any)
	if !ok || len(targets) != 1 || targets[0] != "a.default.svc" {
		t.Errorf("pods/target/settings targets = %v, want [a.default.svc] once", got["targets"])
	}
}

func TestDiscardRoutesOK(t *testing.T) {
	h := NewServer(Config{}, NewStore(), nil).Handler()

	for _, tc := range []struct {
		name string
		path string
		body string
	}{
		{"sentry front", "/sentry/front", `{"event":"pageview","user":{"id":1}}`},
		{"oauth2 logout", "/oauth2/logout", `{}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := doFront(t, h, http.MethodPost, tc.path, tc.body)
			if rec.Code != http.StatusOK {
				t.Errorf("POST %s status = %d, want 200", tc.path, rec.Code)
			}
		})
	}
}

func TestMetadataVersion(t *testing.T) {
	h := NewServer(Config{Version: "9.9.9"}, NewStore(), nil).Handler()

	rec := doRequest(t, h, "/metadata/version")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /metadata/version status = %d, want 200", rec.Code)
	}
	got := decodeFrontJSON(t, rec)
	if v, ok := got["version"].(string); !ok || v != "9.9.9" {
		t.Errorf("metadata/version body = %v, want {version:9.9.9}", got)
	}
}

func TestLicenseFrontRoutes(t *testing.T) {
	store := NewStore()
	h := NewServer(Config{}, store, nil).Handler()

	getLicense := func(t *testing.T) string {
		t.Helper()
		rec := doRequest(t, h, "/license")
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /license status = %d, want 200", rec.Code)
		}
		license, _ := decodeFrontJSON(t, rec)["license"].(string)
		return license
	}

	if got := getLicense(t); got != "" {
		t.Errorf("GET /license = %q, want empty before push", got)
	}

	rec := doFront(t, h, http.MethodPost, "/license/push", `{"license":"KEY-1"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /license/push status = %d, want 200", rec.Code)
	}
	if got := decodeFrontJSON(t, rec); got["status"] != "pushed" {
		t.Errorf("POST /license/push body = %v, want {status:pushed}", got)
	}
	if got := getLicense(t); got != "KEY-1" {
		t.Errorf("GET /license after push = %q, want KEY-1", got)
	}
	if got := store.License(); got != "KEY-1" {
		t.Errorf("store.License() after push = %q, want KEY-1", got)
	}

	rec = doFront(t, h, http.MethodPost, "/license/remove", `{}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /license/remove status = %d, want 200", rec.Code)
	}
	if got := decodeFrontJSON(t, rec); got["status"] != "removed" {
		t.Errorf("POST /license/remove body = %v, want {status:removed}", got)
	}
	if got := getLicense(t); got != "" {
		t.Errorf("GET /license after remove = %q, want empty", got)
	}

	rec = doFront(t, h, http.MethodPost, "/license/push", `{"license":`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST /license/push malformed status = %d, want 400", rec.Code)
	}
}

func TestFlows2ServesRing(t *testing.T) {
	store := NewStore()
	h := NewServer(Config{}, store, nil).Handler()

	for _, entry := range []string{`{"id":"a"}`, `{"id":"b"}`, `{"id":"c"}`} {
		store.Ring().Append(json.RawMessage(entry))
	}

	rec := doRequest(t, h, "/flows2?aggregate=n")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /flows2 status = %d, want 200", rec.Code)
	}
	var got []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode GET /flows2 body %q: %v", rec.Body.String(), err)
	}
	if len(got) != 3 {
		t.Fatalf("GET /flows2 len = %d, want 3 entries in append order", len(got))
	}
	for i, want := range []string{"a", "b", "c"} {
		if got[i]["id"] != want {
			t.Errorf("GET /flows2[%d].id = %v, want %q", i, got[i]["id"], want)
		}
	}
}

func TestFlows2Empty(t *testing.T) {
	h := NewServer(Config{}, NewStore(), nil).Handler()

	rec := doRequest(t, h, "/flows2")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /flows2 status = %d, want 200", rec.Code)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
		t.Errorf("GET /flows2 empty body = %q, want [] (never null)", got)
	}
}

func TestFetchRecordsShapes(t *testing.T) {
	h := NewServer(Config{}, NewStore(), nil).Handler()

	for _, tc := range []struct {
		name   string
		method string
		path   string
		body   string
		want   string
	}{
		{"fetch-records GET", http.MethodGet, "/fetch-records", "", `{"Contents":[]}`},
		{"fetch-records POST", http.MethodPost, "/fetch-records", "{}", `{"Contents":[]}`},
		{"cloud buckets", http.MethodGet, "/fetch-records/cloud/buckets", "", `{"Contents":[]}`},
		{"list nodes", http.MethodGet, "/fetch-records/list/nodes", "", `[]`},
		{"get script", http.MethodPost, "/fetch-records/list/get-script", "{}", `""`},
		{"records POST", http.MethodPost, "/records", "{}", `{}`},
		{"records bulk DELETE", http.MethodDelete, "/records/bulk", "", `{"deleted":0}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := doFront(t, h, tc.method, tc.path, tc.body)
			if rec.Code != http.StatusOK {
				t.Fatalf("%s %s status = %d, want 200", tc.method, tc.path, rec.Code)
			}
			if got := strings.TrimSpace(rec.Body.String()); got != tc.want {
				t.Errorf("%s %s body = %q, want %q", tc.method, tc.path, got, tc.want)
			}
		})
	}
}
