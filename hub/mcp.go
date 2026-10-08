package hub

import (
	"encoding/json"
	"net/http"
	"slices"
)

// mcpTool mirrors the tool object shape the CLI parses from GET /mcp
// (hubMCPTool in cmd/mcpRunner.go; stub expectations in cmd/mcp_test.go).
type mcpTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

type mcpToolsResponse struct {
	Name    string    `json:"name"`
	Version string    `json:"version"`
	Tools   []mcpTool `json:"tools"`
	Prompts []any     `json:"prompts"`
}

type mcpCallRequest struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type mcpCallResult struct {
	Content []mcpContent `json:"content"`
	IsError bool         `json:"isError,omitempty"`
}

const mcpDataPlaneMsg = "data-plane not available in this build"

var mcpObjectSchema = json.RawMessage(`{"type":"object"}`)

// mcpTools is the legacy hub tool list (mcp/README.md:136-160). The CLI-local
// tools get_file_url and download_file are deliberately absent.
var mcpTools = []mcpTool{
	{Name: "list_workloads", Description: "List pods, services, namespaces with observed traffic", InputSchema: mcpObjectSchema},
	{Name: "list_api_calls", Description: "Query L7 API transactions with KFL filtering", InputSchema: mcpObjectSchema},
	{Name: "get_api_call", Description: "Get detailed info about a specific API call", InputSchema: mcpObjectSchema},
	{Name: "get_api_stats", Description: "Get aggregated API statistics", InputSchema: mcpObjectSchema},
	{Name: "list_l4_flows", Description: "List L4 (TCP/UDP) network flows", InputSchema: mcpObjectSchema},
	{Name: "get_l4_flow_summary", Description: "Get L4 connectivity summary", InputSchema: mcpObjectSchema},
	{Name: "list_snapshots", Description: "List all PCAP snapshots", InputSchema: mcpObjectSchema},
	{Name: "create_snapshot", Description: "Create a new PCAP snapshot", InputSchema: mcpObjectSchema},
	{Name: "get_dissection_status", Description: "Check L7 protocol parsing status", InputSchema: mcpObjectSchema},
	{Name: "enable_dissection", Description: "Enable L7 protocol dissection", InputSchema: mcpObjectSchema},
	{Name: "disable_dissection", Description: "Disable L7 protocol dissection", InputSchema: mcpObjectSchema},
	{Name: "check_kubeshark_status", Description: "Check if Kubeshark is running", InputSchema: mcpObjectSchema},
	{Name: "start_kubeshark", Description: "Deploy Kubeshark to cluster", InputSchema: mcpObjectSchema},
	{Name: "stop_kubeshark", Description: "Remove Kubeshark from cluster", InputSchema: mcpObjectSchema},
}

// mcpDataPlaneTools need a live data plane; this build has none.
var mcpDataPlaneTools = []string{
	"create_snapshot", "enable_dissection", "disable_dissection",
	"start_kubeshark", "stop_kubeshark",
}

// mcpEmptyPayloads are the empty/zero data sets for the read tools; keys match
// the stub responses in cmd/mcp_test.go where they are defined there.
var mcpEmptyPayloads = map[string]any{
	"list_workloads":        map[string]any{"workloads": []any{}},
	"list_api_calls":        map[string]any{"calls": []any{}},
	"get_api_call":          map[string]any{"id": "", "path": ""},
	"get_api_stats":         map[string]any{"stats": map[string]any{"total_calls": 0}},
	"list_l4_flows":         map[string]any{"flows": []any{}},
	"get_l4_flow_summary":   map[string]any{"summary": map[string]any{}},
	"list_snapshots":        map[string]any{"snapshots": []any{}},
	"get_dissection_status": map[string]any{"enabled": false},
}

// handleMCPInfo serves GET /mcp: the tool/prompt catalogue the CLI merges
// into its own tools/list (fetchHubMCP in cmd/mcpRunner.go).
func (s *Server) handleMCPInfo(w http.ResponseWriter, _ *http.Request) {
	// ponytail: prompts stay an empty array — the 8 legacy prompts in
	// mcp/README.md:162-173 are not served, so the CLI never merges hub prompts.
	writeJSON(w, mcpToolsResponse{
		Name:    "kubeshark-hub",
		Version: s.cfg.Version,
		Tools:   mcpTools,
		Prompts: []any{},
	})
}

// handleMCPCallTool serves POST /mcp/tools/call; every outcome is HTTP 200 —
// MCP protocol errors travel in-body as isError results.
func (s *Server) handleMCPCallTool(w http.ResponseWriter, r *http.Request) {
	var req mcpCallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, s.mcpCall(req.Name))
}

func (s *Server) mcpCall(name string) mcpCallResult {
	if slices.Contains(mcpDataPlaneTools, name) {
		return mcpSuccess(mcpDataPlaneMsg)
	}
	payload, ok := s.mcpPayload(name)
	if !ok {
		return mcpErrorResult("unknown tool: " + name)
	}
	text, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return mcpErrorResult("encode error: " + err.Error())
	}
	return mcpSuccess(string(text))
}

func (s *Server) mcpPayload(name string) (any, bool) {
	if name == "check_kubeshark_status" {
		return map[string]any{"running": len(s.store.Pods()) > 0}, true
	}
	payload, ok := mcpEmptyPayloads[name]
	return payload, ok
}

func mcpSuccess(text string) mcpCallResult {
	return mcpCallResult{Content: []mcpContent{{Type: "text", Text: text}}}
}

func mcpErrorResult(text string) mcpCallResult {
	return mcpCallResult{Content: []mcpContent{{Type: "text", Text: text}}, IsError: true}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(v)
}
