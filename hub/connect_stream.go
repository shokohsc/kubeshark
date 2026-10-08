package hub

import (
	"bufio"
	"encoding/json"
	"net/http"
	"time"
)

const (
	connectJSONCT = "application/connect+json"
)

type connectEnvelope struct {
	Type  string           `json:"type"`
	Value *json.RawMessage `json:"value,omitempty"`
}

func writeConnectEnd(w http.ResponseWriter) error {
	w.Header().Set("Content-Type", connectJSONCT)
	w.WriteHeader(http.StatusOK)
	b, _ := json.Marshal(struct {
		Type string `json:"type"`
	}{Type: "end"})
	_, err := w.Write(append(b, '\n'))
	return err
}

func (s *Server) streamEndOnly(w http.ResponseWriter, r *http.Request) {
	_ = writeConnectEnd(w)
}

func (s *Server) streamClusterInfo(w http.ResponseWriter, r *http.Request) {
	scanner := bufio.NewScanner(r.Body)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	scanner.Split(bufio.ScanLines)
	var last json.RawMessage
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var env connectEnvelope
		if err := json.Unmarshal(line, &env); err != nil {
			continue
		}
		if env.Type == "end" {
			break
		}
		if env.Type == "message" && env.Value != nil {
			last = append(json.RawMessage(nil), *env.Value...)
		}
	}
	if len(last) > 0 {
		s.store.SetClusterInfo(last)
	}
	_ = writeConnectEnd(w)
}

func (s *Server) streamCaptureBaseEntries(w http.ResponseWriter, r *http.Request) {
	ring := s.store.Ring()
	scanner := bufio.NewScanner(r.Body)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var env connectEnvelope
		if err := json.Unmarshal(line, &env); err != nil {
			continue
		}
		if env.Type == "end" {
			break
		}
		if env.Type == "message" && env.Value != nil {
			ring.Append(append(json.RawMessage(nil), *env.Value...))
		}
	}
	_ = writeConnectEnd(w)
}

func (s *Server) streamScriptLogsWorker(w http.ResponseWriter, r *http.Request) {
	logRing := s.store.LogRing()
	scanner := bufio.NewScanner(r.Body)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var env connectEnvelope
		if err := json.Unmarshal(line, &env); err != nil {
			continue
		}
		if env.Type == "end" {
			break
		}
		if env.Type == "message" && env.Value != nil {
			logRing.Append(append(json.RawMessage(nil), *env.Value...))
		}
	}
	_ = writeConnectEnd(w)
}

func (s *Server) streamRegisterClient(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	flusher, ok := w.(http.Flusher)
	w.Header().Set("Content-Type", connectJSONCT)
	w.WriteHeader(http.StatusOK)

	heartbeat := json.RawMessage(`{"heartbeat":{}}`)
	hv := heartbeat
	env := struct {
		Type  string           `json:"type"`
		Value *json.RawMessage `json:"value,omitempty"`
	}{Type: "message", Value: &hv}
	if b, err := json.Marshal(env); err == nil {
		_, _ = w.Write(append(b, '\n'))
		if ok {
			flusher.Flush()
		}
	}

	ring := s.store.Ring()
	ch, cancel := ring.Subscribe()
	defer cancel()

	heartbeatTicker := time.NewTicker(10 * time.Second)
	defer heartbeatTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeatTicker.C:
			heartbeat := json.RawMessage(`{"heartbeat":{}}`)
			hv := heartbeat
			env := struct {
				Type  string           `json:"type"`
				Value *json.RawMessage `json:"value,omitempty"`
			}{Type: "message", Value: &hv}
			if b, err := json.Marshal(env); err == nil {
				_, _ = w.Write(append(b, '\n'))
				if ok {
					flusher.Flush()
				}
			}
		case entry, more := <-ch:
			if !more {
				return
			}
			v := entry
			env := struct {
				Type  string           `json:"type"`
				Value *json.RawMessage `json:"value,omitempty"`
			}{Type: "message", Value: &v}
			if b, err := json.Marshal(env); err == nil {
				_, _ = w.Write(append(b, '\n'))
				if ok {
					flusher.Flush()
				}
			}
		}
	}
}

func (s *Server) handleConnectStreamClientPost(path string) http.HandlerFunc {
	switch path {
	case "/capture.Capture/StreamClusterInfo":
		return s.streamClusterInfo
	case "/capture.Capture/CaptureBaseEntries":
		return s.streamCaptureBaseEntries
	case "/script_logs.ScriptLogsWorker/StreamLogs":
		return s.streamScriptLogsWorker
	case "/capture.UIEventService/RegisterClient":
		return s.streamRegisterClient
	case "/snapshot.SnapshotData/GetFiles":
		return s.streamEndOnly
	case "/script_logs.ScriptLogsDashboard/StreamLogs":
		return s.streamEndOnly
	case "/base_entries_database.BaseEntriesDatabaseService/FetchBaseEntries":
		return s.streamEndOnly
	case "/base_entries_database.BaseEntriesDatabaseService/FetchSelectedBaseEntries":
		return s.streamEndOnly
	default:
		return func(w http.ResponseWriter, r *http.Request) {
			writeConnectError(w, http.StatusNotFound, "unknown method")
		}
	}
}
