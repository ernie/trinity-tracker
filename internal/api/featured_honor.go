package api

import (
	"encoding/json"
	"net/http"
)

// validFeaturedHonors mirrors the keys in web/src/constants/honors.ts.
// Stored as TEXT on users.featured_honor; this set is the integrity
// boundary between client-supplied keys and what we'll persist.
//
// Keep in lockstep with the HONORS list in honors.ts. Adding a new
// honor cell on the frontend without listing its key here will cause
// PATCH requests for that key to 400.
var validFeaturedHonors = map[string]bool{
	"victories":          true,
	"excellents":         true,
	"impressives":        true,
	"humiliations":       true,
	"captures":           true,
	"skulls_delivered":   true,
	"obelisks_destroyed": true,
	"flag_returns":       true,
	"assists":            true,
	"defends":            true,
}

// SetFeaturedHonorRequest is the body for PATCH /api/account/featured-honor.
type SetFeaturedHonorRequest struct {
	Key string `json:"key"`
}

// handleSetFeaturedHonor updates the authenticated user's featured honor.
// Returns 204 on success, 400 if the key isn't in the allowed set.
func (r *Router) handleSetFeaturedHonor(w http.ResponseWriter, req *http.Request) {
	claims := r.getAuthClaims(req)
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var body SetFeaturedHonorRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if !validFeaturedHonors[body.Key] {
		writeError(w, http.StatusBadRequest, "unknown featured honor key")
		return
	}

	if err := r.store.SetUserFeaturedHonor(req.Context(), claims.UserID, body.Key); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update featured honor")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
