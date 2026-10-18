package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/ernie/trinity-tracker/internal/auth"
)

// CLILoginRequest is the CLI login body, which creates a personal
// access token.
type CLILoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	// Name labels the token, such as "cli:user@host".
	Name string `json:"name"`
}

// CLILoginResponse carries the new token. This is the only time its
// plaintext leaves the server.
type CLILoginResponse struct {
	Token    string `json:"token"`
	Username string `json:"username"`
}

const maxPATNameLen = 80

// handleCLILogin checks the username and password and issues a personal
// access token. It refuses an account that must change its password,
// since a long-lived token shouldn't come from a password due for
// replacement.
func (r *Router) handleCLILogin(w http.ResponseWriter, req *http.Request) {
	var login CLILoginRequest
	if err := json.NewDecoder(req.Body).Decode(&login); err != nil {
		writeError(w, http.StatusBadRequest, "The request couldn't be read.")
		return
	}
	if login.Username == "" || login.Password == "" {
		writeError(w, http.StatusBadRequest, "Enter a username and password.")
		return
	}

	user, err := r.store.GetUserByUsername(req.Context(), login.Username)
	if err != nil || !auth.CheckPassword(login.Password, user.PasswordHash) {
		writeError(w, http.StatusUnauthorized, "Wrong username or password.")
		return
	}
	if user.DisabledAt != nil {
		writeError(w, http.StatusForbidden, "This account is disabled.")
		return
	}
	if user.PasswordChangeRequired {
		writeError(w, http.StatusForbidden, "Change your password on the website first.")
		return
	}

	token, hash, err := auth.NewPAT()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't create a token.")
		return
	}
	if err := r.store.CreatePAT(req.Context(), user.ID, sanitizePATName(login.Name), hash); err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't save the token.")
		return
	}

	r.loginLimiter.Reset(getClientIP(req))
	r.store.UpdateUserLastLogin(req.Context(), user.ID)

	writeJSON(w, http.StatusOK, CLILoginResponse{Token: token, Username: user.Username})
}

// handleCLILogout revokes the personal access token that authenticates
// it; a web session can't revoke CLI tokens. Repeating the call returns
// 401, which clients treat as already logged out.
func (r *Router) handleCLILogout(w http.ResponseWriter, req *http.Request) {
	authHeader := req.Header.Get("Authorization")
	token := strings.TrimPrefix(authHeader, "Bearer ")
	if !strings.HasPrefix(authHeader, "Bearer ") || !auth.IsPAT(token) {
		writeError(w, http.StatusUnauthorized, "A personal access token is required.")
		return
	}
	if err := r.store.RevokePATByHash(req.Context(), auth.HashPAT(token)); err != nil {
		writeError(w, http.StatusUnauthorized, "That token is unknown or already revoked.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

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
