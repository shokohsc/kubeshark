package hub

import (
	"encoding/json"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
)

type HealthHubWorker struct {
	Addr    string `json:"addr"`
	PodName string `json:"podName"`
}

type HealthHubNode struct {
	NodeName string `json:"nodeName"`
	PodCount int    `json:"podCount"`
}

type HealthStorage struct {
	DiskUsed      uint64 `json:"diskUsed"`
	DiskAvailable uint64 `json:"diskAvailable"`
	// ponytail: diverges from kubeshark/api — the key is omitted at zero to satisfy the health endpoints' no-limit-keys contract.
	DiskLimit uint64 `json:"diskLimit,omitempty"`
}

// HealthHub mirrors the public kubeshark/api health.go JSON shape with local
// structs; that module must not be imported (its go.mod pulls a private dep).
type HealthHub struct {
	Workers              []HealthHubWorker           `json:"workers"`
	Nodes                []HealthHubNode             `json:"nodes"`
	NodeName             string                      `json:"nodeName"`
	ClusterID            string                      `json:"clusterID"`
	Version              string                      `json:"version"`
	Timestamp            time.Time                   `json:"timestamp"`
	CPUUsage             float64                     `json:"cpuUsage"`
	MemoryUsage          float64                     `json:"memoryUsage"`
	LastRestartReason    string                      `json:"lastRestartReason"`
	LastRestartTimestamp string                      `json:"lastRestartTimestamp"`
	Resources            corev1.ResourceRequirements `json:"resources"`
	Restarts             int                         `json:"restarts"`
	Storage              HealthStorage               `json:"storage"`
}

type WorkerStatus struct {
	Addr     string    `json:"addr"`
	PodName  string    `json:"podName"`
	NodeName string    `json:"nodeName"`
	LastSeen time.Time `json:"lastSeen"`
}

type WorkersHealth struct {
	Workers []WorkerStatus `json:"workers"`
}

// groupByNode sorts pods by namespace/name and groups them by spec.nodeName,
// returning a deterministic node order and each node's pods (first = smallest
// namespace/name).
func groupByNode(pods []*corev1.Pod) ([]string, map[string][]*corev1.Pod) {
	sort.Slice(pods, func(i, j int) bool {
		if pods[i].Namespace != pods[j].Namespace {
			return pods[i].Namespace < pods[j].Namespace
		}
		return pods[i].Name < pods[j].Name
	})
	order := make([]string, 0, len(pods))
	byNode := make(map[string][]*corev1.Pod, len(pods))
	for _, pod := range pods {
		node := pod.Spec.NodeName
		if _, ok := byNode[node]; !ok {
			order = append(order, node)
		}
		byNode[node] = append(byNode[node], pod)
	}
	return order, byNode
}

// HealthHub aggregates live pods: one entry per distinct spec.nodeName, one
// worker per node (addr = first pod's IP). Slices are never nil, so the JSON
// is [] rather than null on an empty store.
func (s *Store) HealthHub(version string) HealthHub {
	order, byNode := groupByNode(s.Pods())

	hub := HealthHub{
		Workers: make([]HealthHubWorker, 0, len(order)),
		Nodes:   make([]HealthHubNode, 0, len(order)),
		Version: version,
	}
	// ponytail: pod and node counts are reported to callers, never enforced —
	// no caps or quota checks exist anywhere in the hub.
	for _, node := range order {
		onNode := byNode[node]
		hub.Nodes = append(hub.Nodes, HealthHubNode{NodeName: node, PodCount: len(onNode)})
		hub.Workers = append(hub.Workers, HealthHubWorker{Addr: onNode[0].Status.PodIP, PodName: onNode[0].Name})
	}
	return hub
}

// WorkersHealth lists one status per node: the first pod's addr and podName,
// and the most recent time any pod on that node was seen by the store.
func (s *Store) WorkersHealth() WorkersHealth {
	s.mu.RLock()
	defer s.mu.RUnlock()

	order, byNode := groupByNode(s.podsLocked())
	health := WorkersHealth{Workers: make([]WorkerStatus, 0, len(order))}
	for _, node := range order {
		onNode := byNode[node]
		ws := WorkerStatus{Addr: onNode[0].Status.PodIP, PodName: onNode[0].Name, NodeName: node}
		for _, pod := range onNode {
			if seen := s.seen[pod.Namespace+"/"+pod.Name]; seen.After(ws.LastSeen) {
				ws.LastSeen = seen
			}
		}
		health.Workers = append(health.Workers, ws)
	}
	return health
}

func (s *Server) handleHealthHub(w http.ResponseWriter, _ *http.Request) {
	hub := s.store.HealthHub(s.cfg.Version)
	hub.Timestamp = time.Now().UTC()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(hub)
}

func (s *Server) handleHealthWorkers(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.store.WorkersHealth())
}

// ponytail: the spec §7 policy removal is manifests-only; the endpoint keeps
// answering 200 {} so older clients never see an error.
func (s *Server) handleEnsureBlockedPod(w http.ResponseWriter, _ *http.Request) {
	writeOK(w)
}
