package hub

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

type mcpTestResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError"`
}

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

	// Empty data-set shape for the tools whose stub responses are defined in
	// cmd/mcp_test.go; the rest must still return success-shaped pretty JSON.
	wantSubstring := map[string]string{
		"list_workloads": `"workloads": []`,
		"list_api_calls": `"calls": []`,
		"get_api_stats":  `"total_calls": 0`,
	}
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
			res := parseMCPTestResult(t, rec.Body.Bytes())
			if len(res.Content) == 0 || res.Content[0].Type != "text" {
				t.Fatalf("result content = %+v, want one text item", res.Content)
			}
			if res.IsError {
				t.Errorf("isError = true, want success result")
			}
			text := res.Content[0].Text
			if !strings.Contains(text, "\n") {
				t.Errorf("text = %q, want pretty-printed JSON", text)
			}
			if !json.Valid([]byte(text)) {
				t.Errorf("text = %q, want valid JSON", text)
			}
			if sub, ok := wantSubstring[name]; ok && !strings.Contains(text, sub) {
				t.Errorf("text = %q, want it to contain %q", text, sub)
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
			if rec.Code != http.StatusOK {
				t.Fatalf("POST /mcp/tools/call %s status = %d, want 200", name, rec.Code)
			}
			res := parseMCPTestResult(t, rec.Body.Bytes())
			if len(res.Content) == 0 || res.Content[0].Type != "text" {
				t.Fatalf("result content = %+v, want one text item", res.Content)
			}
			if res.IsError {
				t.Errorf("isError = true, want success-shaped result")
			}
			if !strings.Contains(res.Content[0].Text, "data-plane not available in this build") {
				t.Errorf("text = %q, want it to contain %q", res.Content[0].Text, "data-plane not available in this build")
			}
		})
	}
}

func TestMCPCallUnknown(t *testing.T) {
	h := NewServer(Config{}, NewStore(), nil).Handler()

	rec := doPost(t, h, "/mcp/tools/call", `{"name":"nope","arguments":{}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /mcp/tools/call unknown status = %d, want 200 (MCP errors are in-body)", rec.Code)
	}
	res := parseMCPTestResult(t, rec.Body.Bytes())
	if !res.IsError {
		t.Error("isError = false, want MCP error result for unknown tool")
	}
	if len(res.Content) == 0 || res.Content[0].Text == "" {
		t.Errorf("content = %+v, want non-empty error text", res.Content)
	}
}

func parseMCPTestResult(t *testing.T, body []byte) mcpTestResult {
	t.Helper()
	var res mcpTestResult
	if err := json.Unmarshal(body, &res); err != nil {
		t.Fatalf("decode result body %q: %v", body, err)
	}
	return res
}
