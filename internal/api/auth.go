package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/ernie/trinity-tracker/internal/auth"
	"github.com/ernie/trinity-tracker/internal/domain"
	"github.com/ernie/trinity-tracker/internal/storage"
)

// LoginRequest is the request body for login
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse is the response body for successful login. The JWT
// itself travels in the HttpOnly session cookie, never the body.
type LoginResponse struct {
	Username               string `json:"username"`
	IsAdmin                bool   `json:"is_admin"`
	PlayerID               *int64 `json:"player_id,omitempty"`
	PasswordChangeRequired bool   `json:"password_change_required"`
}

// handleLogin authenticates a user and sets the session cookie
func (r *Router) handleLogin(w http.ResponseWriter, req *http.Request) {
	var login LoginRequest
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

	token, err := r.auth.GenerateToken(user.ID, user.Username, user.IsAdmin, user.PlayerID, user.PasswordChangeRequired, user.TokenVersion)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	// Successful login — reset rate limiter for this IP
	r.loginLimiter.Reset(getClientIP(req))

	// Update last login timestamp
	r.store.UpdateUserLastLogin(req.Context(), user.ID)

	setSessionCookie(w, req, token)
	writeJSON(w, http.StatusOK, LoginResponse{
		Username:               user.Username,
		IsAdmin:                user.IsAdmin,
		PlayerID:               user.PlayerID,
		PasswordChangeRequired: user.PasswordChangeRequired,
	})
}

// handleLogout ends this device's session by expiring its cookie.
// Per-device on purpose: other sessions stay valid (logout-all is the
// deliberate version). Unauthenticated — clearing a cookie needs no
// proof of identity.
func (r *Router) handleLogout(w http.ResponseWriter, req *http.Request) {
	clearSessionCookie(w, req)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleLogoutAll bumps the user's token_version, killing every JWT
// they ever held; PATs are unaffected.
func (r *Router) handleLogoutAll(w http.ResponseWriter, req *http.Request) {
	claims := r.getAuthClaims(req)
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if err := r.store.BumpUserTokenVersion(req.Context(), claims.UserID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to log out everywhere")
		return
	}
	clearSessionCookie(w, req)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleAuthCheck checks if the current token is valid
func (r *Router) handleAuthCheck(w http.ResponseWriter, req *http.Request) {
	claims := r.getAuthClaims(req)
	if claims == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"authenticated": false,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"authenticated":            true,
		"username":                 claims.Username,
		"is_admin":                 claims.IsAdmin,
		"player_id":                claims.PlayerID,
		"password_change_required": claims.PasswordChangeRequired,
	})
}

// requireAuth is middleware that validates JWT before calling the handler
func (r *Router) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		claims := r.getAuthClaims(req)
		if claims == nil {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		next(w, req)
	}
}

// requireAdmin is middleware that validates JWT and checks admin status
func (r *Router) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		claims := r.getAuthClaims(req)
		if claims == nil {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		if !claims.IsAdmin {
			writeError(w, http.StatusForbidden, "admin access required")
			return
		}
		next(w, req)
	}
}

// getAuthClaims extracts and validates the request credential. The
// Authorization header wins (PATs and API scripting); otherwise the
// session cookie (browsers). Two credential types share the header:
// PATs (CLI, "trin_" prefix) and JWTs (web sessions). Both resolve
// against the live user row, so revocation, disable, and privilege
// flips all take effect on the next request.
func (r *Router) getAuthClaims(req *http.Request) *auth.Claims {
	if authHeader := req.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if auth.IsPAT(token) {
			return r.getPATClaims(req, token)
		}
		return r.getJWTClaims(req, token)
	}

	cookie, err := req.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return nil
	}
	// Cookies are attached by the browser, not the page — gate
	// cross-site writes before honoring one.
	if !cookieOriginAllowed(req) {
		return nil
	}
	return r.getJWTClaims(req, cookie.Value)
}

// getJWTClaims validates a web-session JWT: signature/expiry first,
// then the live user row — token_version mismatch (revoked by bump)
// and disabled accounts are rejected. Claims are rebuilt from the row,
// mirroring getPATClaims: the JWT is pure identity, privileges are
// always live.
func (r *Router) getJWTClaims(req *http.Request, token string) *auth.Claims {
	claims, err := r.auth.ValidateToken(token)
	if err != nil || r.store == nil {
		return nil
	}
	user, err := r.store.GetUserByID(req.Context(), claims.UserID)
	if err != nil || user.DisabledAt != nil || user.TokenVersion != claims.TokenVersion {
		return nil
	}
	return &auth.Claims{
		Username:               user.Username,
		UserID:                 user.ID,
		IsAdmin:                user.IsAdmin,
		PlayerID:               user.PlayerID,
		PasswordChangeRequired: user.PasswordChangeRequired,
		TokenVersion:           user.TokenVersion,
	}
}

// getPATClaims resolves a personal access token to claims built from
// the user's current DB state: revocation, user deletion, and admin
// flips all take effect on the next request, no re-login involved.
func (r *Router) getPATClaims(req *http.Request, token string) *auth.Claims {
	if r.store == nil {
		return nil
	}
	id, err := r.store.LookupPATByHash(req.Context(), auth.HashPAT(token))
	if err != nil {
		return nil
	}
	r.touchPAT(req.Context(), id.PATID)
	return &auth.Claims{
		Username:               id.Username,
		UserID:                 id.UserID,
		IsAdmin:                id.IsAdmin,
		PlayerID:               id.PlayerID,
		PasswordChangeRequired: id.PasswordChangeRequired,
	}
}

// touchPAT records token activity, throttled to one write per token
// per minute.
func (r *Router) touchPAT(ctx context.Context, patID int64) {
	r.patTouchMu.Lock()
	last, seen := r.patTouchSeen[patID]
	now := time.Now()
	if seen && now.Sub(last) < time.Minute {
		r.patTouchMu.Unlock()
		return
	}
	r.patTouchSeen[patID] = now
	r.patTouchMu.Unlock()

	if err := r.store.TouchPATLastUsed(ctx, patID); err != nil {
		log.Printf("pat: last_used_at update failed: %v", err)
	}
}

// ChangePasswordRequest is the request body for password change
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// handleChangePassword allows users to change their own password
func (r *Router) handleChangePassword(w http.ResponseWriter, req *http.Request) {
	claims := r.getAuthClaims(req)
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var body ChangePasswordRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(body.NewPassword) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	// Verify current password
	user, err := r.store.GetUserByID(req.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get user")
		return
	}

	if !auth.CheckPassword(body.CurrentPassword, user.PasswordHash) {
		writeError(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}

	// Hash and update new password
	hash, err := auth.HashPassword(body.NewPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	if err := r.store.UpdateUserPassword(req.Context(), claims.UserID, hash); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update password")
		return
	}

	// The password write bumped token_version, killing every JWT the
	// user held — including the one authenticating this request.
	// Re-fetch for the new version and re-cookie this device so the
	// changer stays logged in; all other devices die.
	user, err = r.store.GetUserByID(req.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get user")
		return
	}
	newToken, err := r.auth.GenerateToken(user.ID, user.Username, user.IsAdmin, user.PlayerID, false, user.TokenVersion)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate new token")
		return
	}

	setSessionCookie(w, req, newToken)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "password changed successfully",
	})
}

// CreateUserRequest is the request body for creating a user (admin only)
type CreateUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	IsAdmin  bool   `json:"is_admin"`
	PlayerID *int64 `json:"player_id,omitempty"`
}

// handleCreateUser creates a new user (admin only)
func (r *Router) handleCreateUser(w http.ResponseWriter, req *http.Request) {
	var body CreateUserRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.Username == "" || body.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password are required")
		return
	}

	if len(body.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	// Check if player is already claimed
	if body.PlayerID != nil {
		claimed, err := r.store.IsPlayerClaimed(req.Context(), *body.PlayerID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to check player status")
			return
		}
		if claimed {
			writeError(w, http.StatusConflict, "player is already linked to another user")
			return
		}
	}

	hash, err := auth.HashPassword(body.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	if err := r.store.CreateUser(req.Context(), body.Username, hash, body.IsAdmin, body.PlayerID); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			writeError(w, http.StatusConflict, "username already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create user")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"message": "user created"})
}

// UserResponse is a user without the password hash
type UserResponse struct {
	ID                     int64      `json:"id"`
	Username               string     `json:"username"`
	IsAdmin                bool       `json:"is_admin"`
	PlayerID               *int64     `json:"player_id,omitempty"`
	PlayerName             *string    `json:"player_name,omitempty"`
	PasswordChangeRequired bool       `json:"password_change_required"`
	CreatedAt              time.Time  `json:"created_at"`
	LastLogin              *time.Time `json:"last_login,omitempty"`
}

// handleListUsers returns users (admin only). Without query params it returns
// the full list; with ?search=q[&limit=n] it returns up to limit matches by
// username or linked player name.
func (r *Router) handleListUsers(w http.ResponseWriter, req *http.Request) {
	var (
		users []storage.UserWithPlayer
		err   error
	)
	if search := req.URL.Query().Get("search"); search != "" {
		limit := parseLimit(req, 10, 50)
		users, err = r.store.SearchUsersWithPlayer(req.Context(), search, limit)
	} else {
		users, err = r.store.ListUsersWithPlayer(req.Context())
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Convert to response format (don't expose password hashes)
	response := make([]UserResponse, len(users))
	for i, u := range users {
		response[i] = UserResponse{
			ID:                     u.ID,
			Username:               u.Username,
			IsAdmin:                u.IsAdmin,
			PlayerID:               u.PlayerID,
			PlayerName:             u.PlayerName,
			PasswordChangeRequired: u.PasswordChangeRequired,
			CreatedAt:              u.CreatedAt,
			LastLogin:              u.LastLogin,
		}
	}

	writeJSON(w, http.StatusOK, response)
}

// handleDeleteUser deletes a user (admin only)
func (r *Router) handleDeleteUser(w http.ResponseWriter, req *http.Request) {
	username := req.PathValue("username")
	if username == "" {
		writeError(w, http.StatusBadRequest, "username required")
		return
	}

	// Prevent self-deletion
	claims := r.getAuthClaims(req)
	if claims != nil && claims.Username == username {
		writeError(w, http.StatusForbidden, "cannot delete yourself")
		return
	}

	if err := r.store.DeleteUser(req.Context(), username); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "user deleted"})
}

// ResetPasswordRequest is the request body for admin password reset
type ResetPasswordRequest struct {
	NewPassword string `json:"new_password"`
}

// handleResetUserPassword resets a user's password (admin only)
func (r *Router) handleResetUserPassword(w http.ResponseWriter, req *http.Request) {
	userID, err := parseID(req, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	var body ResetPasswordRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(body.NewPassword) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	hash, err := auth.HashPassword(body.NewPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	if err := r.store.ResetUserPassword(req.Context(), userID, hash); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reset password")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "password reset"})
}

// UpdateUserRequest is the request body for updating user properties
type UpdateUserRequest struct {
	IsAdmin  *bool  `json:"is_admin,omitempty"`
	PlayerID *int64 `json:"player_id,omitempty"`
}

// handleUpdateUser updates user properties (admin only)
func (r *Router) handleUpdateUser(w http.ResponseWriter, req *http.Request) {
	userID, err := parseID(req, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	var body UpdateUserRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.IsAdmin != nil {
		if err := r.store.UpdateUserAdmin(req.Context(), userID, *body.IsAdmin); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update admin status")
			return
		}
	}

	if body.PlayerID != nil {
		// Check if player is already claimed by another user
		claimed, err := r.store.IsPlayerClaimed(req.Context(), *body.PlayerID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to check player status")
			return
		}
		if claimed {
			// Check if it's claimed by this same user (which is fine)
			user, _ := r.store.GetUserByID(req.Context(), userID)
			if user == nil || user.PlayerID == nil || *user.PlayerID != *body.PlayerID {
				writeError(w, http.StatusConflict, "player is already linked to another user")
				return
			}
		}
		if err := r.store.UpdateUserPlayerLink(req.Context(), userID, body.PlayerID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update player link")
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "user updated"})
}

// LinkCodeResponse is the response for creating a link code
type LinkCodeResponse struct {
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expires_at"`
}

// AccountProfileResponse is the response for the current user's account profile
type AccountProfileResponse struct {
	User   UserResponse        `json:"user"`
	Player *domain.Player      `json:"player,omitempty"`
	GUIDs  []domain.PlayerGUID `json:"guids,omitempty"`
}

// handleGetAccountProfile returns the current user's profile with linked player info
func (r *Router) handleGetAccountProfile(w http.ResponseWriter, req *http.Request) {
	claims := r.getAuthClaims(req)
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	user, err := r.store.GetUserByID(req.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get user")
		return
	}

	response := AccountProfileResponse{
		User: UserResponse{
			ID:                     user.ID,
			Username:               user.Username,
			IsAdmin:                user.IsAdmin,
			PlayerID:               user.PlayerID,
			PasswordChangeRequired: user.PasswordChangeRequired,
			CreatedAt:              user.CreatedAt,
			LastLogin:              user.LastLogin,
		},
	}

	// If user has linked player, fetch player profile and GUIDs
	if user.PlayerID != nil {
		player, err := r.store.GetPlayerByID(req.Context(), *user.PlayerID)
		if err == nil {
			response.Player = player
		}

		guids, err := r.store.GetPlayerGUIDs(req.Context(), *user.PlayerID)
		if err == nil {
			response.GUIDs = guids
		}
	}

	writeJSON(w, http.StatusOK, response)
}

// handleCreateLinkCode generates a link code for the authenticated user
func (r *Router) handleCreateLinkCode(w http.ResponseWriter, req *http.Request) {
	claims := r.getAuthClaims(req)
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	// Require user to have a player_id already linked
	if claims.PlayerID == nil {
		writeError(w, http.StatusBadRequest, "you must have a linked player to generate a link code")
		return
	}

	// Invalidate any existing pending codes for this user
	if err := r.store.InvalidateUserLinkCodes(req.Context(), claims.UserID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to invalidate existing codes")
		return
	}

	// Create new code with 10-minute expiry
	expiresAt := time.Now().Add(10 * time.Minute)
	linkCode, err := r.store.CreateLinkCode(req.Context(), claims.UserID, *claims.PlayerID, expiresAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create link code")
		return
	}

	writeJSON(w, http.StatusOK, LinkCodeResponse{
		Code:      linkCode.Code,
		ExpiresAt: linkCode.ExpiresAt,
	})
}
