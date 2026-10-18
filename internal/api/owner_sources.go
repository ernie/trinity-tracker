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

// MaxRequestPurposeLen caps the "What is this for?" field so the admin
// pending list shows it whole.
const MaxRequestPurposeLen = 200

// mySourcesEnvelope lists the caller's sources and whether one of them
// is pending.
type mySourcesEnvelope struct {
	Sources    []mySourceEntry `json:"sources"`
	HasPending bool            `json:"has_pending"`
}

// mySourceEntry is one of the caller's sources, with only the fields
// its status uses.
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

type requestSourceBody struct {
	Name    string `json:"name"`
	Purpose string `json:"purpose"`
}

// handleGetMySources returns every source the caller owns.
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

// handleRequestSource creates a pending source for the caller, or
// reactivates one they left under the same name. A user may have one
// pending request at a time and any number of active sources.
//
// path: POST /api/sources/request
func (r *Router) handleRequestSource(w http.ResponseWriter, req *http.Request) {
	if r.userProv == nil {
		writeError(w, http.StatusNotImplemented, "Credential management isn't enabled on this hub.")
		return
	}
	var body requestSourceBody
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "The request couldn't be read.")
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	body.Purpose = strings.TrimSpace(body.Purpose)

	if err := storage.ValidateSource(body.Name); err != nil {
		writeError(w, http.StatusBadRequest, "Name must be 3–16 characters: letters, numbers, _ or -.")
		return
	}
	// Requested names need at least 3 characters; storage allows 1.
	if len(body.Name) < 3 || len(body.Name) > 16 {
		writeError(w, http.StatusBadRequest, "Name must be 3–16 characters: letters, numbers, _ or -.")
		return
	}
	if len(body.Purpose) > MaxRequestPurposeLen {
		writeError(w, http.StatusBadRequest, "Purpose can be at most 200 characters.")
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
		writeError(w, http.StatusConflict, "You already have a pending request. Wait for an admin to review it before sending another.")
		return
	case errors.Is(err, storage.ErrSourceNameTaken):
		writeError(w, http.StatusConflict, "That source name is taken.")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Requesting a name the caller left approves it automatically, so
	// it needs new credentials and an open ingest gate.
	post, err := r.store.GetSourceByName(req.Context(), body.Name)
	if err == nil && post.Status == "active" && post.OwnerUserID.Valid && post.OwnerUserID.Int64 == claims.UserID {
		if _, mintErr := r.userProv.MintUserCreds(req.Context(), post.Source); mintErr != nil {
			log.Printf("handleRequestSource: rejoin mint creds for %q: %v", post.Source, mintErr)
		}
		r.writer.MarkSourceApproved(post.Source)
		_ = r.store.WriteSourceAudit(req.Context(), post.Source, &claims.UserID, "rejoined", "")
	} else {
		_ = r.store.WriteSourceAudit(req.Context(), body.Name, &claims.UserID, "requested", body.Purpose)
	}
	w.WriteHeader(http.StatusCreated)
}

// handleDownloadMyCreds returns the .creds file for an active source the
// caller owns, and audits the download.
//
// path: GET /api/sources/mine/{source}/creds
func (r *Router) handleDownloadMyCreds(w http.ResponseWriter, req *http.Request) {
	if r.userProv == nil {
		writeError(w, http.StatusNotImplemented, "Credential management isn't enabled on this hub.")
		return
	}
	src, claims, ok := r.requireOwnedActiveSource(w, req)
	if !ok {
		return
	}
	data, err := os.ReadFile(r.userProv.CredsPath(src.Source))
	if err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "This source has no credentials file yet.")
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

// handleRotateMyCreds issues new credentials for the source and returns
// the .creds file, at most 5 times in 24 hours per source.
//
// path: POST /api/sources/mine/{source}/rotate-creds
func (r *Router) handleRotateMyCreds(w http.ResponseWriter, req *http.Request) {
	if r.userProv == nil {
		writeError(w, http.StatusNotImplemented, "Credential management isn't enabled on this hub.")
		return
	}
	src, claims, ok := r.requireOwnedActiveSource(w, req)
	if !ok {
		return
	}
	if !r.rotateLimiter.allow(src.Source) {
		writeError(w, http.StatusTooManyRequests, "You can rotate credentials up to 5 times in 24 hours. Try again later.")
		return
	}
	creds, err := r.userProv.MintUserCreds(req.Context(), src.Source)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't create new credentials: "+err.Error())
		return
	}
	_ = r.store.WriteSourceAudit(req.Context(), src.Source, &claims.UserID, "rotated", "")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+src.Source+".creds\"")
	_, _ = w.Write(creds)
}

// handleLeaveMySource takes one of the caller's sources off the network
// and revokes its credentials. The owner can request the name again.
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
		if err := r.userProv.RevokeSource(req.Context(), src.Source); err != nil {
			log.Printf("handleLeaveMySource: revoke creds for %q: %v", src.Source, err)
		}
	}
	_ = r.store.WriteSourceAudit(req.Context(), src.Source, &claims.UserID, "left", "")
	w.WriteHeader(http.StatusNoContent)
}

// requireOwnedActiveSource returns the {source} row when the caller owns
// it and it is active. Otherwise it writes the error response and
// returns false.
func (r *Router) requireOwnedActiveSource(w http.ResponseWriter, req *http.Request) (*storage.Source, *authClaimsProxy, bool) {
	claims := r.getAuthClaims(req)
	source, ok := sourceFromPath(req)
	if !ok {
		writeError(w, http.StatusBadRequest, "The source name is missing from the request.")
		return nil, nil, false
	}
	src, err := r.store.GetSourceByName(req.Context(), source)
	if err != nil {
		// Every failure is a 404 so non-owners can't probe for names.
		writeError(w, http.StatusNotFound, "No such source.")
		return nil, nil, false
	}
	if !src.OwnerUserID.Valid || src.OwnerUserID.Int64 != claims.UserID {
		writeError(w, http.StatusNotFound, "No such source.")
		return nil, nil, false
	}
	if src.Status != "active" {
		writeError(w, http.StatusForbidden, "This source isn't active.")
		return nil, nil, false
	}
	return src, &authClaimsProxy{UserID: claims.UserID}, true
}

type authClaimsProxy struct{ UserID int64 }
