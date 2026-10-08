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
	"get_api_call":          map[string]any{"id": "", "path": ""}, // ponytail: missing-id args not validated hub-side; the CLI tolerates both 200 and non-200
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

// handleMCPCallTool serves POST /mcp/tools/call. The CLI (callHubTool in
// cmd/mcpRunner.go) passes the raw body through as the tool text and derives
// isError from the HTTP status only, so success is the raw payload JSON and
// failures are non-2xx plain-text bodies.
func (s *Server) handleMCPCallTool(w http.ResponseWriter, r *http.Request) {
	var req mcpCallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if slices.Contains(mcpDataPlaneTools, req.Name) {
		http.Error(w, mcpDataPlaneMsg, http.StatusNotImplemented)
		return
	}
	payload, ok := s.mcpPayload(req.Name)
	if !ok {
		http.Error(w, "unknown tool: "+req.Name, http.StatusNotFound)
		return
	}
	writeJSON(w, payload)
}

func (s *Server) mcpPayload(name string) (any, bool) {
	if name == "check_kubeshark_status" {
		return map[string]any{"running": len(s.store.Pods()) > 0}, true
	}
	payload, ok := mcpEmptyPayloads[name]
	return payload, ok
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(v)
}
