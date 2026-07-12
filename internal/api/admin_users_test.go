package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func patchDisplayName(tr *testRouter, token string, userID int64, name string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"display_name": name})
	return tr.do("PATCH", fmt.Sprintf("/api/users/%d", userID), string(body), token)
}

func TestUpdateUserDisplayName(t *testing.T) {
	tr := newTestRouter(t)
	adminTok, _ := tr.loginAs(t, "admin", true)
	_, targetID := tr.loginAs(t, "target", false)
	_, otherID := tr.loginAs(t, "other", false)

	if w := patchDisplayName(tr, adminTok, targetID, "^1Neo"); w.Code != http.StatusOK {
		t.Fatalf("set: code = %d, body = %s", w.Code, w.Body.String())
	}
	user, err := tr.store.GetUserByID(context.Background(), targetID)
	if err != nil {
		t.Fatal(err)
	}
	if user.DisplayName != "^1Neo" {
		t.Fatalf("DisplayName = %q, want ^1Neo", user.DisplayName)
	}

	w := tr.do("GET", "/api/users", "", adminTok)
	if w.Code != http.StatusOK {
		t.Fatalf("list: code = %d", w.Code)
	}
	var listed []UserResponse
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, u := range listed {
		if u.ID == targetID {
			found = true
			if u.DisplayName != "^1Neo" {
				t.Fatalf("listed DisplayName = %q, want ^1Neo", u.DisplayName)
			}
		}
	}
	if !found {
		t.Fatal("target user missing from list")
	}

	// Same canonical as target's name.
	if w := patchDisplayName(tr, adminTok, otherID, "^4Neo"); w.Code != http.StatusConflict {
		t.Fatalf("taken: code = %d, want 409", w.Code)
	}

	if w := patchDisplayName(tr, adminTok, otherID, "Sarge"); w.Code != http.StatusConflict {
		t.Fatalf("reserved: code = %d, want 409", w.Code)
	}

	if w := patchDisplayName(tr, adminTok, otherID, "   "); w.Code != http.StatusBadRequest {
		t.Fatalf("empty: code = %d, want 400", w.Code)
	}

	// Cleans non-empty, canonicalizes to empty.
	if w := patchDisplayName(tr, adminTok, otherID, "[VR]"); w.Code != http.StatusBadRequest {
		t.Fatalf("canonical-empty: code = %d, want 400", w.Code)
	}

	nonAdminTok, _ := tr.loginAs(t, "peon", false)
	if w := patchDisplayName(tr, nonAdminTok, targetID, "Morpheus"); w.Code != http.StatusForbidden {
		t.Fatalf("non-admin: code = %d, want 403", w.Code)
	}
}

func TestUpdateUserCombinedPatch(t *testing.T) {
	tr := newTestRouter(t)
	adminTok, _ := tr.loginAs(t, "admin", true)
	_, targetID := tr.loginAs(t, "target", false)

	body := `{"is_admin":true,"display_name":"^2Tank"}`
	w := tr.do("PATCH", fmt.Sprintf("/api/users/%d", targetID), body, adminTok)
	if w.Code != http.StatusOK {
		t.Fatalf("combined patch: code = %d, body = %s", w.Code, w.Body.String())
	}
	user, err := tr.store.GetUserByID(context.Background(), targetID)
	if err != nil {
		t.Fatal(err)
	}
	if !user.IsAdmin || user.DisplayName != "^2Tank" {
		t.Fatalf("IsAdmin = %v, DisplayName = %q; want true, ^2Tank", user.IsAdmin, user.DisplayName)
	}
}
