package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/ernie/trinity-tracker/internal/auth"
)

// cliUser creates a user whose forced password change is already
// cleared (CreateUser defaults the flag on), so cli-login will mint.
func (tr *testRouter) cliUser(t *testing.T, username string, isAdmin bool) int64 {
	t.Helper()
	_, userID := tr.loginAs(t, username, isAdmin)
	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if err := tr.store.UpdateUserPassword(context.Background(), userID, hash); err != nil {
		t.Fatalf("clear password_change_required: %v", err)
	}
	return userID
}

// cliLogin mints a PAT through the endpoint and returns the plaintext.
func (tr *testRouter) cliLogin(t *testing.T, username string) string {
	t.Helper()
	w := tr.do("POST", "/api/auth/cli-login",
		`{"username":"`+username+`","password":"password123","name":"cli:test@box"}`, "")
	if w.Code != http.StatusOK {
		t.Fatalf("cli-login: status %d body %s", w.Code, w.Body.String())
	}
	var resp CLILoginResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.HasPrefix(resp.Token, auth.PATPrefix) {
		t.Fatalf("token missing %q prefix: %q", auth.PATPrefix, resp.Token)
	}
	return resp.Token
}

// authCheck probes how a credential resolves via /api/auth/check.
func (tr *testRouter) authCheck(t *testing.T, token string) map[string]any {
	t.Helper()
	w := tr.do("GET", "/api/auth/check", "", token)
	if w.Code != http.StatusOK {
		t.Fatalf("auth/check: status %d", w.Code)
	}
	var out map[string]any
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

func TestCLILoginMintsUsablePAT(t *testing.T) {
	tr := newTestRouter(t)
	tr.cliUser(t, "ernie", true)

	token := tr.cliLogin(t, "ernie")

	check := tr.authCheck(t, token)
	if check["authenticated"] != true || check["username"] != "ernie" || check["is_admin"] != true {
		t.Errorf("PAT did not resolve to its user: %v", check)
	}

	// Plaintext is never stored: only the hash matches a row.
	if _, err := tr.store.LookupPATByHash(context.Background(), auth.HashPAT(token)); err != nil {
		t.Errorf("hash lookup failed: %v", err)
	}
	if _, err := tr.store.LookupPATByHash(context.Background(), token); err == nil {
		t.Error("plaintext stored as hash — must never happen")
	}
}

func TestCLILoginRejectsBadPassword(t *testing.T) {
	tr := newTestRouter(t)
	tr.cliUser(t, "ernie", true)

	w := tr.do("POST", "/api/auth/cli-login",
		`{"username":"ernie","password":"wrong","name":"x"}`, "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
}

func TestCLILoginRejectsPendingPasswordChange(t *testing.T) {
	tr := newTestRouter(t)
	// loginAs leaves password_change_required = TRUE (CreateUser default).
	tr.loginAs(t, "fresh", false)

	w := tr.do("POST", "/api/auth/cli-login",
		`{"username":"fresh","password":"password123","name":"x"}`, "")
	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d body %s", w.Code, w.Body.String())
	}
}

func TestCLILogoutRevokes(t *testing.T) {
	tr := newTestRouter(t)
	tr.cliUser(t, "ernie", true)
	token := tr.cliLogin(t, "ernie")

	w := tr.do("DELETE", "/api/auth/cli-login", "", token)
	if w.Code != http.StatusOK {
		t.Fatalf("logout: status %d", w.Code)
	}

	// Replaying the revoked token resolves to nobody.
	check := tr.authCheck(t, token)
	if check["authenticated"] != false {
		t.Errorf("revoked token still authenticates: %v", check)
	}

	// Repeat logout: 401, which clients treat as already-logged-out.
	w = tr.do("DELETE", "/api/auth/cli-login", "", token)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("repeat logout: want 401, got %d", w.Code)
	}
}

func TestCLILogoutRequiresPAT(t *testing.T) {
	tr := newTestRouter(t)
	jwt, _ := tr.loginAs(t, "webuser", true)

	// A web JWT must not be able to revoke CLI credentials.
	w := tr.do("DELETE", "/api/auth/cli-login", "", jwt)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("JWT logout: want 401, got %d", w.Code)
	}
}

func TestPATClaimsAreLive(t *testing.T) {
	tr := newTestRouter(t)
	userID := tr.cliUser(t, "ernie", true)
	token := tr.cliLogin(t, "ernie")

	if check := tr.authCheck(t, token); check["is_admin"] != true {
		t.Fatalf("precondition: expected admin, got %v", check)
	}

	// Demote in the DB — the very next request must see it.
	if err := tr.store.UpdateUserAdmin(context.Background(), userID, false); err != nil {
		t.Fatalf("UpdateUserAdmin: %v", err)
	}
	if check := tr.authCheck(t, token); check["is_admin"] != false {
		t.Errorf("PAT claims stale after demotion: %v", check)
	}
}

func TestDisabledUserBlockedEverywhere(t *testing.T) {
	tr := newTestRouter(t)
	tr.cliUser(t, "ernie", true)
	pat := tr.cliLogin(t, "ernie")

	if err := tr.store.SetUserDisabled(context.Background(), "ernie", true); err != nil {
		t.Fatalf("SetUserDisabled: %v", err)
	}

	// Existing PAT dies on the next request.
	if check := tr.authCheck(t, pat); check["authenticated"] != false {
		t.Errorf("disabled user's PAT still authenticates: %v", check)
	}

	// Web login: 403 account disabled (password is correct).
	w := tr.do("POST", "/api/auth/login",
		`{"username":"ernie","password":"password123"}`, "")
	if w.Code != http.StatusForbidden {
		t.Errorf("web login while disabled: want 403, got %d", w.Code)
	}

	// CLI login: same.
	w = tr.do("POST", "/api/auth/cli-login",
		`{"username":"ernie","password":"password123","name":"x"}`, "")
	if w.Code != http.StatusForbidden {
		t.Errorf("cli login while disabled: want 403, got %d", w.Code)
	}

	// Game login: same.
	w = tr.do("POST", "/api/auth/game-login",
		`{"username":"ernie","password":"password123"}`, "")
	if w.Code != http.StatusForbidden {
		t.Errorf("game login while disabled: want 403, got %d", w.Code)
	}

	// Re-enable: the un-revoked PAT works again.
	if err := tr.store.SetUserDisabled(context.Background(), "ernie", false); err != nil {
		t.Fatalf("SetUserDisabled(enable): %v", err)
	}
	if check := tr.authCheck(t, pat); check["authenticated"] != true {
		t.Errorf("re-enabled user's PAT rejected: %v", check)
	}
}

func TestPATAndJWTParityThroughMiddleware(t *testing.T) {
	tr := newTestRouter(t)
	tr.cliUser(t, "ernie", true)
	pat := tr.cliLogin(t, "ernie")
	jwt, _ := tr.loginAs(t, "other-admin", true)

	probe := func(token string) int {
		w := tr.do("GET", "/probe-admin", "", token)
		return w.Code
	}
	tr.r.mux.HandleFunc("GET /probe-admin", tr.r.requireAdmin(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	if got := probe(pat); got != http.StatusNoContent {
		t.Errorf("PAT through requireAdmin: want 204, got %d", got)
	}
	if got := probe(jwt); got != http.StatusNoContent {
		t.Errorf("JWT through requireAdmin: want 204, got %d", got)
	}
	if got := probe("trin_garbage"); got != http.StatusUnauthorized {
		t.Errorf("malformed PAT: want 401, got %d", got)
	}
	if got := probe(""); got != http.StatusUnauthorized {
		t.Errorf("anonymous: want 401, got %d", got)
	}
}
