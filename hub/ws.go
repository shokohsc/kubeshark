package hub

import (
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ponytail: origin check disabled — front is same-origin behind its nginx anyway
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// ponytail: query tokens accepted but treated as authenticated only if AUTH_ENABLED=false, header auth unchanged
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	ctx := r.Context()
	readDone := make(chan struct{}, 1)

	// ponytail: no server-side KFL evaluation, front filters client-side
	// Read first message (KFL) and discard; do not block streaming
	// Start read loop concurrently to detect close and to consume first message
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer conn.Close()
		defer func() {
			select {
			case readDone <- struct{}{}:
			default:
			}
		}()
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		// Reset read deadline on pong
		conn.SetPongHandler(func(string) error {
			conn.SetReadDeadline(time.Now().Add(60 * time.Second))
			return nil
		})
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
			// ignore client payloads, just reset read deadline
			conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		}
	}()

	ring := s.store.Ring()
	ch, cancel := ring.Subscribe()
	defer cancel()

	// Send immediate handshake heartbeat to ensure subscription is active
	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"heartbeat"}`)); err != nil {
		conn.Close()
		wg.Wait()
		return
	}
	// Also send ping for keepalive
	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_ = conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(5*time.Second))

	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			conn.Close()
			wg.Wait()
			return
		case <-readDone:
			conn.Close()
			wg.Wait()
			return
		case <-heartbeat.C:
			// send heartbeat as text message and ping
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"heartbeat"}`)); err != nil {
				conn.Close()
				wg.Wait()
				return
			}
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			_ = conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(5*time.Second))
		case entry, more := <-ch:
			if !more {
				conn.Close()
				wg.Wait()
				return
			}
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(websocket.TextMessage, entry); err != nil {
				conn.Close()
				wg.Wait()
				return
			}
		}
	}
}
