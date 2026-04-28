package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestHandleListPendingSources_OnlyPending(t *testing.T) {
	tr := newTestRouter(t)
	adminTok, _ := tr.loginAs(t, "admin", true)
	aliceTok, _ := tr.loginAs(t, "alice", false)
	bobTok, _ := tr.loginAs(t, "bob", false)

	mustOK(t, tr.requestSource(t, aliceTok, "alice-q3", "for fun"))
	mustOK(t, tr.requestSource(t, bobTok, "bob-q3", ""))
	// Approve alice → only bob should appear in pending list.
	if w := tr.do("POST", "/api/admin/sources/alice-q3/approve", "", adminTok); w.Code != http.StatusNoContent {
		t.Fatalf("approve alice: %d %s", w.Code, w.Body)
	}

	w := tr.do("GET", "/api/admin/sources/pending", "", adminTok)
	if w.Code != http.StatusOK {
		t.Fatalf("list pending: %d %s", w.Code, w.Body)
	}
	var rows []pendingSourceRow
	if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1: %+v", len(rows), rows)
	}
	if rows[0].Source != "bob-q3" || rows[0].OwnerUsername != "bob" {
		t.Errorf("unexpected row: %+v", rows[0])
	}
}

func TestHandleListPendingSources_RequiresAdmin(t *testing.T) {
	tr := newTestRouter(t)
	tok, _ := tr.loginAs(t, "alice", false)
	w := tr.do("GET", "/api/admin/sources/pending", "", tok)
	if w.Code != http.StatusForbidden {
		t.Errorf("non-admin code = %d, want 403", w.Code)
	}
}

func TestHandleApproveSource_FlipsToActiveAndMints(t *testing.T) {
	tr := newTestRouter(t)
	adminTok, _ := tr.loginAs(t, "admin", true)
	aliceTok, aliceID := tr.loginAs(t, "alice", false)

	mustOK(t, tr.requestSource(t, aliceTok, "alice-q3", ""))
	mintsBefore := tr.userProv.mintCalls

	w := tr.do("POST", "/api/admin/sources/alice-q3/approve", "", adminTok)
	if w.Code != http.StatusNoContent {
		t.Fatalf("approve: %d %s", w.Code, w.Body)
	}
	if tr.userProv.mintCalls <= mintsBefore {
		t.Error("approve did not call MintUserCreds")
	}
	rows, _ := tr.store.ListSourcesByOwner(context.Background(), aliceID)
	if len(rows) != 1 || rows[0].Status != "active" {
		t.Errorf("unexpected state: %+v", rows)
	}

	audit, _ := tr.store.ListSourceAudit(context.Background(), "alice-q3", 10)
	var sawApproved bool
	for _, r := range audit {
		if r.Action == "approved" {
			sawApproved = true
		}
	}
	if !sawApproved {
		t.Errorf("no 'approved' audit row in %+v", audit)
	}
}

func TestHandleApproveSource_NotPendingIs409(t *testing.T) {
	tr := newTestRouter(t)
	adminTok, _ := tr.loginAs(t, "admin", true)
	w := tr.do("POST", "/api/admin/sources/never-existed/approve", "", adminTok)
	if w.Code != http.StatusConflict {
		t.Errorf("approve missing = %d, want 409", w.Code)
	}
}

func TestHandleRejectSource_StoresReason(t *testing.T) {
	tr := newTestRouter(t)
	adminTok, _ := tr.loginAs(t, "admin", true)
	aliceTok, aliceID := tr.loginAs(t, "alice", false)

	mustOK(t, tr.requestSource(t, aliceTok, "alice-q3", ""))
	body := `{"reason":"name is reserved"}`
	w := tr.do("POST", "/api/admin/sources/alice-q3/reject", body, adminTok)
	if w.Code != http.StatusNoContent {
		t.Fatalf("reject: %d %s", w.Code, w.Body)
	}
	rows, _ := tr.store.ListSourcesByOwner(context.Background(), aliceID)
	if len(rows) != 1 || rows[0].Status != "rejected" || rows[0].RejectionReason != "name is reserved" {
		t.Errorf("after reject: %+v", rows)
	}

	// Caller can re-fetch /api/sources/mine and see the reason for the modal.
	resp := tr.do("GET", "/api/sources/mine", "", aliceTok)
	var env mySourcesEnvelope
	_ = json.Unmarshal(resp.Body.Bytes(), &env)
	if len(env.Sources) != 1 || env.Sources[0].RejectionReason != "name is reserved" {
		t.Errorf("envelope = %+v", env)
	}
}

func TestHandleRejectSource_RequiresReason(t *testing.T) {
	tr := newTestRouter(t)
	adminTok, _ := tr.loginAs(t, "admin", true)
	aliceTok, _ := tr.loginAs(t, "alice", false)
	mustOK(t, tr.requestSource(t, aliceTok, "alice-q3", ""))
	w := tr.do("POST", "/api/admin/sources/alice-q3/reject", `{"reason":"   "}`, adminTok)
	if w.Code != http.StatusBadRequest {
		t.Errorf("blank reason = %d, want 400", w.Code)
	}
}

func TestHandleRenamePendingSource_HappyPath(t *testing.T) {
	tr := newTestRouter(t)
	adminTok, _ := tr.loginAs(t, "admin", true)
	aliceTok, aliceID := tr.loginAs(t, "alice", false)

	mustOK(t, tr.requestSource(t, aliceTok, "old-name", ""))

	body := `{"name":"better-name"}`
	w := tr.do("POST", "/api/admin/sources/old-name/rename", body, adminTok)
	if w.Code != http.StatusNoContent {
		t.Fatalf("rename = %d %s", w.Code, w.Body)
	}

	rows, _ := tr.store.ListSourcesByOwner(context.Background(), aliceID)
	if len(rows) != 1 || rows[0].Source != "better-name" || rows[0].Status != "pending" {
		t.Errorf("after rename: %+v", rows)
	}
}

func TestHandleRenamePendingSource_NotPendingIs409(t *testing.T) {
	tr := newTestRouter(t)
	adminTok, _ := tr.loginAs(t, "admin", true)
	aliceTok, _ := tr.loginAs(t, "alice", false)

	mustOK(t, tr.requestSource(t, aliceTok, "host", ""))
	mustOK(t, tr.do("POST", "/api/admin/sources/host/approve", "", adminTok))

	w := tr.do("POST", "/api/admin/sources/host/rename", `{"name":"renamed"}`, adminTok)
	if w.Code != http.StatusConflict {
		t.Errorf("rename of active = %d, want 409", w.Code)
	}
}

func TestHandleRenamePendingSource_NameTakenIs409(t *testing.T) {
	tr := newTestRouter(t)
	adminTok, _ := tr.loginAs(t, "admin", true)
	aliceTok, _ := tr.loginAs(t, "alice", false)
	bobTok, _ := tr.loginAs(t, "bob", false)

	mustOK(t, tr.requestSource(t, aliceTok, "alice-q3", ""))
	mustOK(t, tr.do("POST", "/api/admin/sources/alice-q3/approve", "", adminTok))
	mustOK(t, tr.requestSource(t, bobTok, "bob-q3", ""))

	w := tr.do("POST", "/api/admin/sources/bob-q3/rename", `{"name":"alice-q3"}`, adminTok)
	if w.Code != http.StatusConflict {
		t.Errorf("rename to taken = %d, want 409", w.Code)
	}
}

func TestHandleDeactivateSource_SetsStatusRevoked(t *testing.T) {
	tr := newTestRouter(t)
	adminTok, _ := tr.loginAs(t, "admin", true)
	aliceTok, aliceID := tr.loginAs(t, "alice", false)

	mustOK(t, tr.requestSource(t, aliceTok, "alice-q3", ""))
	if w := tr.do("POST", "/api/admin/sources/alice-q3/approve", "", adminTok); w.Code != http.StatusNoContent {
		t.Fatalf("approve: %d", w.Code)
	}
	w := tr.do("POST", "/api/admin/sources/alice-q3/deactivate", "", adminTok)
	if w.Code != http.StatusOK && w.Code != http.StatusNoContent {
		t.Fatalf("deactivate: %d %s", w.Code, w.Body)
	}
	rows, _ := tr.store.ListSourcesByOwner(context.Background(), aliceID)
	if len(rows) != 1 || rows[0].Status != "revoked" {
		t.Errorf("unexpected state: %+v", rows)
	}
}
