package hub

import "sync"

// Store is the in-memory hub state. Skeleton only in this task; later tasks extend it.
type Store struct {
	// ponytail: no state to guard yet — Task 3 adds the first methods that use this mutex.
	mu sync.Mutex //nolint:unused
}

func NewStore() *Store {
	return &Store{}
}
