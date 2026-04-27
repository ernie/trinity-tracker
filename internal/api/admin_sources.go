package api

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/ernie/trinity-tracker/internal/domain"
)

// handleListPendingSources returns every source awaiting admin approval.
func (r *Router) handleListPendingSources(w http.ResponseWriter, req *http.Request) {
	rows, err := r.store.ListPendingSources(req.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	type entry struct {
		SourceUUID string `json:"source_uuid"`
		Source     string `json:"source"`
		Version    string `json:"version"`
		FirstSeen  string `json:"first_seen"`
		LastSeen   string `json:"last_seen,omitempty"`
		Servers    json.RawMessage `json:"servers"`
	}
	out := make([]entry, 0, len(rows))
	for _, p := range rows {
		e := entry{
			SourceUUID: p.SourceUUID,
			Source:     p.Source,
			Version:    p.Version,
			FirstSeen:  p.FirstSeen.UTC().Format("2006-01-02T15:04:05Z"),
			Servers:    json.RawMessage(p.ServersJSON),
		}
		if !p.LastSeen.IsZero() {
			e.LastSeen = p.LastSeen.UTC().Format("2006-01-02T15:04:05Z")
		}
		out = append(out, e)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// handleApproveSource accepts a pending source, creates its remote
// servers rows, and drains any in-memory DLQ that accumulated while
// it was pending. Request body is optional:
//
//	{ "demo_base_url": "https://example.com" }
//
// path: POST /api/admin/sources/{source_uuid}/approve
func (r *Router) handleApproveSource(w http.ResponseWriter, req *http.Request) {
	uuid := req.PathValue("source_uuid")
	if uuid == "" {
		http.Error(w, "source_uuid path parameter required", http.StatusBadRequest)
		return
	}
	var body struct {
		DemoBaseURL string `json:"demo_base_url,omitempty"`
	}
	if req.ContentLength > 0 {
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	pendings, err := r.store.ListPendingSources(req.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var target *domain.Registration
	for _, p := range pendings {
		if p.SourceUUID == uuid {
			reg := domain.Registration{
				SourceUUID: p.SourceUUID,
				Source:     p.Source,
				Version:    p.Version,
			}
			if len(p.ServersJSON) > 0 {
				_ = json.Unmarshal([]byte(p.ServersJSON), &reg.Servers)
			}
			target = &reg
			break
		}
	}
	if target == nil {
		http.Error(w, "source not pending", http.StatusNotFound)
		return
	}

	if err := r.writer.ApproveSource(req.Context(), *target, body.DemoBaseURL); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// In hub mode, mint a per-source user NKey + JWT. The response
	// carries the .creds file so the operator can hand it to the
	// remote collector in a single approval round-trip.
	if r.userProv != nil {
		creds, err := r.userProv.MintUserCreds(target.Source, target.SourceUUID)
		if err != nil {
			http.Error(w, "mint creds: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", "attachment; filename=\""+target.SourceUUID+".creds\"")
		_, _ = w.Write(creds)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleDownloadSourceCreds streams the current .creds file for an
// approved source. 404 if the file doesn't exist (e.g. source was
// rejected, or a pre-auth-phase hub hasn't minted it yet).
//
// path: GET /api/admin/sources/{source_uuid}/creds
func (r *Router) handleDownloadSourceCreds(w http.ResponseWriter, req *http.Request) {
	if r.userProv == nil {
		http.Error(w, "cred management not enabled on this hub", http.StatusNotImplemented)
		return
	}
	uuid := req.PathValue("source_uuid")
	if uuid == "" {
		http.Error(w, "source_uuid path parameter required", http.StatusBadRequest)
		return
	}
	data, err := os.ReadFile(r.userProv.CredsPath(uuid))
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "creds not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+uuid+".creds\"")
	_, _ = w.Write(data)
}

// handleRotateSourceCreds issues a fresh user NKey for the source,
// revokes the previous one, and returns the new .creds file. 404 if
// the source is unknown to the provisioner (never approved).
//
// path: POST /api/admin/sources/{source_uuid}/rotate-creds
func (r *Router) handleRotateSourceCreds(w http.ResponseWriter, req *http.Request) {
	if r.userProv == nil {
		http.Error(w, "cred management not enabled on this hub", http.StatusNotImplemented)
		return
	}
	uuid := req.PathValue("source_uuid")
	if uuid == "" {
		http.Error(w, "source_uuid path parameter required", http.StatusBadRequest)
		return
	}
	// The sourceID scopes the user's publish permissions
	// (trinity.*.<sourceID>.>). Re-resolve from the first approved
	// servers row for this uuid; without it, permissions wouldn't
	// match the collector's publish subjects and the new creds would
	// be dead on arrival.
	sourceID, err := r.store.LookupSourceIDByUUID(req.Context(), uuid)
	if err != nil {
		http.Error(w, "lookup source: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if sourceID == "" {
		http.Error(w, "source not approved", http.StatusNotFound)
		return
	}
	creds, err := r.userProv.MintUserCreds(sourceID, uuid)
	if err != nil {
		http.Error(w, "mint creds: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+uuid+".creds\"")
	_, _ = w.Write(creds)
}

// handleRejectSource drops a pending source and blocks it in-memory
// until the hub restarts.
//
// path: POST /api/admin/sources/{source_uuid}/reject
func (r *Router) handleRejectSource(w http.ResponseWriter, req *http.Request) {
	uuid := req.PathValue("source_uuid")
	if uuid == "" {
		http.Error(w, "source_uuid path parameter required", http.StatusBadRequest)
		return
	}
	if err := r.writer.RejectSource(uuid); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if r.userProv != nil {
		if err := r.userProv.RevokeSource(uuid); err != nil {
			// Reject already succeeded — log-only on revoke failure
			// so the HTTP response reflects the primary action.
			http.Error(w, "revoked pending source but failed to revoke creds: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
