package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/ernie/trinity-tracker/internal/storage"
)

// MaxRequestPurposeLen caps the free-text "what is this for?" field
// to a length the admin pending-list can render inline without
// truncation.
const MaxRequestPurposeLen = 200

// mySourcesEnvelope is the JSON shape returned by GET /api/sources/mine.
// One entry per source the caller owns; an empty list means "no source
// row at all" → button shows "Add Servers". A pending entry in the
// list (HasPending also true) means the button shows "Request Pending".
type mySourcesEnvelope struct {
	Sources    []mySourceEntry `json:"sources"`
	HasPending bool            `json:"has_pending"`
}

// mySourceEntry is one source the caller owns. Only fields relevant
// to the source's current status are populated; absent fields are
// omitted on the wire.
type mySourceEntry struct {
	Source          string        `json:"source"`
	Status          string        `json:"status"`
	Purpose         string        `json:"purpose,omitempty"`
	RejectionReason string        `json:"rejection_reason,omitempty"`
	Version         string        `json:"version,omitempty"`
	DemoBaseURL     string        `json:"demo_base_url,omitempty"`
	LastHeartbeatAt string        `json:"last_heartbeat_at,omitempty"`
	Servers         []serverEntry `json:"servers,omitempty"`
}

type serverEntry struct {
	Key     string `json:"key"`
	Address string `json:"address"`
	Active  bool   `json:"active"`
}

// requestSourceBody is what the request modal POSTs.
type requestSourceBody struct {
	Name    string `json:"name"`
	Purpose string `json:"purpose"`
}

// handleGetMySources returns the caller's full list of sources. The
// React MySourceButton drives its label off this: empty → "Add
// Servers", any pending → "Request Pending", otherwise → "My Servers".
//
// path: GET /api/sources/mine
func (r *Router) handleGetMySources(w http.ResponseWriter, req *http.Request) {
	claims := r.getAuthClaims(req)
	rows, err := r.store.ListSourcesByOwner(req.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	env := mySourcesEnvelope{Sources: make([]mySourceEntry, 0, len(rows))}
	for _, src := range rows {
		entry := mySourceEntry{
			Source:          src.Source,
			Status:          src.Status,
			Purpose:         src.RequestedPurpose,
			RejectionReason: src.RejectionReason,
		}
		if src.Status == "pending" {
			env.HasPending = true
		}
		if src.Status == "active" {
			entry.Version = src.Version
			entry.DemoBaseURL = src.DemoBaseURL
			if !src.LastHeartbeatAt.IsZero() {
				entry.LastHeartbeatAt = src.LastHeartbeatAt.UTC().Format("2006-01-02T15:04:05Z")
			}
			servers, err := r.store.ListServersForSource(req.Context(), src.Source)
			if err != nil {
				log.Printf("handleGetMySources: list servers for %q: %v", src.Source, err)
			}
			for _, s := range servers {
				entry.Servers = append(entry.Servers, serverEntry{
					Key: s.Key, Address: s.Address, Active: s.Active,
				})
			}
		}
		env.Sources = append(env.Sources, entry)
	}
	writeJSON(w, http.StatusOK, env)
}

// handleRequestSource creates a pending source for the caller (or
// re-activates their 'left' row of the same name). One pending at a
// time per user; multiple actives are allowed (one source per host
// is the recommended pattern). The handler does the cred mint inline
// only on the rejoin path.
//
// path: POST /api/sources/request
func (r *Router) handleRequestSource(w http.ResponseWriter, req *http.Request) {
	if r.userProv == nil {
		writeError(w, http.StatusNotImplemented, "cred management not enabled on this hub")
		return
	}
	var body requestSourceBody
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	body.Purpose = strings.TrimSpace(body.Purpose)

	if err := storage.ValidateSource(body.Name); err != nil {
		writeError(w, http.StatusBadRequest, "invalid name: "+err.Error())
		return
	}
	// User-facing tighter bound than the storage layer's 64-char ceiling:
	// the request modal enforces 3–32 to keep names short and memorable.
	if len(body.Name) < 3 || len(body.Name) > 32 {
		writeError(w, http.StatusBadRequest, "name must be 3-32 characters")
		return
	}
	if len(body.Purpose) > MaxRequestPurposeLen {
		writeError(w, http.StatusBadRequest, "purpose exceeds 200 characters")
		return
	}

	claims := r.getAuthClaims(req)
	err := r.store.CreateSourceRequest(req.Context(), storage.CreateSourceRequestArgs{
		Source:           body.Name,
		OwnerUserID:      claims.UserID,
		RequestedPurpose: body.Purpose,
	})
	switch {
	case errors.Is(err, storage.ErrPendingRequestExists):
		writeError(w, http.StatusConflict, "you already have a pending request — wait for an admin to review it before submitting another")
		return
	case errors.Is(err, storage.ErrSourceNameTaken):
		writeError(w, http.StatusConflict, "source name is taken")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Rejoin path: storage layer auto-approved a 'left' row, so the
	// row is now 'active'. Re-mint creds and prime the ingest cache.
	post, err := r.store.GetSourceByName(req.Context(), body.Name)
	if err == nil && post.Status == "active" && post.OwnerUserID.Valid && post.OwnerUserID.Int64 == claims.UserID {
		if _, mintErr := r.userProv.MintUserCreds(post.Source); mintErr != nil {
			log.Printf("handleRequestSource: rejoin mint creds for %q: %v", post.Source, mintErr)
		}
		r.writer.MarkSourceApproved(post.Source)
		_ = r.store.WriteSourceAudit(req.Context(), post.Source, &claims.UserID, "rejoined", "")
	} else {
		_ = r.store.WriteSourceAudit(req.Context(), body.Name, &claims.UserID, "requested", body.Purpose)
	}
	w.WriteHeader(http.StatusCreated)
}

// handleDownloadMyCreds streams the .creds file for the named source,
// gated on caller ownership + status='active'. Audited per download.
//
// path: GET /api/sources/mine/{source}/creds
func (r *Router) handleDownloadMyCreds(w http.ResponseWriter, req *http.Request) {
	if r.userProv == nil {
		writeError(w, http.StatusNotImplemented, "cred management not enabled on this hub")
		return
	}
	src, claims, ok := r.requireOwnedActiveSource(w, req)
	if !ok {
		return
	}
	data, err := os.ReadFile(r.userProv.CredsPath(src.Source))
	if err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "no creds on disk for this source")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = r.store.WriteSourceAudit(req.Context(), src.Source, &claims.UserID, "downloaded", "")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+src.Source+".creds\"")
	_, _ = w.Write(data)
}

// handleRotateMyCreds mints a fresh user JWT for the named source,
// rate-limited to 5/24h per source. Same wire shape as the admin
// rotate path.
//
// path: POST /api/sources/mine/{source}/rotate-creds
func (r *Router) handleRotateMyCreds(w http.ResponseWriter, req *http.Request) {
	if r.userProv == nil {
		writeError(w, http.StatusNotImplemented, "cred management not enabled on this hub")
		return
	}
	src, claims, ok := r.requireOwnedActiveSource(w, req)
	if !ok {
		return
	}
	if !r.rotateLimiter.allow(src.Source) {
		writeError(w, http.StatusTooManyRequests, "rotation limit exceeded (5 per 24h)")
		return
	}
	creds, err := r.userProv.MintUserCreds(src.Source)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "mint: "+err.Error())
		return
	}
	_ = r.store.WriteSourceAudit(req.Context(), src.Source, &claims.UserID, "rotated", "")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+src.Source+".creds\"")
	_, _ = w.Write(creds)
}

// handleLeaveMySource is the soft owner-self-service exit for a
// single named source. Other sources owned by the caller are
// untouched.
//
// path: POST /api/sources/mine/{source}/leave
func (r *Router) handleLeaveMySource(w http.ResponseWriter, req *http.Request) {
	src, claims, ok := r.requireOwnedActiveSource(w, req)
	if !ok {
		return
	}
	if err := r.writer.LeaveSource(req.Context(), src.Source); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if r.userProv != nil {
		if err := r.userProv.RevokeSource(src.Source); err != nil {
			log.Printf("handleLeaveMySource: revoke creds for %q: %v", src.Source, err)
		}
	}
	_ = r.store.WriteSourceAudit(req.Context(), src.Source, &claims.UserID, "left", "")
	w.WriteHeader(http.StatusNoContent)
}

// requireOwnedActiveSource is the shared gate for the three
// owner-action handlers. Pulls {source} from the path, looks the row
// up, and verifies (a) caller is authenticated, (b) source exists,
// (c) status='active', (d) caller owns it. On not-ok it has already
// written an error response.
func (r *Router) requireOwnedActiveSource(w http.ResponseWriter, req *http.Request) (*storage.Source, *authClaimsProxy, bool) {
	claims := r.getAuthClaims(req)
	source, ok := sourceFromPath(req)
	if !ok {
		writeError(w, http.StatusBadRequest, "source path parameter required")
		return nil, nil, false
	}
	src, err := r.store.GetSourceByName(req.Context(), source)
	if err != nil {
		// Either ErrNoRows or a real error — both go to 404 from the
		// caller's perspective; revealing existence to non-owners would
		// leak source names.
		writeError(w, http.StatusNotFound, "no such source")
		return nil, nil, false
	}
	if !src.OwnerUserID.Valid || src.OwnerUserID.Int64 != claims.UserID {
		writeError(w, http.StatusNotFound, "no such source")
		return nil, nil, false
	}
	if src.Status != "active" {
		writeError(w, http.StatusForbidden, "source is not active")
		return nil, nil, false
	}
	return src, &authClaimsProxy{UserID: claims.UserID}, true
}

// authClaimsProxy carries just the bits the audit writer needs from
// a Claims pointer — keeps the audit signature using *int64 without
// the handler having to take the address of a struct field.
type authClaimsProxy struct{ UserID int64 }
