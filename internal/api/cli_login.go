package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/ernie/trinity-tracker/internal/auth"
)

// CLILoginRequest is the request body for CLI login (PAT mint).
type CLILoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	// Name labels the token (e.g. "cli:ernie@host"); server-sanitized.
	Name string `json:"name"`
}

// CLILoginResponse carries the freshly minted PAT — the only time the
// plaintext ever leaves the server.
type CLILoginResponse struct {
	Token    string `json:"token"`
	Username string `json:"username"`
}

// maxPATNameLen bounds the stored token label.
const maxPATNameLen = 80

// handleCLILogin verifies username/password and mints a PAT. Unlike
// web login it refuses accounts with a pending forced password change:
// a long-lived credential shouldn't be minted from a password the user
// is required to replace.
func (r *Router) handleCLILogin(w http.ResponseWriter, req *http.Request) {
	var login CLILoginRequest
	if err := json.NewDecoder(req.Body).Decode(&login); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if login.Username == "" || login.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password are required")
		return
	}

	user, err := r.store.GetUserByUsername(req.Context(), login.Username)
	if err != nil || !auth.CheckPassword(login.Password, user.PasswordHash) {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if user.DisabledAt != nil {
		writeError(w, http.StatusForbidden, "account disabled")
		return
	}
	if user.PasswordChangeRequired {
		writeError(w, http.StatusForbidden, "password change required: log in to the web UI first")
		return
	}

	token, hash, err := auth.NewPAT()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to mint token")
		return
	}
	if err := r.store.CreatePAT(req.Context(), user.ID, sanitizePATName(login.Name), hash); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store token")
		return
	}

	// Successful login — reset rate limiter for this IP (same policy
	// as web login).
	r.loginLimiter.Reset(getClientIP(req))
	r.store.UpdateUserLastLogin(req.Context(), user.ID)

	writeJSON(w, http.StatusOK, CLILoginResponse{Token: token, Username: user.Username})
}

// handleCLILogout revokes the presenting PAT. Only a PAT can revoke
// itself — a (possibly stolen) web JWT cannot kill CLI credentials.
// Repeating the call yields 401 (the token no longer resolves), which
// clients treat as already-logged-out.
func (r *Router) handleCLILogout(w http.ResponseWriter, req *http.Request) {
	authHeader := req.Header.Get("Authorization")
	token := strings.TrimPrefix(authHeader, "Bearer ")
	if !strings.HasPrefix(authHeader, "Bearer ") || !auth.IsPAT(token) {
		writeError(w, http.StatusUnauthorized, "a personal access token is required")
		return
	}
	if err := r.store.RevokePATByHash(req.Context(), auth.HashPAT(token)); err != nil {
		writeError(w, http.StatusUnauthorized, "token unknown or already revoked")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

// sanitizePATName keeps token labels printable and bounded.
func sanitizePATName(name string) string {
	name = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if len(name) > maxPATNameLen {
		name = name[:maxPATNameLen]
	}
	return name
}
