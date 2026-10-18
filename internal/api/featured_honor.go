package api

import (
	"encoding/json"
	"net/http"
)

// validFeaturedHonors must list every key in the HONORS list in
// web/src/constants/honors.tsx; a key missing here is rejected with 400.
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

func (r *Router) handleSetFeaturedHonor(w http.ResponseWriter, req *http.Request) {
	claims := r.getAuthClaims(req)
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Log in first.")
		return
	}

	var body SetFeaturedHonorRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "The request couldn't be read.")
		return
	}

	if !validFeaturedHonors[body.Key] {
		writeError(w, http.StatusBadRequest, "Unknown honor.")
		return
	}

	if err := r.store.SetUserFeaturedHonor(req.Context(), claims.UserID, body.Key); err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't update your featured honor.")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
