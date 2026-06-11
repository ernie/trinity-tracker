package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// portraitPartPattern bounds both halves of a "model/skin" value to names
// the asset extractor can produce, which also keeps the os.Stat below
// inside the portraits tree.
var portraitPartPattern = regexp.MustCompile(`^[a-z0-9_-]+$`)

// SetPortraitRequest is the body for PATCH /api/account/portrait.
// Null or empty portrait clears the choice.
type SetPortraitRequest struct {
	Portrait *string `json:"portrait"`
}

// handleSetPortrait updates the authenticated user's chosen profile icon.
// The extracted portrait files on disk are the whitelist: any "model/skin"
// whose icon_<skin>.png exists is allowed. Returns 204 on success.
func (r *Router) handleSetPortrait(w http.ResponseWriter, req *http.Request) {
	claims := r.getAuthClaims(req)
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var body SetPortraitRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	value := ""
	if body.Portrait != nil {
		value = strings.ToLower(strings.TrimSpace(*body.Portrait))
	}
	if value != "" {
		model, skin, ok := strings.Cut(value, "/")
		if !ok || !portraitPartPattern.MatchString(model) || !portraitPartPattern.MatchString(skin) {
			writeError(w, http.StatusBadRequest, "portrait must be \"model/skin\"")
			return
		}
		iconPath := filepath.Join(r.staticDir, "assets", "portraits", model, "icon_"+skin+".png")
		if _, err := os.Stat(iconPath); err != nil {
			writeError(w, http.StatusBadRequest, "unknown portrait")
			return
		}
	}

	if err := r.store.SetUserPortrait(req.Context(), claims.UserID, value); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update portrait")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
