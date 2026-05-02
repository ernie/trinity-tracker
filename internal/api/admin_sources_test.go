package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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

func TestHandleListApprovedSources(t *testing.T) {
	tr := newTestRouter(t)
	adminTok, _ := tr.loginAs(t, "admin", true)
	aliceTok, _ := tr.loginAs(t, "alice", false)

	mustOK(t, tr.requestSource(t, aliceTok, "alice-q3", ""))
	mustOK(t, tr.do("POST", "/api/admin/sources/alice-q3/approve", "", adminTok))

	w := tr.do("GET", "/api/admin/sources", "", adminTok)
	if w.Code != http.StatusOK {
		t.Fatalf("list approved: %d %s", w.Code, w.Body)
	}
	var entries []struct {
		Source string `json:"source"`
		Active bool   `json:"active"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &entries); err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, e := range entries {
		if e.Source == "alice-q3" {
			found = true
			if !e.Active {
				t.Errorf("alice-q3 should be active in approved list, got %+v", e)
			}
		}
	}
	if !found {
		t.Errorf("alice-q3 missing from approved list: %+v", entries)
	}
}

func TestHandleListApprovedSources_RequiresAdmin(t *testing.T) {
	tr := newTestRouter(t)
	tok, _ := tr.loginAs(t, "alice", false)
	w := tr.do("GET", "/api/admin/sources", "", tok)
	if w.Code != http.StatusForbidden {
		t.Errorf("non-admin code = %d, want 403", w.Code)
	}
}

func TestHandleCreateSource_HappyPath(t *testing.T) {
	tr := newTestRouter(t)
	adminTok, adminID := tr.loginAs(t, "admin", true)

	mintsBefore := tr.userProv.mintCalls
	body := fmt.Sprintf(`{"source":"hub-direct","owner_user_id":%d}`, adminID)
	w := tr.do("POST", "/api/admin/sources", body, adminTok)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}
	if tr.userProv.mintCalls <= mintsBefore {
		t.Error("create did not call MintUserCreds")
	}

	approved, err := tr.store.IsSourceApproved(context.Background(), "hub-direct")
	if err != nil || !approved {
		t.Errorf("hub-direct not approved after create (err=%v approved=%v)", err, approved)
	}
}

func TestHandleCreateSource_RequiresOwner(t *testing.T) {
	tr := newTestRouter(t)
	adminTok, _ := tr.loginAs(t, "admin", true)
	w := tr.do("POST", "/api/admin/sources", `{"source":"orphan"}`, adminTok)
	if w.Code != http.StatusBadRequest {
		t.Errorf("missing owner_user_id = %d, want 400", w.Code)
	}
}

func TestHandleCreateSource_RejectsUnknownOwner(t *testing.T) {
	tr := newTestRouter(t)
	adminTok, _ := tr.loginAs(t, "admin", true)
	w := tr.do("POST", "/api/admin/sources", `{"source":"phantom","owner_user_id":99999}`, adminTok)
	if w.Code != http.StatusBadRequest {
		t.Errorf("unknown owner = %d, want 400", w.Code)
	}
}

func TestHandleCreateSource_BadName(t *testing.T) {
	tr := newTestRouter(t)
	adminTok, adminID := tr.loginAs(t, "admin", true)
	body := fmt.Sprintf(`{"source":"contains spaces","owner_user_id":%d}`, adminID)
	w := tr.do("POST", "/api/admin/sources", body, adminTok)
	if w.Code != http.StatusBadRequest {
		t.Errorf("invalid name = %d, want 400", w.Code)
	}
}

func TestHandleCreateSource_RequiresAdmin(t *testing.T) {
	tr := newTestRouter(t)
	tok, uid := tr.loginAs(t, "alice", false)
	body := fmt.Sprintf(`{"source":"alice-attempt","owner_user_id":%d}`, uid)
	w := tr.do("POST", "/api/admin/sources", body, tok)
	if w.Code != http.StatusForbidden {
		t.Errorf("non-admin code = %d, want 403", w.Code)
	}
}

func TestHandleTransferSourceOwner_Reassigns(t *testing.T) {
	tr := newTestRouter(t)
	adminTok, adminID := tr.loginAs(t, "admin", true)
	_, aliceID := tr.loginAs(t, "alice", false)

	createBody := fmt.Sprintf(`{"source":"transfer-me","owner_user_id":%d}`, adminID)
	mustOK(t, tr.do("POST", "/api/admin/sources", createBody, adminTok))

	transferBody := fmt.Sprintf(`{"owner_user_id":%d}`, aliceID)
	w := tr.do("POST", "/api/admin/sources/transfer-me/owner", transferBody, adminTok)
	if w.Code != http.StatusNoContent {
		t.Fatalf("transfer: %d %s", w.Code, w.Body)
	}

	// Confirm the owner stuck.
	listed := tr.do("GET", "/api/admin/sources", "", adminTok)
	if !strings.Contains(listed.Body.String(), `"owner_username":"alice"`) {
		t.Errorf("expected alice as new owner; body = %s", listed.Body)
	}
}

func TestHandleTransferSourceOwner_RejectsUnknownUser(t *testing.T) {
	tr := newTestRouter(t)
	adminTok, adminID := tr.loginAs(t, "admin", true)

	createBody := fmt.Sprintf(`{"source":"orphan-test","owner_user_id":%d}`, adminID)
	mustOK(t, tr.do("POST", "/api/admin/sources", createBody, adminTok))

	w := tr.do("POST", "/api/admin/sources/orphan-test/owner", `{"owner_user_id":99999}`, adminTok)
	if w.Code != http.StatusBadRequest {
		t.Errorf("unknown user = %d, want 400", w.Code)
	}
}

func TestHandleTransferSourceOwner_RequiresAdmin(t *testing.T) {
	tr := newTestRouter(t)
	adminTok, adminID := tr.loginAs(t, "admin", true)
	aliceTok, aliceID := tr.loginAs(t, "alice", false)

	createBody := fmt.Sprintf(`{"source":"locked","owner_user_id":%d}`, adminID)
	mustOK(t, tr.do("POST", "/api/admin/sources", createBody, adminTok))

	body := fmt.Sprintf(`{"owner_user_id":%d}`, aliceID)
	w := tr.do("POST", "/api/admin/sources/locked/owner", body, aliceTok)
	if w.Code != http.StatusForbidden {
		t.Errorf("non-admin = %d, want 403", w.Code)
	}
}

func TestHandleReactivateSource_RestoresAndAudits(t *testing.T) {
	tr := newTestRouter(t)
	adminTok, _ := tr.loginAs(t, "admin", true)
	aliceTok, aliceID := tr.loginAs(t, "alice", false)

	mustOK(t, tr.requestSource(t, aliceTok, "alice-q3", ""))
	mustOK(t, tr.do("POST", "/api/admin/sources/alice-q3/approve", "", adminTok))
	mustOK(t, tr.do("POST", "/api/admin/sources/alice-q3/deactivate", "", adminTok))

	w := tr.do("POST", "/api/admin/sources/alice-q3/reactivate", "", adminTok)
	if w.Code != http.StatusNoContent {
		t.Fatalf("reactivate: %d %s", w.Code, w.Body)
	}

	approved, err := tr.store.IsSourceApproved(context.Background(), "alice-q3")
	if err != nil || !approved {
		t.Errorf("after reactivate: approved=%v err=%v", approved, err)
	}
	rows, _ := tr.store.ListSourcesByOwner(context.Background(), aliceID)
	if len(rows) != 1 || !rows[0].Active {
		t.Errorf("after reactivate: %+v", rows)
	}

	audit, _ := tr.store.ListSourceAudit(context.Background(), "alice-q3", 10)
	var sawReactivated bool
	for _, r := range audit {
		if r.Action == "reactivated" {
			sawReactivated = true
		}
	}
	if !sawReactivated {
		t.Errorf("no 'reactivated' audit row in %+v", audit)
	}
}

func TestHandleDownloadSourceCreds_AdminPath(t *testing.T) {
	tr := newTestRouter(t)
	adminTok, _ := tr.loginAs(t, "admin", true)
	aliceTok, _ := tr.loginAs(t, "alice", false)

	mustOK(t, tr.requestSource(t, aliceTok, "alice-q3", ""))
	mustOK(t, tr.do("POST", "/api/admin/sources/alice-q3/approve", "", adminTok))

	w := tr.do("GET", "/api/admin/sources/alice-q3/creds", "", adminTok)
	if w.Code != http.StatusOK {
		t.Fatalf("download: %d %s", w.Code, w.Body)
	}
	if !bytes.HasPrefix(w.Body.Bytes(), []byte("MINT:alice-q3:")) {
		t.Errorf("unexpected creds body: %q", w.Body.String())
	}
	if cd := w.Header().Get("Content-Disposition"); !strings.Contains(cd, "alice-q3.creds") {
		t.Errorf("Content-Disposition = %q", cd)
	}
}

func TestHandleDownloadSourceCreds_UnknownIs404(t *testing.T) {
	tr := newTestRouter(t)
	adminTok, _ := tr.loginAs(t, "admin", true)
	w := tr.do("GET", "/api/admin/sources/never-existed/creds", "", adminTok)
	if w.Code != http.StatusNotFound {
		t.Errorf("missing source code = %d, want 404", w.Code)
	}
}

func TestHandleRotateSourceCreds_AdminPath(t *testing.T) {
	tr := newTestRouter(t)
	adminTok, _ := tr.loginAs(t, "admin", true)
	aliceTok, _ := tr.loginAs(t, "alice", false)

	mustOK(t, tr.requestSource(t, aliceTok, "alice-q3", ""))
	mustOK(t, tr.do("POST", "/api/admin/sources/alice-q3/approve", "", adminTok))
	mintsAfterApprove := tr.userProv.mintCalls

	w := tr.do("POST", "/api/admin/sources/alice-q3/rotate-creds", "", adminTok)
	if w.Code != http.StatusOK {
		t.Fatalf("rotate: %d %s", w.Code, w.Body)
	}
	if tr.userProv.mintCalls != mintsAfterApprove+1 {
		t.Errorf("mint calls = %d, want %d", tr.userProv.mintCalls, mintsAfterApprove+1)
	}
	if !bytes.HasPrefix(w.Body.Bytes(), []byte("MINT:alice-q3:")) {
		t.Errorf("unexpected rotated creds body: %q", w.Body.String())
	}
}

func TestHandleRotateSourceCreds_UnknownIs404(t *testing.T) {
	tr := newTestRouter(t)
	adminTok, _ := tr.loginAs(t, "admin", true)
	w := tr.do("POST", "/api/admin/sources/never-existed/rotate-creds", "", adminTok)
	if w.Code != http.StatusNotFound {
		t.Errorf("missing source code = %d, want 404", w.Code)
	}
}

func TestHandleListAudit_GlobalAndFiltered(t *testing.T) {
	tr := newTestRouter(t)
	adminTok, _ := tr.loginAs(t, "admin", true)
	aliceTok, _ := tr.loginAs(t, "alice", false)
	bobTok, _ := tr.loginAs(t, "bob", false)

	mustOK(t, tr.requestSource(t, aliceTok, "alice-q3", ""))
	mustOK(t, tr.do("POST", "/api/admin/sources/alice-q3/approve", "", adminTok))
	mustOK(t, tr.requestSource(t, bobTok, "bob-q3", ""))
	mustOK(t, tr.do("POST", "/api/admin/sources/bob-q3/reject", `{"reason":"no"}`, adminTok))

	// Unfiltered: all four events present (2 requested + approved + rejected).
	w := tr.do("GET", "/api/admin/audit", "", adminTok)
	if w.Code != http.StatusOK {
		t.Fatalf("audit: %d %s", w.Code, w.Body)
	}
	var rows []auditEntryJSON
	if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) < 4 {
		t.Errorf("expected at least 4 audit rows, got %d: %+v", len(rows), rows)
	}

	// Filter by source.
	w = tr.do("GET", "/api/admin/audit?source=alice-q3", "", adminTok)
	_ = json.Unmarshal(w.Body.Bytes(), &rows)
	for _, r := range rows {
		if r.Source != "alice-q3" {
			t.Errorf("source filter leaked %q", r.Source)
		}
	}

	// Filter by action.
	w = tr.do("GET", "/api/admin/audit?action=rejected", "", adminTok)
	_ = json.Unmarshal(w.Body.Bytes(), &rows)
	if len(rows) != 1 || rows[0].Action != "rejected" || rows[0].Source != "bob-q3" {
		t.Errorf("action filter: %+v", rows)
	}

	// Filter by actor.
	w = tr.do("GET", "/api/admin/audit?actor=admin", "", adminTok)
	_ = json.Unmarshal(w.Body.Bytes(), &rows)
	for _, r := range rows {
		if r.ActorUsername != "admin" {
			t.Errorf("actor filter leaked actor %q", r.ActorUsername)
		}
	}

	// Bad since.
	w = tr.do("GET", "/api/admin/audit?since=yesterday", "", adminTok)
	if w.Code != http.StatusBadRequest {
		t.Errorf("bad since = %d, want 400", w.Code)
	}
}

func TestHandleListAudit_RequiresAdmin(t *testing.T) {
	tr := newTestRouter(t)
	tok, _ := tr.loginAs(t, "alice", false)
	w := tr.do("GET", "/api/admin/audit", "", tok)
	if w.Code != http.StatusForbidden {
		t.Errorf("non-admin code = %d, want 403", w.Code)
	}
}
