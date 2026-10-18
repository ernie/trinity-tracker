package api

import (
	"encoding/json"
	"net/http"

	"github.com/ernie/trinity-tracker/internal/auth"
)

type GameLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type GameLoginResponse struct {
	Token    string `json:"token"`
	Username string `json:"username"`
}

type GameTokenResponse struct {
	Token    string `json:"token"`
	Username string `json:"username"`
}

func (r *Router) handleGameLogin(w http.ResponseWriter, req *http.Request) {
	var login GameLoginRequest
	if err := json.NewDecoder(req.Body).Decode(&login); err != nil {
		writeError(w, http.StatusBadRequest, "The request couldn't be read.")
		return
	}

	if login.Username == "" || login.Password == "" {
		writeError(w, http.StatusBadRequest, "Enter a username and password.")
		return
	}

	user, err := r.store.GetUserByUsername(req.Context(), login.Username)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Wrong username or password.")
		return
	}
	if !auth.CheckPassword(login.Password, user.PasswordHash) {
		writeError(w, http.StatusUnauthorized, "Wrong username or password.")
		return
	}
	if user.DisabledAt != nil {
		writeError(w, http.StatusForbidden, "This account is disabled.")
		return
	}

	token, err := r.store.EnsureGameToken(req.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't log you in.")
		return
	}

	r.loginLimiter.Reset(getClientIP(req))

	r.store.UpdateUserLastLogin(req.Context(), user.ID)

	writeJSON(w, http.StatusOK, GameLoginResponse{
		Token:    token,
		Username: user.Username,
	})
}

func (r *Router) handleGetGameToken(w http.ResponseWriter, req *http.Request) {
	claims := r.getAuthClaims(req)
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Log in first.")
		return
	}

	token, err := r.store.EnsureGameToken(req.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't load your game token.")
		return
	}

	writeJSON(w, http.StatusOK, GameTokenResponse{Token: token, Username: claims.Username})
}

func (r *Router) handleRotateGameToken(w http.ResponseWriter, req *http.Request) {
	claims := r.getAuthClaims(req)
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Log in first.")
		return
	}

	token, err := r.store.RotateGameToken(req.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't rotate your token.")
		return
	}

	writeJSON(w, http.StatusOK, GameTokenResponse{Token: token, Username: claims.Username})
}
