package api

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/ernie/trinity-tools/internal/auth"
	"github.com/ernie/trinity-tools/internal/domain"
)

var validUsernameRegex = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// ClaimValidateRequest is the request body for claim code validation
type ClaimValidateRequest struct {
	Code string `json:"code"`
}

// ClaimValidateResponse is the response for a valid claim code
type ClaimValidateResponse struct {
	CodeID   int64                      `json:"code_id"`
	PlayerID int64                      `json:"player_id"`
	Player   *domain.Player             `json:"player"`
	Stats    *domain.AggregatedStats    `json:"stats,omitempty"`
}

// handleClaimValidate validates a claim code and returns player info
func (r *Router) handleClaimValidate(w http.ResponseWriter, req *http.Request) {
	var body ClaimValidateRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(body.Code) != 6 {
		writeError(w, http.StatusBadRequest, "code must be 6 digits")
		return
	}

	claimCode, err := r.store.GetValidClaimCode(req.Context(), body.Code)
	if err != nil {
		writeError(w, http.StatusNotFound, "invalid or expired claim code")
		return
	}

	// Get player info
	player, err := r.store.GetPlayerByID(req.Context(), claimCode.PlayerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get player info")
		return
	}

	response := ClaimValidateResponse{
		CodeID:   claimCode.ID,
		PlayerID: claimCode.PlayerID,
		Player:   player,
	}

	// Get player stats
	if stats, err := r.store.GetPlayerStatsByID(req.Context(), claimCode.PlayerID, "all"); err == nil {
		response.Stats = &stats.Stats
	}

	writeJSON(w, http.StatusOK, response)
}

// ClaimRegisterRequest is the request body for registering via claim code
type ClaimRegisterRequest struct {
	Code            string `json:"code"`
	Username        string `json:"username"`
	Password        string `json:"password"`
	ConfirmPassword string `json:"confirm_password"`
}

// handleClaimRegister creates a new account and claims the player
func (r *Router) handleClaimRegister(w http.ResponseWriter, req *http.Request) {
	var body ClaimRegisterRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.Username == "" || body.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password are required")
		return
	}

	if len(body.Username) < 2 || len(body.Username) > 16 || !validUsernameRegex.MatchString(body.Username) {
		writeError(w, http.StatusBadRequest, "username must be 2-16 characters and contain only letters, numbers, and underscores")
		return
	}

	if len(body.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	if body.Password != body.ConfirmPassword {
		writeError(w, http.StatusBadRequest, "passwords do not match")
		return
	}

	// Validate the claim code
	claimCode, err := r.store.GetValidClaimCode(req.Context(), body.Code)
	if err != nil {
		writeError(w, http.StatusNotFound, "invalid or expired claim code")
		return
	}

	// Hash password
	hash, err := auth.HashPassword(body.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	// Create account and claim player (atomic)
	userID, err := r.store.ClaimRegister(req.Context(), claimCode.ID, claimCode.PlayerID, body.Username, hash)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			writeError(w, http.StatusConflict, "username already exists")
			return
		}
		if strings.Contains(err.Error(), "already linked") {
			writeError(w, http.StatusConflict, "player is already linked to another account")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create account")
		return
	}

	// Generate login token
	token, err := r.auth.GenerateToken(userID, body.Username, false, &claimCode.PlayerID, false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "account created but failed to generate token")
		return
	}

	writeJSON(w, http.StatusCreated, LoginResponse{
		Token:    token,
		Username: body.Username,
		IsAdmin:  false,
		PlayerID: &claimCode.PlayerID,
	})
}

// ClaimLinkRequest is the request body for linking a claim code to an existing account
type ClaimLinkRequest struct {
	Code string `json:"code"`
}

// handleClaimLink merges a claim code's player into the logged-in user's account
func (r *Router) handleClaimLink(w http.ResponseWriter, req *http.Request) {
	claims := r.getAuthClaims(req)
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var body ClaimLinkRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate the claim code
	claimCode, err := r.store.GetValidClaimCode(req.Context(), body.Code)
	if err != nil {
		writeError(w, http.StatusNotFound, "invalid or expired claim code")
		return
	}

	// Link/merge the claim player into the user's account
	if err := r.store.ClaimLink(req.Context(), claimCode.ID, claimCode.PlayerID, claims.UserID); err != nil {
		if strings.Contains(err.Error(), "already linked") {
			writeError(w, http.StatusConflict, "player is already linked to another account")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to link player")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "player linked successfully"})
}
