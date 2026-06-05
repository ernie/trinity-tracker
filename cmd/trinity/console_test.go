package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestCLITokenRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// Absent is a state, not an error.
	tok, err := loadCLIToken()
	if err != nil || tok != nil {
		t.Fatalf("absent token: got (%v, %v), want (nil, nil)", tok, err)
	}

	want := &cliToken{URL: "https://hub.example", Username: "ernie", Token: "trin_abc"}
	if err := saveCLIToken(want); err != nil {
		t.Fatalf("saveCLIToken: %v", err)
	}

	path, _ := tokenPath()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("token file mode: got %o, want 600", perm)
	}
	if dirInfo, err := os.Stat(filepath.Dir(path)); err == nil {
		if perm := dirInfo.Mode().Perm(); perm != 0o700 {
			t.Errorf("token dir mode: got %o, want 700", perm)
		}
	}

	got, err := loadCLIToken()
	if err != nil {
		t.Fatalf("loadCLIToken: %v", err)
	}
	if *got != *want {
		t.Errorf("round trip: got %+v, want %+v", got, want)
	}

	if err := deleteCLIToken(); err != nil {
		t.Fatalf("deleteCLIToken: %v", err)
	}
	if err := deleteCLIToken(); err != nil {
		t.Errorf("delete is not idempotent: %v", err)
	}
	if tok, _ := loadCLIToken(); tok != nil {
		t.Error("token survived delete")
	}
}

func TestRevokeToken(t *testing.T) {
	var sawAuth string
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/auth/cli-login" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		sawAuth = r.Header.Get("Authorization")
		w.WriteHeader(status)
	}))
	defer srv.Close()

	tok := &cliToken{URL: srv.URL, Username: "e", Token: "trin_x"}

	if err := revokeToken(tok); err != nil {
		t.Errorf("200 revoke: %v", err)
	}
	if sawAuth != "Bearer trin_x" {
		t.Errorf("auth header: %q", sawAuth)
	}

	// 401 = already dead = success for logout purposes.
	status = http.StatusUnauthorized
	if err := revokeToken(tok); err != nil {
		t.Errorf("401 revoke: %v", err)
	}

	status = http.StatusInternalServerError
	if err := revokeToken(tok); err == nil {
		t.Error("500 revoke: want error")
	}
}

func TestMatchTarget(t *testing.T) {
	servers := []consoleServer{
		{Source: "hub-q3", Key: "ffa"},
		{Source: "hub-q3", Key: "ctf"},
		{Source: "alice-q3", Key: "ctf"},
	}

	// Qualified target passes through without consulting the list.
	if s, k, _ := matchTarget(servers, "eu/duel"); s != "eu" || k != "duel" {
		t.Errorf("qualified: got %s/%s", s, k)
	}

	// Unique bare key resolves (case-insensitive, like server keys).
	if s, k, _ := matchTarget(servers, "FFA"); s != "hub-q3" || k != "ffa" {
		t.Errorf("unique: got %s/%s", s, k)
	}

	// Ambiguous bare key: no resolution, candidates returned.
	s, _, matches := matchTarget(servers, "ctf")
	if s != "" || len(matches) != 2 {
		t.Errorf("ambiguous: got source %q, %d matches", s, len(matches))
	}

	// Unknown key: no resolution, no candidates.
	s, _, matches = matchTarget(servers, "nope")
	if s != "" || len(matches) != 0 {
		t.Errorf("unknown: got source %q, %d matches", s, len(matches))
	}
}
