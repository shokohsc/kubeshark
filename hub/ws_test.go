package hub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func dialWS(t *testing.T, server *httptest.Server, path string, subprotocols ...string) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	dialer := &websocket.Dialer{
		Subprotocols:     subprotocols,
		HandshakeTimeout: 5 * time.Second,
	}
	url := strings.Replace(server.URL, "http://", "ws://", 1) + path
	conn, resp, err := dialer.Dial(url, nil)
	return conn, resp, err
}

func TestWSEchoUpgrade(t *testing.T) {
	store := NewStore()
	h := NewServer(Config{}, store, nil).Handler()
	srv := httptest.NewServer(h)
	defer srv.Close()

	conn, _, err := dialWS(t, srv, "/ws")
	if err != nil {
		t.Fatalf("dial /ws: %v", err)
	}
	defer conn.Close()

	// Read immediate handshake heartbeat first
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read handshake: %v", err)
	}
	if string(msg) != `{"type":"heartbeat"}` {
		t.Fatalf("handshake msg = %s, want {\"type\":\"heartbeat\"}", msg)
	}

	// Send KFL as text
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`req = "x"`)); err != nil {
		t.Fatalf("write kfl: %v", err)
	}

	// Append an entry to ring
	entry := json.RawMessage(`{"test": "data"}`)
	store.Ring().Append(entry)

	// Read the entry back
	var readErr error
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, msg, readErr = conn.ReadMessage()
	if readErr != nil {
		t.Fatalf("read message: %v", readErr)
	}
	if string(msg) != string(entry) {
		t.Fatalf("msg = %s, want %s", msg, entry)
	}
}

func TestWSNDAliasWorks(t *testing.T) {
	h := NewServer(Config{}, NewStore(), nil).Handler()
	srv := httptest.NewServer(h)
	defer srv.Close()

	conn, _, err := dialWS(t, srv, "/wsnd")
	if err != nil {
		t.Fatalf("dial /wsnd: %v", err)
	}
	conn.Close()
}

func TestWSFullAliasWorks(t *testing.T) {
	h := NewServer(Config{}, NewStore(), nil).Handler()
	srv := httptest.NewServer(h)
	defer srv.Close()

	conn, _, err := dialWS(t, srv, "/wsFull")
	if err != nil {
		t.Fatalf("dial /wsFull: %v", err)
	}
	conn.Close()
}

func TestWSWithClientSubprotocol(t *testing.T) {
	h := NewServer(Config{}, NewStore(), nil).Handler()
	srv := httptest.NewServer(h)
	defer srv.Close()

	conn, _, err := dialWS(t, srv, "/ws", "kubeshark")
	if err != nil {
		t.Fatalf("dial /ws with subprotocol: %v", err)
	}
	conn.Close()
}

func TestWSAuthRejected(t *testing.T) {
	cfg := Config{AuthEnabled: true, ServiceAccounts: []string{"ns:kubeshark-cli"}}
	h := NewServer(cfg, NewStore(), &stubVerifier{subject: "ns:kubeshark-cli"}).Handler()
	srv := httptest.NewServer(h)
	defer srv.Close()

	_, resp, err := dialWS(t, srv, "/ws")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if resp == nil {
		t.Fatal("expected non-nil response for bad handshake")
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestWSReceivesEntriesWithoutFirstMessage(t *testing.T) {
	store := NewStore()
	h := NewServer(Config{}, store, nil).Handler()
	srv := httptest.NewServer(h)
	defer srv.Close()

	conn, _, err := dialWS(t, srv, "/ws")
	if err != nil {
		t.Fatalf("dial /ws: %v", err)
	}
	defer conn.Close()

	// Read immediate handshake heartbeat first
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read handshake: %v", err)
	}
	if string(msg) != `{"type":"heartbeat"}` {
		t.Fatalf("handshake msg = %s, want {\"type\":\"heartbeat\"}", msg)
	}

	// Do NOT send first message (no KFL) - append entry and read
	entry := json.RawMessage(`{"no": "kfl", "v": 1}`)
	store.Ring().Append(entry)

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, msg, readErr := conn.ReadMessage()
	if readErr != nil {
		t.Fatalf("read message: %v", readErr)
	}
	if string(msg) != string(entry) {
		t.Fatalf("msg = %s, want %s", msg, entry)
	}
}
