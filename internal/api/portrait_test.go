package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// seedPortraitAsset creates <staticDir>/assets/portraits/sarge/icon_krusade.png
// and points the router's staticDir at it.
func seedPortraitAsset(t *testing.T, tr *testRouter) {
	t.Helper()
	staticDir := t.TempDir()
	iconDir := filepath.Join(staticDir, "assets", "portraits", "sarge")
	if err := os.MkdirAll(iconDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(iconDir, "icon_krusade.png"), []byte("png"), 0644); err != nil {
		t.Fatal(err)
	}
	tr.r.staticDir = staticDir
}

func TestSetPortrait_Unauthenticated(t *testing.T) {
	tr := newTestRouter(t)
	w := tr.do("PATCH", "/api/account/portrait", `{"portrait":"sarge/krusade"}`, "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("code = %d, want 401", w.Code)
	}
}

func TestSetPortrait_RejectsInvalidValues(t *testing.T) {
	tr := newTestRouter(t)
	seedPortraitAsset(t, tr)
	tok, _ := tr.loginAs(t, "alice", false)

	for _, bad := range []string{
		`{"portrait":"sarge"}`,
		`{"portrait":"sarge/krusade/extra"}`,
		`{"portrait":"../../etc/passwd"}`,
		`{"portrait":"sarge/.."}`,
		`{"portrait":"sarge/missing"}`,
		`{"portrait":"xaero/krusade"}`,
	} {
		w := tr.do("PATCH", "/api/account/portrait", bad, tok)
		if w.Code != http.StatusBadRequest {
			t.Errorf("body %s: code = %d, want 400", bad, w.Code)
		}
	}
}

func TestSetPortrait_SetAndClear(t *testing.T) {
	tr := newTestRouter(t)
	seedPortraitAsset(t, tr)
	tok, _ := tr.loginAs(t, "alice", false)

	// Mixed case normalizes to the lowercase on-disk form.
	w := tr.do("PATCH", "/api/account/portrait", `{"portrait":"Sarge/Krusade"}`, tok)
	if w.Code != http.StatusNoContent {
		t.Fatalf("set: code = %d, body = %s", w.Code, w.Body)
	}

	profile := func() map[string]json.RawMessage {
		w := tr.do("GET", "/api/account/profile", "", tok)
		if w.Code != http.StatusOK {
			t.Fatalf("profile: code = %d", w.Code)
		}
		var resp struct {
			User map[string]json.RawMessage `json:"user"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		return resp.User
	}

	if got := string(profile()["portrait"]); got != `"sarge/krusade"` {
		t.Fatalf("profile portrait = %s, want \"sarge/krusade\"", got)
	}

	w = tr.do("PATCH", "/api/account/portrait", `{"portrait":null}`, tok)
	if w.Code != http.StatusNoContent {
		t.Fatalf("clear: code = %d, body = %s", w.Code, w.Body)
	}
	if _, present := profile()["portrait"]; present {
		t.Fatal("cleared portrait still present in profile response")
	}

	w = tr.do("PATCH", "/api/account/portrait", `{"portrait":"sarge/krusade"}`, tok)
	if w.Code != http.StatusNoContent {
		t.Fatalf("re-set: code = %d, body = %s", w.Code, w.Body)
	}
	if got := string(profile()["portrait"]); got != `"sarge/krusade"` {
		t.Fatalf("re-set portrait = %s, want \"sarge/krusade\"", got)
	}

	// An absent field clears the portrait, same as explicit null.
	w = tr.do("PATCH", "/api/account/portrait", `{}`, tok)
	if w.Code != http.StatusNoContent {
		t.Fatalf("absent-field clear: code = %d, body = %s", w.Code, w.Body)
	}
	if _, present := profile()["portrait"]; present {
		t.Fatal("absent-field patch left portrait present in profile response")
	}
}
