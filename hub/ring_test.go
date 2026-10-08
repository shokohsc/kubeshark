package hub

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestRingSnapshotOrder(t *testing.T) {
	r := NewRing(10)
	for i := 1; i <= 3; i++ {
		r.Append(json.RawMessage(fmt.Sprintf(`"e%d"`, i)))
	}

	got := r.Snapshot()
	if len(got) != 3 {
		t.Fatalf("Snapshot len = %d, want 3", len(got))
	}
	for i, want := range []string{`"e1"`, `"e2"`, `"e3"`} {
		if string(got[i]) != want {
			t.Errorf("Snapshot[%d] = %s, want %s", i, got[i], want)
		}
	}
}

func TestRingEvictsOldest(t *testing.T) {
	r := NewRing(10)
	for i := 1; i <= 25; i++ {
		r.Append(json.RawMessage(fmt.Sprintf(`"e%d"`, i)))
	}

	got := r.Snapshot()
	if len(got) != 10 {
		t.Fatalf("Snapshot len = %d, want 10 (bounded at capacity)", len(got))
	}
	if first := string(got[0]); first != `"e16"` {
		t.Errorf("first entry = %s, want \"e16\" (oldest evicted)", first)
	}
	if last := string(got[9]); last != `"e25"` {
		t.Errorf("last entry = %s, want \"e25\"", last)
	}
}

func TestRingSubscribeReceivesLive(t *testing.T) {
	r := NewRing(10)
	ch, cancel := r.Subscribe()
	defer cancel()

	r.Append(json.RawMessage(`"live"`))

	select {
	case got := <-ch:
		if string(got) != `"live"` {
			t.Errorf("received %s, want \"live\"", got)
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber did not receive entry appended after Subscribe")
	}
}

func TestRingConcurrentAppendSnapshot(t *testing.T) {
	r := NewRing(1000)

	var wg sync.WaitGroup
	for g := 0; g < 100; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				r.Append(json.RawMessage(fmt.Sprintf(`{"g":%d,"i":%d}`, g, i)))
				if i%10 == 0 {
					_ = r.Snapshot()
				}
			}
		}(g)
	}
	wg.Wait()

	if got := len(r.Snapshot()); got != 1000 {
		t.Errorf("Snapshot len after 5000 appends to capacity 1000 = %d, want 1000", got)
	}
}
