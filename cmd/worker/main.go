package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// workerPod is the minimal corev1.Pod projection the hub's POST /pods/worker
// handler and /health aggregations read: metadata.name, spec.nodeName and
// status.podIP.
type workerPod struct {
	Metadata struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Spec struct {
		NodeName string `json:"nodeName"`
	} `json:"spec"`
	Status struct {
		PodIP string `json:"podIP"`
	} `json:"status"`
}

// worker beacons the hub Service's cluster FQDN (defaultHubURL) so a real data
// plane can be added later; today it registers the pod and answers probes.
type worker struct {
	hubURL string
	token  string
	pod    workerPod
	client *http.Client
}

const beaconInterval = 30 * time.Second

// defaultHubURL is the hub Service's cluster FQDN; override per namespace with
// HUB_URL or -hub-url.
func defaultHubURL() string {
	if v := os.Getenv("HUB_URL"); v != "" {
		return v
	}
	return "http://kubeshark-hub.default.svc.cluster.local"
}

func main() {
	port := flag.Int("port", 48999, "port to serve the daemonset readiness/liveness probes on")
	metricsPort := flag.Int("metrics-port", 49100, "port to serve metrics on")
	hubURL := flag.String("hub-url", defaultHubURL(), "hub base URL (FQDN) to report to")
	flag.Parse()

	w := &worker{
		hubURL: *hubURL,
		token:  tokenFromEnv("HUB_INTERNAL_TOKEN_PATH"),
		pod:    workerPodFromEnv(),
		client: &http.Client{Timeout: 5 * time.Second},
	}

	health := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	metrics := http.HandlerFunc(w.serveMetrics)
	go func() { log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *metricsPort), metrics)) }()
	go func() { log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), health)) }()

	log.Printf("kubeshark worker reporting to hub %s", w.hubURL)
	go w.run()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, os.Interrupt)
	<-stop
}

func (w *worker) run() {
	for {
		if err := w.register(); err != nil {
			log.Printf("hub %s: %v", w.hubURL, err)
		}
		time.Sleep(beaconInterval)
	}
}

// register reports the worker pod to the hub and returns whether the hub
// accepted it; a bearer token is forwarded when the hub is auth-gated.
func (w *worker) register() error {
	body, err := json.Marshal(w.pod)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, w.hubURL+"/pods/worker", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if w.token != "" {
		req.Header.Set("Authorization", "Bearer "+w.token)
	}
	resp, err := w.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("hub reported worker pod: status %s", resp.Status)
	}
	return nil
}

func (w *worker) serveMetrics(rw http.ResponseWriter, _ *http.Request) {
	connected := 0
	if err := w.register(); err == nil {
		connected = 1
	}
	rw.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintf(rw, "# TYPE kubeshark_worker_connected gauge\nkubeshark_worker_connected %d\n", connected)
}

// workerPodFromEnv builds the pod projection from the downward-API env vars the
// daemonset already sets (POD_NAME, POD_NAMESPACE, NODE_NAME, POD_IP).
func workerPodFromEnv() (p workerPod) {
	p.Metadata.Name = os.Getenv("POD_NAME")
	p.Metadata.Namespace = os.Getenv("POD_NAMESPACE")
	p.Spec.NodeName = os.Getenv("NODE_NAME")
	p.Status.PodIP = os.Getenv("POD_IP")
	return p
}

// tokenFromEnv reads a hub service-account token from the file named by envVar;
// empty when the variable is unset or unreadable.
func tokenFromEnv(envVar string) string {
	path := os.Getenv(envVar)
	if path == "" {
		return ""
	}
	b, err := os.ReadFile(path)
	if err != nil {
		log.Printf("reading %s: %v", path, err)
		return ""
	}
	return strings.TrimSpace(string(b))
}