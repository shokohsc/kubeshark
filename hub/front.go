package hub

import (
	"encoding/json"
	"net/http"
)

type whoamiResponse struct {
	Authenticated   bool `json:"authenticated"`
	User            any  `json:"user"`
	Capabilities    any  `json:"capabilities"`
	AuthzNamespaces any  `json:"authzNamespaces"`
}

type authSessionResponse struct {
	Authenticated bool `json:"authenticated"`
	User          any  `json:"user"`
}

// handleWhoami serves GET /whoami. Field names are pinned by the front
// bundle's parse (o?.capabilities ?? null, o?.authzNamespaces, o?.user):
// user is the verified service-account subject from the auth middleware, or
// null when auth is off or the credential was a License-Key.
// ponytail: capabilities and authzNamespaces stay null — no RBAC mapping exists in this build.
func (s *Server) handleWhoami(w http.ResponseWriter, r *http.Request) {
	var user any
	if subject, ok := r.Context().Value(subjectKey{}).(string); ok && subject != "" {
		user = subject
	}
	writeJSON(w, whoamiResponse{Authenticated: s.cfg.AuthEnabled, User: user})
}

// handleAuthSession serves GET /auth/session.
// ponytail: session user is always null — identity is served by /whoami only.
func (s *Server) handleAuthSession(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, authSessionResponse{Authenticated: s.cfg.AuthEnabled})
}

func (s *Server) handleMetadataVersion(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"version": s.cfg.Version})
}

func (s *Server) handleSettingsStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.store.Settings())
}

func (s *Server) handleGetDissection(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"enabled": s.dissectionEnabled()})
}

func (s *Server) handlePostDissection(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.store.SetSetting("dissection", req.Enabled)
	writeJSON(w, map[string]any{"enabled": req.Enabled})
}

func (s *Server) dissectionEnabled() bool {
	if enabled, ok := s.store.Settings()["dissection"].(bool); ok {
		return enabled
	}
	return true
}

func (s *Server) handleGetScripts(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.store.Scripts())
}

func (s *Server) handlePostScript(w http.ResponseWriter, r *http.Request) {
	var script Script
	if err := json.NewDecoder(r.Body).Decode(&script); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.store.PutScript(script)
	writeJSON(w, script)
}

func (s *Server) handlePutScript(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.hasScript(id) {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}
	var script Script
	if err := json.NewDecoder(r.Body).Decode(&script); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	script.ID = id // the path id is authoritative over the body
	s.store.PutScript(script)
	writeJSON(w, script)
}

func (s *Server) handleDeleteScript(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.hasScript(id) {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}
	s.store.DeleteScript(id)
	writeOK(w)
}

func (s *Server) handleScriptActivate(w http.ResponseWriter, r *http.Request) {
	s.setScriptActive(w, r, true)
}

func (s *Server) handleScriptDeactivate(w http.ResponseWriter, r *http.Request) {
	s.setScriptActive(w, r, false)
}

func (s *Server) setScriptActive(w http.ResponseWriter, r *http.Request, on bool) {
	id := r.PathValue("id")
	if !s.hasScript(id) {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}
	s.store.SetScriptActive(id, on)
	writeOK(w)
}

// ponytail: scripts are stored, listed and toggled but never executed — this
// build has no script runtime; the endpoint only acknowledges the body.
func (s *Server) handleScriptExec(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"status": "ok"})
}

func (s *Server) hasScript(id string) bool {
	for _, script := range s.store.Scripts() {
		if script.ID == id {
			return true
		}
	}
	return false
}

func (s *Server) handleGetLicense(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"license": s.store.License()})
}

// ponytail: push/remove store the key verbatim with a best-effort shape — no
// validation, entitlement or limit logic exists anywhere in the hub (spec §13).
func (s *Server) handleLicensePush(w http.ResponseWriter, r *http.Request) {
	var req struct {
		License string `json:"license"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.store.SetLicense(req.License)
	writeJSON(w, map[string]any{"status": "pushed"})
}

func (s *Server) handleLicenseRemove(w http.ResponseWriter, _ *http.Request) {
	s.store.SetLicense("")
	writeJSON(w, map[string]any{"status": "removed"})
}

// ponytail: validate always reports valid and filter always matches nothing —
// KFL parsing and L7 matching live in the CLI and data plane, not this build.
func (s *Server) handleQueryValidate(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"valid": true})
}

func (s *Server) handleQueryFilters(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, []string{})
}

func (s *Server) handleQueryFilter(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"matched": 0})
}

func (s *Server) handlePostPodsTarget(w http.ResponseWriter, r *http.Request) {
	s.store.SetTarget(r.PathValue("host"))
	writeOK(w)
}

func (s *Server) handlePodsTargetSettings(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"targets": s.store.Targets()})
}

// handleFlows2 serves GET /flows2 — the ring's entries, oldest first.
// ponytail: the aggregate query parameter is ignored — no server-side
// aggregation; the front filters client-side.
func (s *Server) handleFlows2(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.store.Ring().Snapshot())
}

// ponytail: cloud-record browsing is not implemented; the body matches the
// only observed bundle parse guard s?.Contents → [].
func (s *Server) handleFetchRecords(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"Contents": []any{}})
}

func (s *Server) handleFetchRecordsListNodes(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, []string{})
}

func (s *Server) handleFetchRecordsGetScript(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, "")
}

// ponytail: record ingestion is not implemented — POST /records only
// acknowledges so the front moves on (bundle checks status 200 only).
func (s *Server) handlePostRecords(w http.ResponseWriter, _ *http.Request) {
	writeOK(w)
}

// ponytail: nothing is ever deleted because nothing is stored — the bundle
// checks status 200 only.
func (s *Server) handleDeleteRecordsBulk(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"deleted": 0})
}

// ponytail: accept-and-discard — the Sentry event sink and OAuth2 logout have
// no backend in this build; both answer 200 so clients move on.
func (s *Server) handleSentryFront(w http.ResponseWriter, _ *http.Request) {
	writeOK(w)
}

func (s *Server) handleOAuth2Logout(w http.ResponseWriter, _ *http.Request) {
	writeOK(w)
}
