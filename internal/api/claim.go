package api

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/ernie/trinity-tracker/internal/auth"
	"github.com/ernie/trinity-tracker/internal/domain"
	"github.com/ernie/trinity-tracker/internal/storage"
)

var validUsernameRegex = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// ClaimValidateRequest is the claim-code check request body.
type ClaimValidateRequest struct {
	Code string `json:"code"`
}

// ClaimValidateResponse describes the player a valid claim code is for.
type ClaimValidateResponse struct {
	CodeID   int64                   `json:"code_id"`
	PlayerID int64                   `json:"player_id"`
	Player   *domain.Player          `json:"player"`
	Stats    *domain.AggregatedStats `json:"stats,omitempty"`
}

func (r *Router) handleClaimValidate(w http.ResponseWriter, req *http.Request) {
	var body ClaimValidateRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "The request couldn't be read.")
		return
	}

	if len(body.Code) != 6 {
		writeError(w, http.StatusBadRequest, "Claim codes are six digits.")
		return
	}

	claimCode, err := r.store.GetValidClaimCode(req.Context(), body.Code)
	if err != nil {
		writeError(w, http.StatusNotFound, "That code is wrong or has expired.")
		return
	}

	player, err := r.store.GetPlayerByID(req.Context(), claimCode.PlayerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't load that player.")
		return
	}

	response := ClaimValidateResponse{
		CodeID:   claimCode.ID,
		PlayerID: claimCode.PlayerID,
		Player:   player,
	}

	if stats, err := r.store.GetPlayerStatsByID(req.Context(), claimCode.PlayerID, "all"); err == nil {
		response.Stats = &stats.Stats
	}

	writeJSON(w, http.StatusOK, response)
}

// ClaimRegisterRequest is the body for creating an account from a claim
// code.
type ClaimRegisterRequest struct {
	Code            string `json:"code"`
	Username        string `json:"username"`
	Password        string `json:"password"`
	ConfirmPassword string `json:"confirm_password"`
	DisplayName     string `json:"display_name"`
}

// handleClaimRegister creates an account that owns the claim code's
// player and logs it in.
func (r *Router) handleClaimRegister(w http.ResponseWriter, req *http.Request) {
	var body ClaimRegisterRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "The request couldn't be read.")
		return
	}

	if body.Username == "" || body.Password == "" {
		writeError(w, http.StatusBadRequest, "Enter a username and password.")
		return
	}

	if len(body.Username) < 2 || len(body.Username) > 16 || !validUsernameRegex.MatchString(body.Username) {
		writeError(w, http.StatusBadRequest, "Usernames must be 2–16 characters: letters, numbers and underscores.")
		return
	}

	if len(body.Password) < 8 {
		writeError(w, http.StatusBadRequest, "Password must be at least 8 characters.")
		return
	}

	if body.Password != body.ConfirmPassword {
		writeError(w, http.StatusBadRequest, "The passwords don't match.")
		return
	}

	var displayName, displayNameCanonical string
	if body.DisplayName != "" {
		displayName = domain.CleanQ3DisplayName(body.DisplayName)
		if displayName == "" {
			writeError(w, http.StatusBadRequest, "Enter a display name.")
			return
		}
		displayNameCanonical = storage.CanonicalizeDisplayName(displayName)
		if displayNameCanonical == "" {
			writeError(w, http.StatusBadRequest, "Display names need some visible text, not just color codes.")
			return
		}
		if r.store.IsNameReserved(req.Context(), displayName, 0) {
			writeError(w, http.StatusConflict, "That display name is reserved or taken.")
			return
		}
	}

	claimCode, err := r.store.GetValidClaimCode(req.Context(), body.Code)
	if err != nil {
		writeError(w, http.StatusNotFound, "That code is wrong or has expired.")
		return
	}

	hash, err := auth.HashPassword(body.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't save the password.")
		return
	}

	// One transaction creates the account and claims the player.
	userID, err := r.store.ClaimRegister(req.Context(), claimCode.ID, claimCode.PlayerID, body.Username, hash, displayName, displayNameCanonical)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			writeError(w, http.StatusConflict, "That username is taken.")
			return
		}
		if strings.Contains(err.Error(), "already linked") {
			writeError(w, http.StatusConflict, "That player is already linked to another account.")
			return
		}
		writeError(w, http.StatusInternalServerError, "Couldn't create your account.")
		return
	}

	// The token needs the new row's token_version.
	user, err := r.store.GetUserByID(req.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Your account was created, but it couldn't be loaded. Log in to continue.")
		return
	}
	token, err := r.auth.GenerateToken(user.ID, user.Username, user.IsAdmin, user.PlayerID, false, user.TokenVersion)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Your account was created. Log in to continue.")
		return
	}

	setSessionCookie(w, req, token)
	writeJSON(w, http.StatusCreated, LoginResponse{
		Username: user.Username,
		IsAdmin:  user.IsAdmin,
		PlayerID: user.PlayerID,
	})
}

// ClaimLinkRequest is the body for linking a claim code's player to the
// signed-in account.
type ClaimLinkRequest struct {
	Code string `json:"code"`
}

// handleClaimLink links a claim code's player to the signed-in account.
func (r *Router) handleClaimLink(w http.ResponseWriter, req *http.Request) {
	claims := r.getAuthClaims(req)
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Log in first.")
		return
	}

	var body ClaimLinkRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "The request couldn't be read.")
		return
	}

	claimCode, err := r.store.GetValidClaimCode(req.Context(), body.Code)
	if err != nil {
		writeError(w, http.StatusNotFound, "That code is wrong or has expired.")
		return
	}

	if err := r.store.ClaimLink(req.Context(), claimCode.ID, claimCode.PlayerID, claims.UserID); err != nil {
		if strings.Contains(err.Error(), "already linked") {
			writeError(w, http.StatusConflict, "That player is already linked to another account.")
			return
		}
		writeError(w, http.StatusInternalServerError, "Couldn't link this player.")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Player linked to your account."})
}
