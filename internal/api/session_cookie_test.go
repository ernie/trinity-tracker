package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// webUser creates a user with the forced password change already
// cleared, ready for cookie logins. Returns the user ID.
func (tr *testRouter) webUser(t *testing.T, username string, isAdmin bool) int64 {
	t.Helper()
	return tr.cliUser(t, username, isAdmin)
}

// loginWeb performs a real POST /api/auth/login and returns the
// session cookie it set.
func (tr *testRouter) loginWeb(t *testing.T, username string) *http.Cookie {
	t.Helper()
	w := tr.do("POST", "/api/auth/login",
		`{"username":"`+username+`","password":"password123"}`, "")
	if w.Code != http.StatusOK {
		t.Fatalf("login: status %d body %s", w.Code, w.Body.String())
	}
	c := sessionCookieFrom(t, w)
	if c == nil {
		t.Fatal("login did not set the session cookie")
	}
	return c
}

// sessionCookieFrom extracts the trinity_session Set-Cookie from a
// response, nil if absent.
func sessionCookieFrom(t *testing.T, w *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range w.Result().Cookies() {
		if c.Name == sessionCookieName {
			return c
		}
	}
	return nil
}

// doWeb issues a request authenticated by the session cookie. origin
// sets an Origin header when non-empty. httptest requests carry
// Host: example.com, so "http://example.com" is the same-origin value.
func (tr *testRouter) doWeb(method, path, body string, cookie *http.Cookie, origin string) *httptest.ResponseRecorder {
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rdr)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	tr.r.ServeHTTP(w, req)
	return w
}

// authCheckWeb probes how a session cookie resolves via /api/auth/check.
func (tr *testRouter) authCheckWeb(t *testing.T, cookie *http.Cookie) map[string]any {
	t.Helper()
	w := tr.doWeb("GET", "/api/auth/check", "", cookie, "")
	if w.Code != http.StatusOK {
		t.Fatalf("auth/check: status %d", w.Code)
	}
	var out map[string]any
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

func TestLoginSetsSessionCookie(t *testing.T) {
	tr := newTestRouter(t)
	tr.webUser(t, "ernie", false)

	w := tr.do("POST", "/api/auth/login",
		`{"username":"ernie","password":"password123"}`, "")
	if w.Code != http.StatusOK {
		t.Fatalf("login: status %d", w.Code)
	}

	c := sessionCookieFrom(t, w)
	if c == nil {
		t.Fatal("no session cookie set")
	}
	if !c.HttpOnly {
		t.Error("cookie must be HttpOnly")
	}
	if c.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite: want Lax, got %v", c.SameSite)
	}
	if c.Path != "/" {
		t.Errorf("Path: want /, got %q", c.Path)
	}
	if c.MaxAge != sessionCookieMaxAge {
		t.Errorf("Max-Age: want %d, got %d", sessionCookieMaxAge, c.MaxAge)
	}
	if c.Secure {
		t.Error("plain-HTTP request must not get a Secure cookie")
	}

	// The JWT travels only in the cookie, never the body.
	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := body["token"]; ok {
		t.Error("login body must not carry the token")
	}

	// The cookie authenticates requests.
	if check := tr.authCheckWeb(t, c); check["authenticated"] != true {
		t.Errorf("cookie did not authenticate: %v", check)
	}
}

func TestSecureFlagFollowsForwardedProto(t *testing.T) {
	tr := newTestRouter(t)
	tr.webUser(t, "ernie", false)

	req := httptest.NewRequest("POST", "/api/auth/login",
		strings.NewReader(`{"username":"ernie","password":"password123"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()
	tr.r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("login: status %d", w.Code)
	}
	c := sessionCookieFrom(t, w)
	if c == nil || !c.Secure {
		t.Error("TLS-terminated request must get a Secure cookie")
	}
}

func TestLogoutExpiresCookie(t *testing.T) {
	tr := newTestRouter(t)
	tr.webUser(t, "ernie", false)
	c := tr.loginWeb(t, "ernie")

	w := tr.doWeb("POST", "/api/auth/logout", "", c, "")
	if w.Code != http.StatusOK {
		t.Fatalf("logout: status %d", w.Code)
	}
	expired := sessionCookieFrom(t, w)
	if expired == nil || expired.MaxAge >= 0 {
		t.Errorf("logout must send an expiring Set-Cookie, got %+v", expired)
	}
}

func TestBearerWinsOverCookie(t *testing.T) {
	tr := newTestRouter(t)
	tr.webUser(t, "webby", false)
	cookie := tr.loginWeb(t, "webby")
	tr.cliUser(t, "clive", true)
	pat := tr.cliLogin(t, "clive")

	// Valid PAT in the header + another user's cookie → the PAT user.
	req := httptest.NewRequest("GET", "/api/auth/check", nil)
	req.Header.Set("Authorization", "Bearer "+pat)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	tr.r.ServeHTTP(w, req)
	var out map[string]any
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["username"] != "clive" {
		t.Errorf("header must win over cookie: %v", out)
	}

	// Garbage Bearer + valid cookie: the header short-circuits.
	req = httptest.NewRequest("GET", "/api/auth/check", nil)
	req.Header.Set("Authorization", "Bearer garbage")
	req.AddCookie(cookie)
	w = httptest.NewRecorder()
	tr.r.ServeHTTP(w, req)
	out = map[string]any{}
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["authenticated"] != false {
		t.Errorf("invalid Bearer must not fall back to the cookie: %v", out)
	}
}

func TestCookieOriginCSRF(t *testing.T) {
	tr := newTestRouter(t)
	tr.webUser(t, "ernie", false)
	c := tr.loginWeb(t, "ernie")

	// Side-effect-free authenticated POST probe.
	tr.r.mux.HandleFunc("POST /probe-auth", tr.r.requireAuth(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	cases := []struct {
		name   string
		method string
		origin string
		want   int
	}{
		{"no origin (curl)", "POST", "", http.StatusNoContent},
		{"same origin", "POST", "http://example.com", http.StatusNoContent},
		{"cross origin", "POST", "https://evil.example", http.StatusUnauthorized},
		{"cross-origin GET passes", "GET", "https://evil.example", http.StatusOK},
	}
	for _, tc := range cases {
		path := "/probe-auth"
		if tc.method == "GET" {
			path = "/api/auth/check"
		}
		w := tr.doWeb(tc.method, path, "", c, tc.origin)
		if w.Code != tc.want {
			t.Errorf("%s: want %d, got %d", tc.name, tc.want, w.Code)
		}
	}

	// Bearer requests are exempt: cookies are the CSRF vector, headers
	// can't be attached cross-site.
	tr.cliUser(t, "clive", false)
	pat := tr.cliLogin(t, "clive")
	req := httptest.NewRequest("POST", "/probe-auth", nil)
	req.Header.Set("Authorization", "Bearer "+pat)
	req.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	tr.r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("Bearer with foreign Origin: want 204, got %d", w.Code)
	}
}

func TestLegacyUnversionedJWTRejected(t *testing.T) {
	tr := newTestRouter(t)
	_, userID := tr.loginAs(t, "oldtimer", false)

	// A pre-versioning token carries token_ver 0; live rows start at 1.
	user, err := tr.store.GetUserByID(context.Background(), userID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	legacy, err := tr.auth.GenerateToken(user.ID, user.Username, user.IsAdmin, user.PlayerID, false, 0)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	if check := tr.authCheck(t, legacy); check["authenticated"] != false {
		t.Errorf("legacy unversioned JWT still authenticates: %v", check)
	}
}
