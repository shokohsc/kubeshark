package hub

import (
	"encoding/json"
	"net/http"
	"time"
)

type Config struct {
	AuthEnabled bool
	// ServiceAccounts allowlists verified subjects as bare "<ns>:<name>"; the
	// full "system:serviceaccount:<ns>:<name>" form from TokenReview also matches.
	ServiceAccounts []string
	License         string
	RingSize        int
	LogLevel        string
	Version         string
}

type Server struct {
	cfg      Config
	store    *Store
	verifier TokenVerifier
	launch   time.Time
}

// NewServer builds the hub server. verifier may be nil only when
// !cfg.AuthEnabled; with auth enabled a nil verifier fails closed (all
// requests 401).
func NewServer(cfg Config, store *Store, verifier TokenVerifier) *Server {
	store.setRingSize(cfg.RingSize)
	return &Server{cfg: cfg, store: store, verifier: verifier, launch: time.Now()}
}

// Handler returns the auth-wrapped mux; routes registered on the mux are
// covered by the auth middleware.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/{$}", s.handleRoot)
	mux.HandleFunc("/echo", s.handleEcho)
	mux.HandleFunc("POST /pods/worker", s.handlePostWorkerPod)
	mux.HandleFunc("POST /license", s.handlePostLicense)
	mux.HandleFunc("POST /pcaps/merge", s.handlePcapsMerge)
	mux.HandleFunc("GET /worker", s.handleGetWorker)
	mux.HandleFunc("GET /mcp", s.handleMCPInfo)
	mux.HandleFunc("POST /mcp/tools/call", s.handleMCPCallTool)
	mux.HandleFunc("GET /health/hub", s.handleHealthHub)
	mux.HandleFunc("GET /health/workers", s.handleHealthWorkers)
	mux.HandleFunc("POST /networkpolicies/ensure-blocked-pod", s.handleEnsureBlockedPod)
	mux.HandleFunc("GET /whoami", s.handleWhoami)
	mux.HandleFunc("GET /auth/session", s.handleAuthSession)
	mux.HandleFunc("GET /metadata/version", s.handleMetadataVersion)
	mux.HandleFunc("GET /settings/status", s.handleSettingsStatus)
	mux.HandleFunc("GET /settings/dissection", s.handleGetDissection)
	mux.HandleFunc("POST /settings/dissection", s.handlePostDissection)
	mux.HandleFunc("GET /scripts", s.handleGetScripts)
	mux.HandleFunc("POST /scripts", s.handlePostScript)
	mux.HandleFunc("POST /scripts/exec", s.handleScriptExec)
	mux.HandleFunc("PUT /scripts/{id}", s.handlePutScript)
	mux.HandleFunc("DELETE /scripts/{id}", s.handleDeleteScript)
	mux.HandleFunc("POST /scripts/{id}/activate", s.handleScriptActivate)
	mux.HandleFunc("POST /scripts/{id}/deactivate", s.handleScriptDeactivate)
	mux.HandleFunc("GET /license", s.handleGetLicense)
	mux.HandleFunc("POST /license/push", s.handleLicensePush)
	mux.HandleFunc("POST /license/remove", s.handleLicenseRemove)
	mux.HandleFunc("GET /query/validate", s.handleQueryValidate)
	mux.HandleFunc("GET /query/filters", s.handleQueryFilters)
	mux.HandleFunc("POST /query/filter", s.handleQueryFilter)
	mux.HandleFunc("POST /pods/target/{host}", s.handlePostPodsTarget)
	mux.HandleFunc("GET /pods/target/settings", s.handlePodsTargetSettings)
	mux.HandleFunc("POST /sentry/front", s.handleSentryFront)
	mux.HandleFunc("POST /oauth2/logout", s.handleOAuth2Logout)
	mux.HandleFunc("GET /flows2", s.handleFlows2)
	mux.HandleFunc("GET /fetch-records", s.handleFetchRecords)
	mux.HandleFunc("POST /fetch-records", s.handleFetchRecords)
	mux.HandleFunc("GET /fetch-records/cloud/buckets", s.handleFetchRecords)
	mux.HandleFunc("GET /fetch-records/list/nodes", s.handleFetchRecordsListNodes)
	mux.HandleFunc("POST /fetch-records/list/get-script", s.handleFetchRecordsGetScript)
	mux.HandleFunc("POST /records", s.handlePostRecords)
	mux.HandleFunc("DELETE /records/bulk", s.handleDeleteRecordsBulk)
	mux.HandleFunc("GET /ws", s.handleWS)
	mux.HandleFunc("GET /wsnd", s.handleWS)
	mux.HandleFunc("GET /wsFull", s.handleWS)
	mux.Handle("/debug/pprof/", http.DefaultServeMux)
	// Connect unary RPCs are dispatched before the mux: Go 1.22 patterns cannot
	// match /{pkg}.{Service}/{Method} (one wildcard per segment), and a
	// POST /{path...} catch-all conflicts with the method-less /debug/pprof/
	// pattern. No existing route has a dotted first segment to shadow.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			if path := connectPath(r.URL.Path); path != "" {
				s.dispatchConnect(w, r, path)
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
	return s.requireAuth(handler)
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Service string `json:"service"`
		Version string `json:"version"`
	}{Service: "kubeshark-hub", Version: s.cfg.Version})
}

func (s *Server) handleEcho(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
