package api

import (
	"encoding/json"
	"net"
	"net/http"

	"github.com/ernie/trinity-tools/internal/auth"
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
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if login.Username == "" || login.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password are required")
		return
	}

	user, err := r.store.GetUserByUsername(req.Context(), login.Username)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if !auth.CheckPassword(login.Password, user.PasswordHash) {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	token, err := r.store.EnsureGameToken(req.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	// Successful login — reset rate limiter for this IP
	if ip, _, err := net.SplitHostPort(req.RemoteAddr); err == nil && ip != "" {
		r.loginLimiter.Reset(ip)
	}

	r.store.UpdateUserLastLogin(req.Context(), user.ID)

	writeJSON(w, http.StatusOK, GameLoginResponse{
		Token:    token,
		Username: user.Username,
	})
}

func (r *Router) handleGetGameToken(w http.ResponseWriter, req *http.Request) {
	claims := r.getAuthClaims(req)
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	token, err := r.store.EnsureGameToken(req.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get token")
		return
	}

	writeJSON(w, http.StatusOK, GameTokenResponse{Token: token, Username: claims.Username})
}

func (r *Router) handleRotateGameToken(w http.ResponseWriter, req *http.Request) {
	claims := r.getAuthClaims(req)
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	token, err := r.store.RotateGameToken(req.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to rotate token")
		return
	}

	writeJSON(w, http.StatusOK, GameTokenResponse{Token: token, Username: claims.Username})
}
