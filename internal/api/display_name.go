package api

import (
	"encoding/json"
	"net/http"

	"github.com/ernie/trinity-tracker/internal/domain"
	"github.com/ernie/trinity-tracker/internal/storage"
)

type UpdateDisplayNameRequest struct {
	DisplayName string `json:"display_name"`
}

func (r *Router) handleUpdateDisplayName(w http.ResponseWriter, req *http.Request) {
	claims := r.getAuthClaims(req)
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Log in first.")
		return
	}

	var body UpdateDisplayNameRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "The request couldn't be read.")
		return
	}

	cleaned := domain.CleanQ3DisplayName(body.DisplayName)
	if cleaned == "" {
		writeError(w, http.StatusBadRequest, "Enter a display name.")
		return
	}

	newCanonical := storage.CanonicalizeDisplayName(cleaned)
	if newCanonical == "" {
		writeError(w, http.StatusBadRequest, "Display names need some visible text, not just color codes.")
		return
	}

	user, err := r.store.GetUserByID(req.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't load your account.")
		return
	}

	existingCanonical := storage.CanonicalizeDisplayName(user.DisplayName)
	if newCanonical != existingCanonical {
		writeError(w, http.StatusUnprocessableEntity, "You can change colors and capitalization, but not the name itself.")
		return
	}

	if err := r.store.SetUserDisplayName(req.Context(), claims.UserID, cleaned); err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't update your display name.")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
