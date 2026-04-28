package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/ernie/trinity-tracker/internal/storage"
)

// handleListApprovedSources returns every provisioned source, grouped
// by source, with its server roster and last heartbeat for UI health
// indicators. Empty Servers slice = source exists but its collector
// has not yet registered.
//
// path: GET /api/admin/sources
func (r *Router) handleListApprovedSources(w http.ResponseWriter, req *http.Request) {
	sources, err := r.store.ListApprovedSources(req.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	type server struct {
		ID      int64  `json:"id"`
		LocalID int64  `json:"local_id"`
		Key     string `json:"key"`
		Address string `json:"address"`
		Active  bool   `json:"active"`
	}
	type entry struct {
		Source          string   `json:"source"`
		Version         string   `json:"version,omitempty"`
		DemoBaseURL     string   `json:"demo_base_url,omitempty"`
		LastHeartbeatAt string   `json:"last_heartbeat_at,omitempty"`
		IsRemote        bool     `json:"is_remote"`
		Active          bool     `json:"active"`
		Servers         []server `json:"servers"`
	}
	out := make([]entry, 0, len(sources))
	for _, s := range sources {
		e := entry{
			Source:      s.Source,
			Version:     s.Version,
			DemoBaseURL: s.DemoBaseURL,
			IsRemote:    s.IsRemote,
			Active:      s.Active,
			Servers:     make([]server, 0, len(s.Servers)),
		}
		if !s.LastHeartbeatAt.IsZero() {
			e.LastHeartbeatAt = s.LastHeartbeatAt.UTC().Format("2006-01-02T15:04:05Z")
		}
		for _, srv := range s.Servers {
			e.Servers = append(e.Servers, server{
				ID:      srv.ID,
				LocalID: srv.LocalID,
				Key:     srv.Key,
				Address: srv.Address,
				Active:  srv.Active,
			})
		}
		out = append(out, e)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// handleCreateSource provisions a new remote source: inserts the
// sources row and mints NATS creds scoped to the admin-chosen source.
// The .creds file is persisted on disk; the admin pulls it via the
// separate GET /api/admin/sources/{source}/creds endpoint. The
// collector's demo_base_url is operator-owned and arrives via
// registration heartbeats — not an admin input here. Body:
//
//	{ "source": "remote-1" }
//
// path: POST /api/admin/sources
func (r *Router) handleCreateSource(w http.ResponseWriter, req *http.Request) {
	if r.userProv == nil {
		http.Error(w, "cred management not enabled on this hub", http.StatusNotImplemented)
		return
	}
	var body struct {
		Source string `json:"source"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := storage.ValidateSource(body.Source); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := r.store.CreateSource(req.Context(), body.Source, true); err != nil {
		http.Error(w, "create source: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := r.userProv.MintUserCreds(body.Source); err != nil {
		// Creds minting failed — roll back by deactivating so the
		// admin can retry without hitting a "source already exists"
		// (CreateSource is strict; reactivation is a separate path).
		_ = r.store.DeactivateSource(req.Context(), body.Source)
		http.Error(w, "mint creds: "+err.Error(), http.StatusInternalServerError)
		return
	}
	r.writer.MarkSourceApproved(body.Source)
	r.auditCredsAccess(req, "create", body.Source)
	w.WriteHeader(http.StatusCreated)
}

// sourceFromPath pulls {source} out of the URL and validates it. The
// request path arrives URL-decoded, so no extra work is needed beyond
// confirming it matches the NATS-safe pattern — a malformed source
// couldn't have been created through POST /api/admin/sources anyway,
// so 404 is the right answer.
func sourceFromPath(req *http.Request) (string, bool) {
	raw := req.PathValue("source")
	if raw == "" {
		return "", false
	}
	if s, err := url.PathUnescape(raw); err == nil {
		raw = s
	}
	if err := storage.ValidateSource(raw); err != nil {
		return "", false
	}
	return raw, true
}

// handleDownloadSourceCreds streams the current .creds file for a
// provisioned source. 404 if the file doesn't exist (local sources
// use hub-internal creds and don't have a .creds on disk).
//
// path: GET /api/admin/sources/{source}/creds
func (r *Router) handleDownloadSourceCreds(w http.ResponseWriter, req *http.Request) {
	if r.userProv == nil {
		http.Error(w, "cred management not enabled on this hub", http.StatusNotImplemented)
		return
	}
	source, ok := sourceFromPath(req)
	if !ok {
		http.Error(w, "source path parameter required", http.StatusBadRequest)
		return
	}
	data, err := os.ReadFile(r.userProv.CredsPath(source))
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "no creds for this source (local sources use hub-internal creds)", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Seed bytes leave the hub on every successful call: audit every
	// download so a stolen admin JWT can't silently exfil credentials.
	r.auditCredsAccess(req, "download", source)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+source+".creds\"")
	_, _ = w.Write(data)
}

func (r *Router) auditCredsAccess(req *http.Request, action, source string) {
	claims := r.getAuthClaims(req)
	actor := "unknown"
	var userID int64
	if claims != nil {
		actor = claims.Username
		userID = claims.UserID
	}
	log.Printf("audit: source_creds %s source=%q actor=%s user_id=%d remote=%s",
		action, source, actor, userID, req.RemoteAddr)
}

// handleRotateSourceCreds issues a fresh user NKey for the source,
// revokes the previous one, and returns the new .creds file. 404 if
// the source is unknown.
//
// path: POST /api/admin/sources/{source}/rotate-creds
func (r *Router) handleRotateSourceCreds(w http.ResponseWriter, req *http.Request) {
	if r.userProv == nil {
		http.Error(w, "cred management not enabled on this hub", http.StatusNotImplemented)
		return
	}
	source, ok := sourceFromPath(req)
	if !ok {
		http.Error(w, "source path parameter required", http.StatusBadRequest)
		return
	}
	known, err := r.store.IsSourceApproved(req.Context(), source)
	if err != nil {
		http.Error(w, "lookup source: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if !known {
		http.Error(w, "unknown source", http.StatusNotFound)
		return
	}
	creds, err := r.userProv.MintUserCreds(source)
	if err != nil {
		http.Error(w, "mint creds: "+err.Error(), http.StatusInternalServerError)
		return
	}
	r.auditCredsAccess(req, "rotate", source)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+source+".creds\"")
	_, _ = w.Write(creds)
}

// handleDeactivateSource flips the source + its servers to active=0,
// revokes the source's NATS creds, and updates the in-memory cache so
// any in-flight message is refused. Rows stay in the DB; historical
// matches keep working, the UI dims them.
//
// path: POST /api/admin/sources/{source}/deactivate
func (r *Router) handleDeactivateSource(w http.ResponseWriter, req *http.Request) {
	source, ok := sourceFromPath(req)
	if !ok {
		http.Error(w, "source path parameter required", http.StatusBadRequest)
		return
	}
	if err := r.writer.DeactivateSource(req.Context(), source); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Mirror the active=0 in the status enum so the user-facing flow
	// agrees with the admin's punitive intent: 'revoked' specifically
	// means "owner cannot self-rejoin," distinct from 'left'.
	if err := r.store.RevokeSourceStatus(req.Context(), source); err != nil {
		log.Printf("admin deactivate: RevokeSourceStatus(%q): %v", source, err)
	}
	warning := ""
	if r.userProv != nil {
		if err := r.userProv.RevokeSource(source); err != nil {
			warning = "source deactivated but creds revocation failed: " + err.Error()
			log.Printf("admin deactivate: revoke creds for %q failed: %v", source, err)
		}
	}
	if claims := r.getAuthClaims(req); claims != nil {
		_ = r.store.WriteSourceAudit(req.Context(), source, &claims.UserID, "revoked", "")
	}
	r.auditCredsAccess(req, "deactivate", source)
	w.Header().Set("Content-Type", "application/json")
	if warning != "" {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"warning": warning})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleReactivateSource is the inverse of handleDeactivateSource: it
// flips both flags back to 1 and re-mints the source's NATS creds so
// the operator gets a fresh download link in the response.
//
// path: POST /api/admin/sources/{source}/reactivate
func (r *Router) handleReactivateSource(w http.ResponseWriter, req *http.Request) {
	source, ok := sourceFromPath(req)
	if !ok {
		http.Error(w, "source path parameter required", http.StatusBadRequest)
		return
	}
	if err := r.writer.ReactivateSource(req.Context(), source); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	r.auditCredsAccess(req, "reactivate", source)
	w.WriteHeader(http.StatusNoContent)
}

// pendingSourceRow is the JSON shape rendered in Admin → Sources →
// Pending Requests. Joined to users so the row carries the requester's
// username; the spec opted to omit email (the users table has no email
// column). The collector's URL isn't asked of the requester (it
// arrives via heartbeat once the collector connects), so it's not
// part of the admin's review surface.
type pendingSourceRow struct {
	Source           string `json:"source"`
	OwnerUserID      int64  `json:"owner_user_id"`
	OwnerUsername    string `json:"owner_username"`
	RequestedPurpose string `json:"requested_purpose"`
	SubmittedAt      string `json:"submitted_at"`
}

// handleListPendingSources returns every status='pending' source,
// joined to users for the requester's username. Drives both the
// Sources nav badge and the Pending Requests section at the top of
// the admin Sources page.
//
// path: GET /api/admin/sources/pending
func (r *Router) handleListPendingSources(w http.ResponseWriter, req *http.Request) {
	rows, err := r.store.ListPendingRequests(req.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]pendingSourceRow, 0, len(rows))
	for _, p := range rows {
		entry := pendingSourceRow{
			Source:           p.Source,
			OwnerUserID:      p.OwnerUserID,
			OwnerUsername:    p.OwnerUsername,
			RequestedPurpose: p.RequestedPurpose,
		}
		if !p.StatusChangedAt.IsZero() {
			entry.SubmittedAt = p.StatusChangedAt.UTC().Format("2006-01-02T15:04:05Z")
		}
		out = append(out, entry)
	}
	writeJSON(w, http.StatusOK, out)
}

// handleApproveSource is the admin one-click "yes" on a pending
// request. Flips the row to 'active' (active=1), mints initial creds,
// primes the in-memory ingest cache. The owner's button transitions
// to "My Servers" on the next /api/sources/mine poll (~30s).
//
// path: POST /api/admin/sources/{source}/approve
func (r *Router) handleApproveSource(w http.ResponseWriter, req *http.Request) {
	if r.userProv == nil {
		writeError(w, http.StatusNotImplemented, "cred management not enabled on this hub")
		return
	}
	source, ok := sourceFromPath(req)
	if !ok {
		writeError(w, http.StatusBadRequest, "source path parameter required")
		return
	}
	if err := r.store.ApproveSource(req.Context(), source); err != nil {
		if errors.Is(err, storage.ErrSourceNotPending) {
			writeError(w, http.StatusConflict, "source is not pending")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := r.userProv.MintUserCreds(source); err != nil {
		writeError(w, http.StatusInternalServerError, "mint creds: "+err.Error())
		return
	}
	r.writer.MarkSourceApproved(source)
	if claims := r.getAuthClaims(req); claims != nil {
		_ = r.store.WriteSourceAudit(req.Context(), source, &claims.UserID, "approved", "")
	}
	r.auditCredsAccess(req, "approve", source)
	w.WriteHeader(http.StatusNoContent)
}

// rejectSourceBody is the admin reject form payload — a free-text
// reason that's surfaced verbatim to the requester next time they
// open the request modal.
type rejectSourceBody struct {
	Reason string `json:"reason"`
}

// renameSourceBody is the admin rename payload.
type renameSourceBody struct {
	Name string `json:"name"`
}

// handleRenamePendingSource lets an admin clean up a source name
// before approval. Allowed only while status='pending' — once a
// source is active the name is locked into NATS scope and the
// running collector's .creds, so renaming would require coordinated
// re-credentialing (out of scope for v1).
//
// path: POST /api/admin/sources/{source}/rename
func (r *Router) handleRenamePendingSource(w http.ResponseWriter, req *http.Request) {
	source, ok := sourceFromPath(req)
	if !ok {
		writeError(w, http.StatusBadRequest, "source path parameter required")
		return
	}
	var body renameSourceBody
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if err := r.store.RenamePendingSource(req.Context(), source, body.Name); err != nil {
		switch {
		case errors.Is(err, storage.ErrSourceNotPending):
			writeError(w, http.StatusConflict, "source is not pending — only pending sources can be renamed")
			return
		case errors.Is(err, storage.ErrSourceNameTaken):
			writeError(w, http.StatusConflict, "name is taken")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if claims := r.getAuthClaims(req); claims != nil {
		_ = r.store.WriteSourceAudit(req.Context(), body.Name, &claims.UserID, "renamed", "from "+source)
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleRejectSource sets status='rejected' with a non-empty reason.
// The requester's button stays "Add Servers"; clicking re-opens the
// modal pre-populated with the rejection reason at the top.
//
// path: POST /api/admin/sources/{source}/reject
func (r *Router) handleRejectSource(w http.ResponseWriter, req *http.Request) {
	source, ok := sourceFromPath(req)
	if !ok {
		writeError(w, http.StatusBadRequest, "source path parameter required")
		return
	}
	var body rejectSourceBody
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(body.Reason) == "" {
		writeError(w, http.StatusBadRequest, "reason is required")
		return
	}
	if err := r.store.RejectSource(req.Context(), source, body.Reason); err != nil {
		if errors.Is(err, storage.ErrSourceNotPending) {
			writeError(w, http.StatusConflict, "source is not pending")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if claims := r.getAuthClaims(req); claims != nil {
		_ = r.store.WriteSourceAudit(req.Context(), source, &claims.UserID, "rejected", body.Reason)
	}
	r.auditCredsAccess(req, "reject", source)
	w.WriteHeader(http.StatusNoContent)
}
