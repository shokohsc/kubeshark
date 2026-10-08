package hub

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestMCPToolsList(t *testing.T) {
	h := NewServer(Config{Version: "1.2.3"}, NewStore(), nil).Handler()

	rec := doRequest(t, h, "/mcp")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /mcp status = %d, want 200", rec.Code)
	}
	var got struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		Tools   []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"inputSchema"`
		} `json:"tools"`
		Prompts []json.RawMessage `json:"prompts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode GET /mcp body %q: %v", rec.Body.String(), err)
	}
	if len(got.Tools) == 0 {
		t.Fatal("GET /mcp returned no tools")
	}
	names := make(map[string]bool)
	for _, tool := range got.Tools {
		names[tool.Name] = true
		if tool.Description == "" {
			t.Errorf("tool %q has empty description", tool.Name)
		}
		if !json.Valid(tool.InputSchema) {
			t.Errorf("tool %q has invalid inputSchema %q", tool.Name, tool.InputSchema)
		}
	}
	// The 14 hub tools from mcp/README.md:136-160.
	for _, want := range []string{
		"list_workloads", "list_api_calls", "get_api_call", "get_api_stats",
		"list_l4_flows", "get_l4_flow_summary", "list_snapshots", "create_snapshot",
		"get_dissection_status", "enable_dissection", "disable_dissection",
		"check_kubeshark_status", "start_kubeshark", "stop_kubeshark",
	} {
		if !names[want] {
			t.Errorf("GET /mcp tools missing %q", want)
		}
	}
	// Local-only tools are served by the CLI, never by the hub.
	for _, local := range []string{"get_file_url", "download_file"} {
		if names[local] {
			t.Errorf("GET /mcp must not advertise CLI-local tool %q", local)
		}
	}
	if got.Prompts == nil {
		t.Error("GET /mcp body missing prompts array")
	}
}

func TestMCPCallListTool(t *testing.T) {
	h := NewServer(Config{}, NewStore(), nil).Handler()

	for _, name := range []string{
		"list_workloads", "list_api_calls", "get_api_call", "get_api_stats",
		"list_l4_flows", "get_l4_flow_summary", "list_snapshots",
		"get_dissection_status", "check_kubeshark_status",
	} {
		t.Run(name, func(t *testing.T) {
			rec := doPost(t, h, "/mcp/tools/call", `{"name":"`+name+`","arguments":{}}`)
			if rec.Code != http.StatusOK {
				t.Fatalf("POST /mcp/tools/call %s status = %d, want 200", name, rec.Code)
			}
			// The CLI passes the raw body through (callHubTool), so the body must
			// be the raw payload JSON — never an MCP result envelope.
			var got map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode body %q: %v (want raw payload JSON object)", rec.Body.String(), err)
			}
			switch name {
			case "list_workloads":
				assertEmptyArray(t, got, "workloads")
			case "list_api_calls":
				assertEmptyArray(t, got, "calls")
			case "get_api_stats":
				stats, _ := got["stats"].(map[string]any)
				if v, ok := stats["total_calls"]; !ok || v != float64(0) {
					t.Errorf("stats = %v, want total_calls == 0", got["stats"])
				}
			case "check_kubeshark_status":
				if v, ok := got["running"]; !ok || v != false {
					t.Errorf("running = %v, want false with empty store", got["running"])
				}
			}
		})
	}
}

func TestMCPCallControlTool(t *testing.T) {
	h := NewServer(Config{}, NewStore(), nil).Handler()

	for _, name := range []string{
		"create_snapshot", "enable_dissection", "disable_dissection",
		"start_kubeshark", "stop_kubeshark",
	} {
		t.Run(name, func(t *testing.T) {
			rec := doPost(t, h, "/mcp/tools/call", `{"name":"`+name+`","arguments":{}}`)
			if rec.Code != http.StatusNotImplemented {
				t.Fatalf("POST /mcp/tools/call %s status = %d, want 501", name, rec.Code)
			}
			if got := strings.TrimSpace(rec.Body.String()); got != "data-plane not available in this build" {
				t.Errorf("body = %q, want %q", got, "data-plane not available in this build")
			}
		})
	}
}

func TestMCPCallUnknown(t *testing.T) {
	h := NewServer(Config{}, NewStore(), nil).Handler()

	rec := doPost(t, h, "/mcp/tools/call", `{"name":"nope","arguments":{}}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("POST /mcp/tools/call unknown status = %d, want 404", rec.Code)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != "unknown tool: nope" {
		t.Errorf("body = %q, want %q", got, "unknown tool: nope")
	}
}

func assertEmptyArray(t *testing.T, obj map[string]any, key string) {
	t.Helper()
	v, ok := obj[key]
	if !ok {
		t.Errorf("body missing key %q", key)
		return
	}
	arr, ok := v.([]any)
	if !ok || len(arr) != 0 {
		t.Errorf("%q = %v, want empty array", key, v)
	}
}
