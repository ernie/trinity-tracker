package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ernie/trinity-tracker/internal/storage"
)

// handleListApprovedSources returns every approved source with its
// servers and last heartbeat. A source whose collector hasn't
// registered yet has no servers.
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
		OwnerUserID     *int64   `json:"owner_user_id,omitempty"`
		OwnerUsername   string   `json:"owner_username,omitempty"`
		Servers         []server `json:"servers"`
	}
	out := make([]entry, 0, len(sources))
	for _, s := range sources {
		e := entry{
			Source:        s.Source,
			Version:       s.Version,
			DemoBaseURL:   s.DemoBaseURL,
			IsRemote:      s.IsRemote,
			Active:        s.Active,
			OwnerUsername: s.OwnerUsername,
			Servers:       make([]server, 0, len(s.Servers)),
		}
		if s.OwnerUserID.Valid {
			id := s.OwnerUserID.Int64
			e.OwnerUserID = &id
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

// handleCreateSource creates a remote source and issues its NATS
// credentials, which the admin downloads from the creds endpoint.
// Body:
//
//	{ "source": "remote-1", "owner_user_id": 17 }
//
// owner_user_id is required: rotate, leave and RCON delegation all
// authorize against the owner.
//
// path: POST /api/admin/sources
func (r *Router) handleCreateSource(w http.ResponseWriter, req *http.Request) {
	if r.userProv == nil {
		http.Error(w, "Credential management isn't enabled on this hub.", http.StatusNotImplemented)
		return
	}
	var body struct {
		Source      string `json:"source"`
		OwnerUserID int64  `json:"owner_user_id"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		http.Error(w, "The request couldn't be read: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := storage.ValidateSource(body.Source); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.OwnerUserID <= 0 {
		http.Error(w, "Pick an owner.", http.StatusBadRequest)
		return
	}
	if _, err := r.store.GetUserByID(req.Context(), body.OwnerUserID); err != nil {
		http.Error(w, "That owner isn't a user on this hub.", http.StatusBadRequest)
		return
	}

	if err := r.store.CreateSource(req.Context(), body.Source, true, &body.OwnerUserID); err != nil {
		http.Error(w, "create source: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := r.userProv.MintUserCreds(req.Context(), body.Source); err != nil {
		// Leave the source inactive; reactivating it issues new
		// credentials.
		_ = r.store.DeactivateSource(req.Context(), body.Source)
		http.Error(w, "mint creds: "+err.Error(), http.StatusInternalServerError)
		return
	}
	r.writer.MarkSourceApproved(body.Source)
	r.auditCredsAccess(req, "create", body.Source)
	w.WriteHeader(http.StatusCreated)
}

// handleTransferSourceOwner gives a remote source a new owner. Body:
//
//	{ "owner_user_id": 17 }
//
// path: POST /api/admin/sources/{source}/owner
func (r *Router) handleTransferSourceOwner(w http.ResponseWriter, req *http.Request) {
	source, ok := sourceFromPath(req)
	if !ok {
		http.Error(w, "The source name is missing from the request.", http.StatusBadRequest)
		return
	}
	var body struct {
		OwnerUserID int64 `json:"owner_user_id"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		http.Error(w, "The request couldn't be read: "+err.Error(), http.StatusBadRequest)
		return
	}
	if body.OwnerUserID <= 0 {
		http.Error(w, "Pick an owner.", http.StatusBadRequest)
		return
	}
	newOwner, err := r.store.GetUserByID(req.Context(), body.OwnerUserID)
	if err != nil {
		http.Error(w, "That owner isn't a user on this hub.", http.StatusBadRequest)
		return
	}
	if err := r.store.TransferSourceOwner(req.Context(), source, body.OwnerUserID); err != nil {
		http.Error(w, "transfer owner: "+err.Error(), http.StatusInternalServerError)
		return
	}
	claims := r.getAuthClaims(req)
	var actor *int64
	if claims != nil {
		uid := claims.UserID
		actor = &uid
	}
	if err := r.store.WriteSourceAudit(req.Context(), source, actor, "owner.transfer", "→ "+newOwner.Username); err != nil {
		log.Printf("admin: source_audit insert failed: %v", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

// sourceFromPath returns the {source} path value, unescaped, if it is a
// valid source name. An invalid name can't exist, so callers 404.
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

// handleDownloadSourceCreds returns a source's current .creds file.
// Local sources use the hub's internal credentials and have none.
//
// path: GET /api/admin/sources/{source}/creds
func (r *Router) handleDownloadSourceCreds(w http.ResponseWriter, req *http.Request) {
	if r.userProv == nil {
		http.Error(w, "Credential management isn't enabled on this hub.", http.StatusNotImplemented)
		return
	}
	source, ok := sourceFromPath(req)
	if !ok {
		http.Error(w, "The source name is missing from the request.", http.StatusBadRequest)
		return
	}
	data, err := os.ReadFile(r.userProv.CredsPath(source))
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "This source has no credentials file. Local sources use the hub's internal credentials.", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// The file holds the private seed, so every download is audited.
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

// handleRotateSourceCreds issues a new user NKey for the source,
// revokes the old one, and returns the new .creds file.
//
// path: POST /api/admin/sources/{source}/rotate-creds
func (r *Router) handleRotateSourceCreds(w http.ResponseWriter, req *http.Request) {
	if r.userProv == nil {
		http.Error(w, "Credential management isn't enabled on this hub.", http.StatusNotImplemented)
		return
	}
	source, ok := sourceFromPath(req)
	if !ok {
		http.Error(w, "The source name is missing from the request.", http.StatusBadRequest)
		return
	}
	known, err := r.store.IsSourceApproved(req.Context(), source)
	if err != nil {
		http.Error(w, "lookup source: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if !known {
		http.Error(w, "Unknown source.", http.StatusNotFound)
		return
	}
	creds, err := r.userProv.MintUserCreds(req.Context(), source)
	if err != nil {
		http.Error(w, "mint creds: "+err.Error(), http.StatusInternalServerError)
		return
	}
	r.auditCredsAccess(req, "rotate", source)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+source+".creds\"")
	_, _ = w.Write(creds)
}

// handleDeactivateSource marks the source and its servers inactive,
// revokes its NATS credentials, and blocks it at ingest. Its rows and
// matches stay.
//
// path: POST /api/admin/sources/{source}/deactivate
func (r *Router) handleDeactivateSource(w http.ResponseWriter, req *http.Request) {
	source, ok := sourceFromPath(req)
	if !ok {
		http.Error(w, "The source name is missing from the request.", http.StatusBadRequest)
		return
	}
	if err := r.writer.DeactivateSource(req.Context(), source); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// 'revoked' keeps the owner from rejoining under this name.
	if err := r.store.RevokeSourceStatus(req.Context(), source); err != nil {
		log.Printf("admin deactivate: RevokeSourceStatus(%q): %v", source, err)
	}
	warning := ""
	if r.userProv != nil {
		if err := r.userProv.RevokeSource(req.Context(), source); err != nil {
			warning = "Source deactivated, but its credentials couldn't be revoked: " + err.Error()
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
// makes the source and its servers active again.
//
// path: POST /api/admin/sources/{source}/reactivate
func (r *Router) handleReactivateSource(w http.ResponseWriter, req *http.Request) {
	source, ok := sourceFromPath(req)
	if !ok {
		http.Error(w, "The source name is missing from the request.", http.StatusBadRequest)
		return
	}
	if err := r.writer.ReactivateSource(req.Context(), source); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if claims := r.getAuthClaims(req); claims != nil {
		_ = r.store.WriteSourceAudit(req.Context(), source, &claims.UserID, "reactivated", "")
	}
	r.auditCredsAccess(req, "reactivate", source)
	w.WriteHeader(http.StatusNoContent)
}

// pendingSourceRow is one pending request in Admin → Sources.
type pendingSourceRow struct {
	Source           string `json:"source"`
	OwnerUserID      int64  `json:"owner_user_id"`
	OwnerUsername    string `json:"owner_username"`
	RequestedPurpose string `json:"requested_purpose"`
	SubmittedAt      string `json:"submitted_at"`
}

// handleListPendingSources returns every pending source request with
// the requester's username.
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

// handleApproveSource makes a pending source active, issues its
// credentials, and opens ingest to it.
//
// path: POST /api/admin/sources/{source}/approve
func (r *Router) handleApproveSource(w http.ResponseWriter, req *http.Request) {
	if r.userProv == nil {
		writeError(w, http.StatusNotImplemented, "Credential management isn't enabled on this hub.")
		return
	}
	source, ok := sourceFromPath(req)
	if !ok {
		writeError(w, http.StatusBadRequest, "The source name is missing from the request.")
		return
	}
	if err := r.store.ApproveSource(req.Context(), source); err != nil {
		if errors.Is(err, storage.ErrSourceNotPending) {
			writeError(w, http.StatusConflict, "This source isn't pending.")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := r.userProv.MintUserCreds(req.Context(), source); err != nil {
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

// rejectSourceBody carries the reason the requester sees.
type rejectSourceBody struct {
	Reason string `json:"reason"`
}

type renameSourceBody struct {
	Name string `json:"name"`
}

// handleRenamePendingSource renames a source before approval. Once
// active, the name is baked into its NATS permissions and .creds file.
//
// path: POST /api/admin/sources/{source}/rename
func (r *Router) handleRenamePendingSource(w http.ResponseWriter, req *http.Request) {
	source, ok := sourceFromPath(req)
	if !ok {
		writeError(w, http.StatusBadRequest, "The source name is missing from the request.")
		return
	}
	var body renameSourceBody
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "The request couldn't be read.")
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if err := r.store.RenamePendingSource(req.Context(), source, body.Name); err != nil {
		switch {
		case errors.Is(err, storage.ErrSourceNotPending):
			writeError(w, http.StatusConflict, "Only pending sources can be renamed.")
			return
		case errors.Is(err, storage.ErrSourceNameTaken):
			writeError(w, http.StatusConflict, "That name is taken.")
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

// auditEntryJSON is one audit row. System actions have no actor.
type auditEntryJSON struct {
	ID            int64  `json:"id"`
	Source        string `json:"source"`
	ActorUserID   *int64 `json:"actor_user_id,omitempty"`
	ActorUsername string `json:"actor_username,omitempty"`
	Action        string `json:"action"`
	Detail        string `json:"detail,omitempty"`
	CreatedAt     string `json:"created_at"`
}

// handleListAudit returns the audit log, newest first. Optional
// filters: source, actor and action (exact matches), since (RFC 3339)
// and limit (default 100, at most 500).
//
// path: GET /api/admin/audit
func (r *Router) handleListAudit(w http.ResponseWriter, req *http.Request) {
	q := req.URL.Query()
	filters := storage.AuditFilters{
		Source: strings.TrimSpace(q.Get("source")),
		Actor:  strings.TrimSpace(q.Get("actor")),
		Action: strings.TrimSpace(q.Get("action")),
	}
	if since := strings.TrimSpace(q.Get("since")); since != "" {
		t, err := time.Parse(time.RFC3339, since)
		if err != nil {
			writeError(w, http.StatusBadRequest, "since must be RFC 3339, such as 2026-04-01T00:00:00Z.")
			return
		}
		filters.Since = t
	}
	if lim := strings.TrimSpace(q.Get("limit")); lim != "" {
		n, err := strconv.Atoi(lim)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer.")
			return
		}
		filters.Limit = n
	}

	rows, err := r.store.ListAllAudit(req.Context(), filters)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]auditEntryJSON, 0, len(rows))
	for _, e := range rows {
		entry := auditEntryJSON{
			ID:            e.ID,
			Source:        e.Source,
			ActorUsername: e.ActorUsername,
			Action:        e.Action,
			Detail:        e.Detail,
			CreatedAt:     e.CreatedAt.UTC().Format(time.RFC3339),
		}
		if e.ActorUserID.Valid {
			id := e.ActorUserID.Int64
			entry.ActorUserID = &id
		}
		out = append(out, entry)
	}
	writeJSON(w, http.StatusOK, out)
}

// handleRejectSource rejects a pending request with a reason, which the
// requester sees in My servers.
//
// path: POST /api/admin/sources/{source}/reject
func (r *Router) handleRejectSource(w http.ResponseWriter, req *http.Request) {
	source, ok := sourceFromPath(req)
	if !ok {
		writeError(w, http.StatusBadRequest, "The source name is missing from the request.")
		return
	}
	var body rejectSourceBody
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "The request couldn't be read.")
		return
	}
	if strings.TrimSpace(body.Reason) == "" {
		writeError(w, http.StatusBadRequest, "Give a reason.")
		return
	}
	if err := r.store.RejectSource(req.Context(), source, body.Reason); err != nil {
		if errors.Is(err, storage.ErrSourceNotPending) {
			writeError(w, http.StatusConflict, "This source isn't pending.")
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
