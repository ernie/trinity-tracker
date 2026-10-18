package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/ernie/trinity-tracker/internal/auth"
	"github.com/ernie/trinity-tracker/internal/domain"
	"github.com/ernie/trinity-tracker/internal/storage"
)

// LoginRequest is the login request body.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse is the body of a successful login. The JWT travels only
// in the HttpOnly session cookie.
type LoginResponse struct {
	Username               string `json:"username"`
	IsAdmin                bool   `json:"is_admin"`
	PlayerID               *int64 `json:"player_id,omitempty"`
	PasswordChangeRequired bool   `json:"password_change_required"`
}

func (r *Router) handleLogin(w http.ResponseWriter, req *http.Request) {
	var login LoginRequest
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

	token, err := r.auth.GenerateToken(user.ID, user.Username, user.IsAdmin, user.PlayerID, user.PasswordChangeRequired, user.TokenVersion)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't log you in.")
		return
	}

	r.loginLimiter.Reset(getClientIP(req))

	r.store.UpdateUserLastLogin(req.Context(), user.ID)

	setSessionCookie(w, req, token)
	writeJSON(w, http.StatusOK, LoginResponse{
		Username:               user.Username,
		IsAdmin:                user.IsAdmin,
		PlayerID:               user.PlayerID,
		PasswordChangeRequired: user.PasswordChangeRequired,
	})
}

// handleLogout expires this device's session cookie; other sessions
// stay valid. Clearing a cookie needs no authentication.
func (r *Router) handleLogout(w http.ResponseWriter, req *http.Request) {
	clearSessionCookie(w, req)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleLogoutAll bumps the user's token_version, which invalidates
// every JWT they hold. Personal access tokens keep working.
func (r *Router) handleLogoutAll(w http.ResponseWriter, req *http.Request) {
	claims := r.getAuthClaims(req)
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Log in first.")
		return
	}
	if err := r.store.BumpUserTokenVersion(req.Context(), claims.UserID); err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't log you out everywhere.")
		return
	}
	clearSessionCookie(w, req)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

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

func (r *Router) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		claims := r.getAuthClaims(req)
		if claims == nil {
			writeError(w, http.StatusUnauthorized, "Log in first.")
			return
		}
		next(w, req)
	}
}

func (r *Router) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		claims := r.getAuthClaims(req)
		if claims == nil {
			writeError(w, http.StatusUnauthorized, "Log in first.")
			return
		}
		if !claims.IsAdmin {
			writeError(w, http.StatusForbidden, "Only admins can do that.")
			return
		}
		next(w, req)
	}
}

// getAuthClaims validates the request's credential: a bearer PAT or JWT
// in the Authorization header if present, else the session cookie.
// Claims come from the live user row, so revocation, disabling and
// admin changes take effect on the next request.
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
	// The browser attaches cookies to cross-site requests too.
	if !cookieOriginAllowed(req) {
		return nil
	}
	return r.getJWTClaims(req, cookie.Value)
}

// getJWTClaims validates a session JWT, then rejects it if the account
// is disabled or its token_version has moved on. The JWT only names the
// user; the claims come from the user row.
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

// getPATClaims resolves a personal access token to claims from the
// user's current row.
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

// ChangePasswordRequest is the change-password request body.
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (r *Router) handleChangePassword(w http.ResponseWriter, req *http.Request) {
	claims := r.getAuthClaims(req)
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Log in first.")
		return
	}

	var body ChangePasswordRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "The request couldn't be read.")
		return
	}

	if len(body.NewPassword) < 8 {
		writeError(w, http.StatusBadRequest, "Password must be at least 8 characters.")
		return
	}

	user, err := r.store.GetUserByID(req.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't load your account.")
		return
	}

	if !auth.CheckPassword(body.CurrentPassword, user.PasswordHash) {
		writeError(w, http.StatusUnauthorized, "Your current password is wrong.")
		return
	}

	hash, err := auth.HashPassword(body.NewPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't save the password.")
		return
	}

	if err := r.store.UpdateUserPassword(req.Context(), claims.UserID, hash); err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't change your password.")
		return
	}

	// The write bumped token_version, logging out every device. Issue
	// this one a new cookie so it stays logged in.
	user, err = r.store.GetUserByID(req.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't load your account.")
		return
	}
	newToken, err := r.auth.GenerateToken(user.ID, user.Username, user.IsAdmin, user.PlayerID, false, user.TokenVersion)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Your password changed, but this device was logged out. Log in again.")
		return
	}

	setSessionCookie(w, req, newToken)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Password changed.",
	})
}

// CreateUserRequest is the admin create-user request body.
type CreateUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	IsAdmin  bool   `json:"is_admin"`
	PlayerID *int64 `json:"player_id,omitempty"`
}

func (r *Router) handleCreateUser(w http.ResponseWriter, req *http.Request) {
	var body CreateUserRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "The request couldn't be read.")
		return
	}

	if body.Username == "" || body.Password == "" {
		writeError(w, http.StatusBadRequest, "Enter a username and password.")
		return
	}

	if len(body.Password) < 8 {
		writeError(w, http.StatusBadRequest, "Password must be at least 8 characters.")
		return
	}

	if body.PlayerID != nil {
		claimed, err := r.store.IsPlayerClaimed(req.Context(), *body.PlayerID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Couldn't check that player.")
			return
		}
		if claimed {
			writeError(w, http.StatusConflict, "That player is already linked to another account.")
			return
		}
	}

	hash, err := auth.HashPassword(body.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't save the password.")
		return
	}

	if err := r.store.CreateUser(req.Context(), body.Username, hash, body.IsAdmin, body.PlayerID); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			writeError(w, http.StatusConflict, "That username is taken.")
			return
		}
		writeError(w, http.StatusInternalServerError, "Couldn't create the user.")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"message": "User created."})
}

// UserResponse is a user without the password hash.
type UserResponse struct {
	ID                     int64      `json:"id"`
	Username               string     `json:"username"`
	DisplayName            string     `json:"display_name"`
	IsAdmin                bool       `json:"is_admin"`
	PlayerID               *int64     `json:"player_id,omitempty"`
	PlayerName             *string    `json:"player_name,omitempty"`
	PasswordChangeRequired bool       `json:"password_change_required"`
	CreatedAt              time.Time  `json:"created_at"`
	LastLogin              *time.Time `json:"last_login,omitempty"`
	Portrait               *string    `json:"portrait,omitempty"`
}

// handleListUsers returns every user, or with ?search=q[&limit=n] up to
// limit users matching by username or linked player name.
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

	response := make([]UserResponse, len(users))
	for i, u := range users {
		response[i] = UserResponse{
			ID:                     u.ID,
			Username:               u.Username,
			DisplayName:            u.DisplayName,
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

func (r *Router) handleDeleteUser(w http.ResponseWriter, req *http.Request) {
	username := req.PathValue("username")
	if username == "" {
		writeError(w, http.StatusBadRequest, "Enter a username.")
		return
	}

	claims := r.getAuthClaims(req)
	if claims != nil && claims.Username == username {
		writeError(w, http.StatusForbidden, "You can't delete your own account.")
		return
	}

	if err := r.store.DeleteUser(req.Context(), username); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "User deleted."})
}

// ResetPasswordRequest is the admin password-reset request body.
type ResetPasswordRequest struct {
	NewPassword string `json:"new_password"`
}

// handleResetUserPassword sets a temporary password that the user must
// change at their next login.
func (r *Router) handleResetUserPassword(w http.ResponseWriter, req *http.Request) {
	userID, err := parseID(req, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid user ID.")
		return
	}

	var body ResetPasswordRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "The request couldn't be read.")
		return
	}

	if len(body.NewPassword) < 8 {
		writeError(w, http.StatusBadRequest, "Password must be at least 8 characters.")
		return
	}

	hash, err := auth.HashPassword(body.NewPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't save the password.")
		return
	}

	if err := r.store.ResetUserPassword(req.Context(), userID, hash); err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't reset the password.")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Password reset. The user picks a new one at their next login."})
}

// UpdateUserRequest is the admin update-user request body.
type UpdateUserRequest struct {
	IsAdmin     *bool   `json:"is_admin,omitempty"`
	PlayerID    *int64  `json:"player_id,omitempty"`
	DisplayName *string `json:"display_name,omitempty"`
}

func (r *Router) handleUpdateUser(w http.ResponseWriter, req *http.Request) {
	userID, err := parseID(req, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid user ID.")
		return
	}

	var body UpdateUserRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "The request couldn't be read.")
		return
	}

	if body.IsAdmin != nil {
		if err := r.store.UpdateUserAdmin(req.Context(), userID, *body.IsAdmin); err != nil {
			writeError(w, http.StatusInternalServerError, "Couldn't change admin status.")
			return
		}
	}

	if body.PlayerID != nil {
		claimed, err := r.store.IsPlayerClaimed(req.Context(), *body.PlayerID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Couldn't check that player.")
			return
		}
		if claimed {
			user, _ := r.store.GetUserByID(req.Context(), userID)
			if user == nil || user.PlayerID == nil || *user.PlayerID != *body.PlayerID {
				writeError(w, http.StatusConflict, "That player is already linked to another account.")
				return
			}
		}
		if err := r.store.UpdateUserPlayerLink(req.Context(), userID, body.PlayerID); err != nil {
			writeError(w, http.StatusInternalServerError, "Couldn't change the player link.")
			return
		}
	}

	if body.DisplayName != nil {
		cleaned := domain.CleanQ3DisplayName(*body.DisplayName)
		if cleaned == "" {
			writeError(w, http.StatusBadRequest, "Enter a display name.")
			return
		}
		canonical := storage.CanonicalizeDisplayName(cleaned)
		if canonical == "" {
			writeError(w, http.StatusBadRequest, "Display names need some visible text, not just color codes.")
			return
		}
		err := r.store.AdminSetUserDisplayName(req.Context(), userID, cleaned, canonical)
		switch {
		case err == nil:
		case errors.Is(err, storage.ErrDisplayNameReserved):
			writeError(w, http.StatusConflict, "That display name is reserved.")
			return
		case errors.Is(err, storage.ErrDisplayNameTaken):
			writeError(w, http.StatusConflict, "That display name is taken.")
			return
		default:
			writeError(w, http.StatusInternalServerError, "Couldn't update the display name.")
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "User updated."})
}

// LinkCodeResponse is a new link code and its expiry.
type LinkCodeResponse struct {
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expires_at"`
}

// AccountProfileResponse is the signed-in user's account, with their
// linked player and GUIDs when they have one.
type AccountProfileResponse struct {
	User   UserResponse        `json:"user"`
	Player *domain.Player      `json:"player,omitempty"`
	GUIDs  []domain.PlayerGUID `json:"guids,omitempty"`
}

func (r *Router) handleGetAccountProfile(w http.ResponseWriter, req *http.Request) {
	claims := r.getAuthClaims(req)
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Log in first.")
		return
	}

	user, err := r.store.GetUserByID(req.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't load your account.")
		return
	}

	response := AccountProfileResponse{
		User: UserResponse{
			ID:                     user.ID,
			Username:               user.Username,
			DisplayName:            user.DisplayName,
			IsAdmin:                user.IsAdmin,
			PlayerID:               user.PlayerID,
			PasswordChangeRequired: user.PasswordChangeRequired,
			CreatedAt:              user.CreatedAt,
			LastLogin:              user.LastLogin,
		},
	}
	if user.Portrait != "" {
		response.User.Portrait = &user.Portrait
	}

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

// handleCreateLinkCode replaces the user's pending link code with a new
// one valid for 10 minutes.
func (r *Router) handleCreateLinkCode(w http.ResponseWriter, req *http.Request) {
	claims := r.getAuthClaims(req)
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Log in first.")
		return
	}

	if claims.PlayerID == nil {
		writeError(w, http.StatusBadRequest, "Claim a player first, then generate a link code.")
		return
	}

	if err := r.store.InvalidateUserLinkCodes(req.Context(), claims.UserID); err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't replace your old link code.")
		return
	}

	expiresAt := time.Now().Add(10 * time.Minute)
	linkCode, err := r.store.CreateLinkCode(req.Context(), claims.UserID, *claims.PlayerID, expiresAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't generate a link code.")
		return
	}

	writeJSON(w, http.StatusOK, LinkCodeResponse{
		Code:      linkCode.Code,
		ExpiresAt: linkCode.ExpiresAt,
	})
}
