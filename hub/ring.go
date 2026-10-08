package hub

import (
	"encoding/json"
	"sync"
)

// Ring is a bounded buffer of the most recent entries with a registry of
// live-feed subscribers. It owns its own mutex, independent of the store's,
// so the store lock is never held while notifying subscribers.
type Ring struct {
	mu     sync.Mutex
	buf    []json.RawMessage
	start  int // index of the oldest entry
	n      int
	subs   map[int]chan json.RawMessage
	nextID int
}

func NewRing(capacity int) *Ring {
	if capacity < 1 {
		capacity = 1 // clamp: a zero-capacity buffer would panic on append
	}
	return &Ring{
		buf:  make([]json.RawMessage, capacity),
		subs: make(map[int]chan json.RawMessage),
	}
}

// Append stores data, evicting the oldest entry when full, and fans the entry
// out to every subscriber.
func (r *Ring) Append(data json.RawMessage) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.n == len(r.buf) {
		r.buf[r.start] = data
		r.start = (r.start + 1) % len(r.buf)
	} else {
		r.buf[(r.start+r.n)%len(r.buf)] = data
		r.n++
	}
	for _, ch := range r.subs {
		select {
		case ch <- data:
		default:
			// ponytail: a slow subscriber drops its own oldest pending entry —
			// Append never blocks and other subscribers are unaffected.
			select {
			case <-ch:
			default:
			}
			ch <- data
		}
	}
}

// Snapshot copies the entries oldest→newest; safe under concurrent appends.
// The result is never nil, so callers encode a JSON array (empty → []).
func (r *Ring) Snapshot() []json.RawMessage {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]json.RawMessage, r.n)
	for i := range out {
		out[i] = r.buf[(r.start+i)%len(r.buf)]
	}
	return out
}

// Subscribe returns a live feed of subsequently appended entries and a cancel
// func that detaches the subscriber and closes the channel; cancel is
// idempotent.
func (r *Ring) Subscribe() (<-chan json.RawMessage, func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id := r.nextID
	r.nextID++
	ch := make(chan json.RawMessage, 64)
	r.subs[id] = ch
	return ch, func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if _, ok := r.subs[id]; ok {
			delete(r.subs, id)
			close(ch)
		}
	}
}
