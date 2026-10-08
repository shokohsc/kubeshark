package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type connectMethod = func(ctx context.Context, body json.RawMessage) (json.RawMessage, error)

// connectMethods maps full Connect paths (/{pkg}.{Service}/{Method}) to unary
// handlers.
// ponytail: nil = method exists in the proto but is not implemented yet
// (streams and cloud transfer land in later tasks).
var connectMethods = map[string]connectMethod{
	"/capture.UIEventService/ValidateDisplayFilter": constMessage(`{"matched":true}`),
	"/capture.UIEventService/RequestPayload":        constMessage(`{}`),
	"/capture.UIEventService/RequestPcap":           constMessage(`{}`),
	"/capture.Capture/CaptureFilter":                constMessage(`{"status":"accepted"}`),
	"/capture.Capture/DisplayFilter":                constMessage(`{"status":"accepted"}`),
	"/snapshot.SnapshotManagement/CreateSnapshot":   constMessage(`{}`),
	"/snapshot.SnapshotManagement/GetSnapshot":      constMessage(`{}`),
	"/snapshot.SnapshotManagement/ListSnapshots":    constMessage(`{"snapshots":[]}`),
	"/snapshot.SnapshotManagement/DeleteSnapshot":   constMessage(`{}`),
	"/snapshot.SnapshotManagement/RenameSnapshot":   constMessage(`{}`),
	// ponytail: time-boundary responses are empty {} stubs; front shows loading until real data lands.
	"/snapshot.SnapshotManagement/GetDataTimeBoundaries":                           constMessage(`{}`),
	"/snapshot.SnapshotManagement/GetL7DataTimeBoundaries":                         constMessage(`{}`),
	"/snapshot.SnapshotData/GetDataTimeBoundaries":                                 constMessage(`{}`),
	"/base_entries_database.BaseEntriesDatabaseService/GetBaseEntriesDatabaseInfo": constMessage(`{"databaseInfo":{}}`),
	// ponytail: fetch responses are empty {} stubs; front shows empty until real data lands.
	"/base_entries_database.BaseEntriesDatabaseService/FetchBaseEntry":        constMessage(`{}`),
	"/base_entries_database.BaseEntriesDatabaseService/FetchBaseEntryHistory": constMessage(`{}`),
	"/capture.PayloadService/LoadPayload":                                     nil,
	"/capture.PayloadService/LoadPcap":                                        nil,
	"/snapshot.SnapshotManagement/UploadSnapshotToCloud":                      nil,
	"/snapshot.SnapshotManagement/DownloadSnapshotFromCloud":                  nil,
	"/snapshot.SnapshotManagement/UploadSnapshot":                             nil,
	"/snapshot.SnapshotManagement/DownloadSnapshot":                           nil,
	"/snapshot.SnapshotManagement/ListSnapshotFiles":                          nil,
	"/snapshot.SnapshotManagement/GetSnapshotFile":                            nil,
}

func constMessage(raw string) connectMethod {
	return func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(raw), nil
	}
}

// connectPath returns the dispatch key for a /{pkg}.{Service}/{Method} URL
// path, or "" when p does not have that shape.
func connectPath(p string) string {
	parts := strings.SplitN(strings.TrimPrefix(p, "/"), "/", 3)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || !strings.Contains(parts[0], ".") {
		return ""
	}
	return "/" + parts[0] + "/" + parts[1]
}

func (s *Server) dispatchConnect(w http.ResponseWriter, r *http.Request, path string) {
	h, ok := connectMethods[path]
	switch {
	case !ok:
		writeConnectError(w, http.StatusNotFound, "unknown method")
	case h == nil:
		writeConnectError(w, http.StatusNotFound, fmt.Sprintf("method %s not implemented", path))
	default:
		connectUnary(w, r, h)
	}
}

func connectUnary(w http.ResponseWriter, r *http.Request, h func(ctx context.Context, body json.RawMessage) (json.RawMessage, error)) {
	ct := r.Header.Get("Content-Type")
	if ct != "" && !strings.HasPrefix(ct, "application/json") && !strings.Contains(ct, "+json") {
		writeConnectError(w, http.StatusUnsupportedMediaType, "unsupported content type "+ct)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeConnectError(w, http.StatusBadRequest, "read request body: "+err.Error())
		return
	}
	msg, err := h(r.Context(), json.RawMessage(body))
	if err != nil {
		writeConnectError(w, http.StatusNotFound, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(msg)
}

func writeConnectError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{Code: "unimplemented", Message: message})
}
