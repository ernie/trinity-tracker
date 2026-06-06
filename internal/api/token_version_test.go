package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/ernie/trinity-tracker/internal/auth"
)

func TestVersionBumpRevokesSession(t *testing.T) {
	tr := newTestRouter(t)
	userID := tr.webUser(t, "ernie", false)
	c := tr.loginWeb(t, "ernie")

	if check := tr.authCheckWeb(t, c); check["authenticated"] != true {
		t.Fatalf("precondition: cookie should authenticate: %v", check)
	}

	if err := tr.store.BumpUserTokenVersion(context.Background(), userID); err != nil {
		t.Fatalf("BumpUserTokenVersion: %v", err)
	}
	if check := tr.authCheckWeb(t, c); check["authenticated"] != false {
		t.Errorf("bumped version must kill the session: %v", check)
	}

	// Fresh login works and carries the new version.
	c2 := tr.loginWeb(t, "ernie")
	if check := tr.authCheckWeb(t, c2); check["authenticated"] != true {
		t.Errorf("re-login after bump failed: %v", check)
	}
}

func TestPasswordChangeRevokesOtherSessions(t *testing.T) {
	tr := newTestRouter(t)
	tr.webUser(t, "ernie", false)
	browserA := tr.loginWeb(t, "ernie")
	browserB := tr.loginWeb(t, "ernie")

	w := tr.doWeb("POST", "/api/auth/change-password",
		`{"current_password":"password123","new_password":"password456"}`,
		browserA, "")
	if w.Code != http.StatusOK {
		t.Fatalf("change-password: status %d body %s", w.Code, w.Body.String())
	}

	// The other browser dies; the changer's re-issued cookie lives on.
	if check := tr.authCheckWeb(t, browserB); check["authenticated"] != false {
		t.Errorf("other session survived a password change: %v", check)
	}
	reissued := sessionCookieFrom(t, w)
	if reissued == nil {
		t.Fatal("change-password must re-issue the session cookie")
	}
	if check := tr.authCheckWeb(t, reissued); check["authenticated"] != true {
		t.Errorf("changer's re-issued session rejected: %v", check)
	}
	// The pre-change cookie is dead too — only the re-issue survives.
	if check := tr.authCheckWeb(t, browserA); check["authenticated"] != false {
		t.Errorf("pre-change cookie survived: %v", check)
	}
}

func TestAdminResetRevokesSessions(t *testing.T) {
	tr := newTestRouter(t)
	userID := tr.webUser(t, "ernie", false)
	c := tr.loginWeb(t, "ernie")

	// Same storage write the admin API and `trinity user reset` use.
	hash, err := auth.HashPassword("temppass123")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if err := tr.store.ResetUserPassword(context.Background(), userID, hash); err != nil {
		t.Fatalf("ResetUserPassword: %v", err)
	}
	if check := tr.authCheckWeb(t, c); check["authenticated"] != false {
		t.Errorf("session survived an admin password reset: %v", check)
	}
}

func TestLogoutAllRevokesEverything(t *testing.T) {
	tr := newTestRouter(t)
	tr.webUser(t, "ernie", false)
	browserA := tr.loginWeb(t, "ernie")
	browserB := tr.loginWeb(t, "ernie")

	w := tr.doWeb("POST", "/api/auth/logout-all", "", browserA, "")
	if w.Code != http.StatusOK {
		t.Fatalf("logout-all: status %d body %s", w.Code, w.Body.String())
	}
	if expired := sessionCookieFrom(t, w); expired == nil || expired.MaxAge >= 0 {
		t.Errorf("logout-all must expire the device's cookie, got %+v", expired)
	}
	if check := tr.authCheckWeb(t, browserA); check["authenticated"] != false {
		t.Errorf("initiating session survived logout-all: %v", check)
	}
	if check := tr.authCheckWeb(t, browserB); check["authenticated"] != false {
		t.Errorf("other session survived logout-all: %v", check)
	}

	// Unauthenticated logout-all is refused.
	w = tr.doWeb("POST", "/api/auth/logout-all", "", nil, "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("anonymous logout-all: want 401, got %d", w.Code)
	}
}

func TestPlainLogoutIsPerDevice(t *testing.T) {
	tr := newTestRouter(t)
	tr.webUser(t, "ernie", false)
	browserA := tr.loginWeb(t, "ernie")
	browserB := tr.loginWeb(t, "ernie")

	w := tr.doWeb("POST", "/api/auth/logout", "", browserA, "")
	if w.Code != http.StatusOK {
		t.Fatalf("logout: status %d", w.Code)
	}

	// No bump: the other browser keeps its session. (The JWT in A's
	// cookie remains valid server-side too — plain logout is purely
	// the cookie removal; the version is the revocation lever.)
	if check := tr.authCheckWeb(t, browserB); check["authenticated"] != true {
		t.Errorf("plain logout must not touch other sessions: %v", check)
	}
}

func TestDisabledUserSessionDies(t *testing.T) {
	tr := newTestRouter(t)
	tr.webUser(t, "ernie", false)
	c := tr.loginWeb(t, "ernie")

	if err := tr.store.SetUserDisabled(context.Background(), "ernie", true); err != nil {
		t.Fatalf("SetUserDisabled: %v", err)
	}
	if check := tr.authCheckWeb(t, c); check["authenticated"] != false {
		t.Errorf("disabled user's live JWT still authenticates: %v", check)
	}

	// Re-enable: no bump happened, the cookie works again (PAT parity).
	if err := tr.store.SetUserDisabled(context.Background(), "ernie", false); err != nil {
		t.Fatalf("SetUserDisabled(enable): %v", err)
	}
	if check := tr.authCheckWeb(t, c); check["authenticated"] != true {
		t.Errorf("re-enabled user's session rejected: %v", check)
	}
}

func TestAdminFlipIsLiveThroughJWT(t *testing.T) {
	tr := newTestRouter(t)
	userID := tr.webUser(t, "ernie", true)
	c := tr.loginWeb(t, "ernie")

	probe := func() int {
		// GET /api/users is requireAdmin.
		return tr.doWeb("GET", "/api/users", "", c, "").Code
	}
	if got := probe(); got != http.StatusOK {
		t.Fatalf("admin probe: want 200, got %d", got)
	}

	// Demotion bites on the next request, no re-login.
	if err := tr.store.UpdateUserAdmin(context.Background(), userID, false); err != nil {
		t.Fatalf("UpdateUserAdmin: %v", err)
	}
	if got := probe(); got != http.StatusForbidden {
		t.Errorf("demoted admin: want 403, got %d", got)
	}
	if err := tr.store.UpdateUserAdmin(context.Background(), userID, true); err != nil {
		t.Fatalf("UpdateUserAdmin: %v", err)
	}
	if got := probe(); got != http.StatusOK {
		t.Errorf("re-promoted admin: want 200, got %d", got)
	}
}
