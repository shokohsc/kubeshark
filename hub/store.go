package hub

import (
	"encoding/json"
	"slices"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
)

// Store is the in-memory hub state.
type Store struct {
	mu          sync.RWMutex
	pods        map[string]*corev1.Pod
	seen        map[string]time.Time
	license     string
	settings    map[string]any
	scripts     map[string]Script
	targets     []string
	ring        *Ring
	clusterInfo json.RawMessage
	logRing     *Ring
}

// defaultRingSize backs stores built without a configured size; cmd/hub/main.go
// stamps Config.RingSize with the same fixed value.
const defaultRingSize = 100000

// Script is a stored hub script as the front CRUD endpoints round-trip it.
type Script struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Language string `json:"language"`
	Code     string `json:"code"`
	Active   bool   `json:"active"`
}

func NewStore() *Store {
	return &Store{
		pods: make(map[string]*corev1.Pod),
		seen: make(map[string]time.Time),
		settings: map[string]any{
			"dissection": true,
		},
		scripts: make(map[string]Script),
		ring:    NewRing(defaultRingSize),
	}
}

// Ring returns the entry ring; never nil.
func (s *Store) Ring() *Ring {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ring
}

// setRingSize swaps in a ring of the given capacity; construction-time only,
// before the store is shared with running handlers.
func (s *Store) setRingSize(n int) {
	if n < 1 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ring = NewRing(n)
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

// Settings returns a copy of the stored settings; defaults to dissection on.
func (s *Store) Settings() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]any, len(s.settings))
	for key, value := range s.settings {
		out[key] = value
	}
	return out
}

func (s *Store) SetSetting(key string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings[key] = value
}

// Scripts returns a snapshot of the stored scripts; never nil, so callers
// encode a JSON array.
func (s *Store) Scripts() []Script {
	s.mu.RLock()
	defer s.mu.RUnlock()
	scripts := make([]Script, 0, len(s.scripts))
	for _, script := range s.scripts {
		scripts = append(scripts, script)
	}
	return scripts
}

func (s *Store) PutScript(script Script) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scripts[script.ID] = script
}

func (s *Store) DeleteScript(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.scripts, id)
}

func (s *Store) SetScriptActive(id string, on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if script, ok := s.scripts[id]; ok {
		script.Active = on
		s.scripts[id] = script
	}
}

// Targets returns a copy of the target hosts; never nil, so callers encode a
// JSON array.
func (s *Store) Targets() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, len(s.targets))
	copy(out, s.targets)
	return out
}

// SetTarget appends host unless it is already targeted (client retries must
// not grow the list).
func (s *Store) SetTarget(host string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !slices.Contains(s.targets, host) {
		s.targets = append(s.targets, host)
	}
}

var _ = struct{}{} // padding

// SetClusterInfo stores the latest cluster info payload; keep-latest only.
// ponytail: cluster info is overwrite-only, no history retention
func (s *Store) SetClusterInfo(info json.RawMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clusterInfo = append(json.RawMessage(nil), info...)
}

// ClusterInfo returns the latest cluster info; may be empty raw JSON.
func (s *Store) ClusterInfo() json.RawMessage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.clusterInfo) == 0 {
		return json.RawMessage(`{}`)
	}
	return append(json.RawMessage(nil), s.clusterInfo...)
}

// LogRing returns a separate bounded ring for script logs (capacity 10000).
// ponytail: log ring fixed at 10000; no config for logs
func (s *Store) LogRing() *Ring {
	if s.ring == nil {
		s.mu.RLock()
		defer s.mu.RUnlock()
	}
	// try to get/create log ring; store keeps its own if present
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.logRing == nil {
		s.logRing = NewRing(10000)
	}
	return s.logRing
}
