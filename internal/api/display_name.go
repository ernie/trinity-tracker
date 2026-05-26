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
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var body UpdateDisplayNameRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	cleaned := domain.CleanQ3DisplayName(body.DisplayName)
	if cleaned == "" {
		writeError(w, http.StatusBadRequest, "display name is empty after cleaning")
		return
	}

	newCanonical := storage.CanonicalizeDisplayName(cleaned)
	if newCanonical == "" {
		writeError(w, http.StatusBadRequest, "display name is empty after canonicalization")
		return
	}

	user, err := r.store.GetUserByID(req.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get user")
		return
	}

	existingCanonical := storage.CanonicalizeDisplayName(user.DisplayName)
	if newCanonical != existingCanonical {
		writeError(w, http.StatusUnprocessableEntity, "display name cannot change identity, only colors")
		return
	}

	if err := r.store.SetUserDisplayName(req.Context(), claims.UserID, cleaned); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update display name")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
