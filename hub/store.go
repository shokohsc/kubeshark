package hub

import (
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
)

// Store is the in-memory hub state.
type Store struct {
	mu      sync.RWMutex
	pods    map[string]*corev1.Pod
	seen    map[string]time.Time
	license string
}

func NewStore() *Store {
	return &Store{
		pods: make(map[string]*corev1.Pod),
		seen: make(map[string]time.Time),
	}
}

// UpsertPod stores the pod keyed by namespace/name and records when it was seen.
func (s *Store) UpsertPod(pod *corev1.Pod) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := pod.Namespace + "/" + pod.Name
	s.pods[key] = pod
	s.seen[key] = time.Now()
}

// Pods returns the stored pods; never nil, so callers encode a JSON array.
func (s *Store) Pods() []*corev1.Pod {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.podsLocked()
}

// podsLocked returns a snapshot of the pods; callers must hold at least a read lock.
func (s *Store) podsLocked() []*corev1.Pod {
	pods := make([]*corev1.Pod, 0, len(s.pods))
	for _, pod := range s.pods {
		pods = append(pods, pod)
	}
	return pods
}

// SetLicense stores the license key verbatim; no validation, no limits.
func (s *Store) SetLicense(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.license = key
}

func (s *Store) License() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.license
}
