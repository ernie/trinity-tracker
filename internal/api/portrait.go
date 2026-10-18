package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// portraitPartPattern limits both halves of "model/skin" to names the
// asset extractor produces, which also keeps the file check inside the
// portraits tree.
var portraitPartPattern = regexp.MustCompile(`^[a-z0-9_-]+$`)

// SetPortraitRequest is the body for PATCH /api/account/portrait. A null
// or empty portrait clears the choice.
type SetPortraitRequest struct {
	Portrait *string `json:"portrait"`
}

// handleSetPortrait sets the user's profile icon. Any "model/skin" whose
// extracted icon_<skin>.png exists is allowed.
func (r *Router) handleSetPortrait(w http.ResponseWriter, req *http.Request) {
	claims := r.getAuthClaims(req)
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Log in first.")
		return
	}

	var body SetPortraitRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "The request couldn't be read.")
		return
	}

	value := ""
	if body.Portrait != nil {
		value = strings.ToLower(strings.TrimSpace(*body.Portrait))
	}
	if value != "" {
		model, skin, ok := strings.Cut(value, "/")
		if !ok || !portraitPartPattern.MatchString(model) || !portraitPartPattern.MatchString(skin) {
			writeError(w, http.StatusBadRequest, "Portraits are written as \"model/skin\".")
			return
		}
		iconPath := filepath.Join(r.staticDir, "assets", "portraits", model, "icon_"+skin+".png")
		if _, err := os.Stat(iconPath); err != nil {
			writeError(w, http.StatusBadRequest, "Unknown portrait.")
			return
		}
	}

	if err := r.store.SetUserPortrait(req.Context(), claims.UserID, value); err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't update your profile icon.")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
