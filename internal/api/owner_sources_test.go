package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ernie/trinity-tracker/internal/auth"
	"github.com/ernie/trinity-tracker/internal/hub"
	"github.com/ernie/trinity-tracker/internal/storage"
)

// fakeUserProv is the api-test stand-in for the embedded NATS auth
// store. It records mint/revoke calls and serves an in-memory creds
// blob so the .creds download path returns deterministic bytes.
type fakeUserProv struct {
	mu          sync.Mutex
	creds       map[string][]byte
	mintCalls   int
	revokeCalls int
	credsDir    string
}

func newFakeUserProv(t *testing.T) *fakeUserProv {
	return &fakeUserProv{
		creds:    make(map[string][]byte),
		credsDir: t.TempDir(),
	}
}

func (f *fakeUserProv) MintUserCreds(source string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mintCalls++
	blob := []byte(fmt.Sprintf("MINT:%s:%d", source, f.mintCalls))
	f.creds[source] = blob
	if err := os.WriteFile(f.credsPathLocked(source), blob, 0600); err != nil {
		return nil, err
	}
	return blob, nil
}

func (f *fakeUserProv) RevokeSource(source string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.revokeCalls++
	delete(f.creds, source)
	return nil
}

func (f *fakeUserProv) CredsPath(source string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.credsPathLocked(source)
}

func (f *fakeUserProv) credsPathLocked(source string) string {
	return filepath.Join(f.credsDir, source+".creds")
}

// testRouter is the test harness: real Store with the project schema,
// real auth.Service, real Writer, fake UserProvisioner. All five
// owner endpoints exercise the same mux as production.
type testRouter struct {
	r        *Router
	store    *storage.Store
	auth     *auth.Service
	userProv *fakeUserProv
}

func newTestRouter(t *testing.T) *testRouter {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := storage.New(dbPath)
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	authSvc := auth.NewService("test-secret-do-not-ship", time.Hour)
	writer := hub.NewWriter(store)
	r := NewRouter(store, nil, writer, authSvc, "", "")
	prov := newFakeUserProv(t)
	r.SetUserProvisioner(prov)
	return &testRouter{r: r, store: store, auth: authSvc, userProv: prov}
}

// loginAs creates a user (admin or not) and returns a JWT token.
// Tests use this to drive Authorization headers.
func (tr *testRouter) loginAs(t *testing.T, username string, isAdmin bool) (token string, userID int64) {
	t.Helper()
	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if err := tr.store.CreateUser(context.Background(), username, hash, isAdmin, nil); err != nil {
		t.Fatalf("create user: %v", err)
	}
	user, err := tr.store.GetUserByUsername(context.Background(), username)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	tok, err := tr.auth.GenerateToken(user.ID, user.Username, user.IsAdmin, user.PlayerID, false)
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	return tok, user.ID
}

// do issues a request with a Bearer token and returns the recorder.
func (tr *testRouter) do(method, path, body, token string) *httptest.ResponseRecorder {
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rdr)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	tr.r.ServeHTTP(w, req)
	return w
}

// requestSource is a helper that POSTs the request modal payload.
func (tr *testRouter) requestSource(t *testing.T, token, name, purpose string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]string{
		"name": name, "purpose": purpose,
	})
	return tr.do("POST", "/api/sources/request", string(body), token)
}

func TestHandleGetMySources_Unauthenticated(t *testing.T) {
	tr := newTestRouter(t)
	w := tr.do("GET", "/api/sources/mine", "", "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("code = %d, want 401", w.Code)
	}
}

func TestHandleGetMySources_EmptyForUserWithNone(t *testing.T) {
	tr := newTestRouter(t)
	tok, _ := tr.loginAs(t, "alice", false)
	w := tr.do("GET", "/api/sources/mine", "", tok)
	if w.Code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", w.Code, w.Body)
	}
	var got mySourcesEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Sources) != 0 || got.HasPending {
		t.Errorf("envelope = %+v, want empty + no pending", got)
	}
}

func TestHandleGetMySources_TwoActivesAndPending(t *testing.T) {
	tr := newTestRouter(t)
	tok, _ := tr.loginAs(t, "alice", false)

	// Two approved + one pending — the realistic multi-collector case.
	mustOK(t, tr.requestSource(t, tok, "host-a", "first machine"))
	must(t, tr.store.ApproveSource(context.Background(), "host-a"))
	mustOK(t, tr.requestSource(t, tok, "host-b", "second machine"))
	must(t, tr.store.ApproveSource(context.Background(), "host-b"))
	mustOK(t, tr.requestSource(t, tok, "host-c", "third pending"))

	w := tr.do("GET", "/api/sources/mine", "", tok)
	if w.Code != http.StatusOK {
		t.Fatalf("code = %d", w.Code)
	}
	var env mySourcesEnvelope
	_ = json.Unmarshal(w.Body.Bytes(), &env)
	if len(env.Sources) != 3 {
		t.Fatalf("got %d sources, want 3: %+v", len(env.Sources), env)
	}
	if !env.HasPending {
		t.Error("HasPending = false, want true")
	}
	// active rows ordered first by ListSourcesByOwner.
	if env.Sources[0].Status != "active" || env.Sources[1].Status != "active" {
		t.Errorf("expected two actives first, got %+v", env.Sources)
	}
	if env.Sources[2].Status != "pending" {
		t.Errorf("expected pending last, got %+v", env.Sources[2])
	}
}

func TestHandleRequestSource_Pending(t *testing.T) {
	tr := newTestRouter(t)
	tok, uid := tr.loginAs(t, "alice", false)

	w := tr.requestSource(t, tok, "alice-q3", "for fun")
	if w.Code != http.StatusCreated {
		t.Fatalf("code = %d, body = %s", w.Code, w.Body)
	}
	rows, err := tr.store.ListSourcesByOwner(context.Background(), uid)
	if err != nil || len(rows) != 1 || rows[0].Status != "pending" {
		t.Errorf("unexpected state: %+v err=%v", rows, err)
	}
	audit, _ := tr.store.ListSourceAudit(context.Background(), "alice-q3", 10)
	if len(audit) != 1 || audit[0].Action != "requested" {
		t.Errorf("audit = %+v", audit)
	}
}

func TestHandleRequestSource_DuplicatePending(t *testing.T) {
	tr := newTestRouter(t)
	tok, _ := tr.loginAs(t, "alice", false)
	mustOK(t, tr.requestSource(t, tok, "first", ""))
	w := tr.requestSource(t, tok, "second", "")
	if w.Code != http.StatusConflict {
		t.Errorf("second submission code = %d, want 409", w.Code)
	}
}

func TestHandleRequestSource_AfterApproveAllowsAnother(t *testing.T) {
	tr := newTestRouter(t)
	tok, _ := tr.loginAs(t, "alice", false)
	mustOK(t, tr.requestSource(t, tok, "host-a", ""))
	must(t, tr.store.ApproveSource(context.Background(), "host-a"))

	w := tr.requestSource(t, tok, "host-b", "")
	if w.Code != http.StatusCreated {
		t.Errorf("second request after approve = %d, want 201 body=%s", w.Code, w.Body)
	}
}

func TestHandleRequestSource_NameTakenByOther(t *testing.T) {
	tr := newTestRouter(t)
	tokA, _ := tr.loginAs(t, "alice", false)
	tokB, _ := tr.loginAs(t, "bob", false)
	mustOK(t, tr.requestSource(t, tokA, "shared", ""))
	w := tr.requestSource(t, tokB, "shared", "")
	if w.Code != http.StatusConflict {
		t.Errorf("bob code = %d, want 409", w.Code)
	}
}

func TestHandleRequestSource_InvalidName(t *testing.T) {
	tr := newTestRouter(t)
	tok, _ := tr.loginAs(t, "alice", false)
	cases := []string{"ab", "with space", "with.dot", "$bad", "way-too-long-name-exceeding-the-32-character-limit-for-sure"}
	for _, name := range cases {
		w := tr.requestSource(t, tok, name, "")
		if w.Code != http.StatusBadRequest {
			t.Errorf("name %q got %d, want 400", name, w.Code)
		}
	}
}

func TestHandleRequestSource_RejoinFromLeftReusesAndMints(t *testing.T) {
	tr := newTestRouter(t)
	tok, uid := tr.loginAs(t, "alice", false)

	mustOK(t, tr.requestSource(t, tok, "alice-q3", "old"))
	must(t, tr.store.ApproveSource(context.Background(), "alice-q3"))
	must(t, tr.store.LeaveSource(context.Background(), "alice-q3"))

	mintsBefore := tr.userProv.mintCalls
	w := tr.requestSource(t, tok, "alice-q3", "rejoining")
	if w.Code != http.StatusCreated {
		t.Fatalf("rejoin: %d %s", w.Code, w.Body)
	}
	rows, _ := tr.store.ListSourcesByOwner(context.Background(), uid)
	if len(rows) != 1 || rows[0].Status != "active" || rows[0].RequestedPurpose != "rejoining" {
		t.Errorf("after rejoin: %+v", rows)
	}
	if tr.userProv.mintCalls <= mintsBefore {
		t.Error("rejoin did not call MintUserCreds")
	}
}

func TestHandleDownloadMyCreds_OnlyOwnerActive(t *testing.T) {
	tr := newTestRouter(t)
	tok, _ := tr.loginAs(t, "alice", false)

	mustOK(t, tr.requestSource(t, tok, "alice-q3", ""))
	// Pending → 403 (no longer the simple "no source" check, since we
	// might own multiple — but the named one isn't active).
	if w := tr.do("GET", "/api/sources/mine/alice-q3/creds", "", tok); w.Code != http.StatusForbidden {
		t.Errorf("download while pending = %d, want 403", w.Code)
	}

	// Approve + mint, now download succeeds.
	must(t, tr.store.ApproveSource(context.Background(), "alice-q3"))
	if _, err := tr.userProv.MintUserCreds("alice-q3"); err != nil {
		t.Fatal(err)
	}
	w := tr.do("GET", "/api/sources/mine/alice-q3/creds", "", tok)
	if w.Code != http.StatusOK {
		t.Fatalf("download = %d, body = %s", w.Code, w.Body)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("MINT:alice-q3")) {
		t.Errorf("body did not contain expected creds blob: %q", w.Body)
	}
}

func TestHandleDownloadMyCreds_NotOwner(t *testing.T) {
	tr := newTestRouter(t)
	tokA, _ := tr.loginAs(t, "alice", false)
	tokB, _ := tr.loginAs(t, "bob", false)
	mustOK(t, tr.requestSource(t, tokA, "alice-q3", ""))
	must(t, tr.store.ApproveSource(context.Background(), "alice-q3"))

	// Bob asking for alice-q3 should get 404 (don't leak existence).
	w := tr.do("GET", "/api/sources/mine/alice-q3/creds", "", tokB)
	if w.Code != http.StatusNotFound {
		t.Errorf("bob downloading alice's creds = %d, want 404", w.Code)
	}
}

func TestHandleRotateMyCreds_RateLimited(t *testing.T) {
	tr := newTestRouter(t)
	tok, _ := tr.loginAs(t, "alice", false)

	mustOK(t, tr.requestSource(t, tok, "alice-q3", ""))
	must(t, tr.store.ApproveSource(context.Background(), "alice-q3"))

	for i := 1; i <= 5; i++ {
		w := tr.do("POST", "/api/sources/mine/alice-q3/rotate-creds", "", tok)
		if w.Code != http.StatusOK {
			t.Errorf("rotate %d = %d, want 200", i, w.Code)
		}
	}
	w := tr.do("POST", "/api/sources/mine/alice-q3/rotate-creds", "", tok)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("6th rotate = %d, want 429", w.Code)
	}
}

func TestHandleLeaveMySource_OnlyAffectsTargeted(t *testing.T) {
	// Alice owns two sources; leaving one should not touch the other.
	tr := newTestRouter(t)
	tok, uid := tr.loginAs(t, "alice", false)

	mustOK(t, tr.requestSource(t, tok, "host-a", ""))
	must(t, tr.store.ApproveSource(context.Background(), "host-a"))
	mustOK(t, tr.requestSource(t, tok, "host-b", ""))
	must(t, tr.store.ApproveSource(context.Background(), "host-b"))

	w := tr.do("POST", "/api/sources/mine/host-a/leave", "", tok)
	if w.Code != http.StatusNoContent {
		t.Fatalf("leave host-a = %d, body = %s", w.Code, w.Body)
	}
	rows, _ := tr.store.ListSourcesByOwner(context.Background(), uid)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %+v", rows)
	}
	for _, r := range rows {
		switch r.Source {
		case "host-a":
			if r.Status != "left" {
				t.Errorf("host-a status = %q, want left", r.Status)
			}
		case "host-b":
			if r.Status != "active" {
				t.Errorf("host-b status = %q, want active", r.Status)
			}
		}
	}
}

func TestHandleLeaveMySource_NotOwner(t *testing.T) {
	tr := newTestRouter(t)
	tokA, _ := tr.loginAs(t, "alice", false)
	tokB, _ := tr.loginAs(t, "bob", false)
	mustOK(t, tr.requestSource(t, tokA, "alice-q3", ""))
	must(t, tr.store.ApproveSource(context.Background(), "alice-q3"))
	w := tr.do("POST", "/api/sources/mine/alice-q3/leave", "", tokB)
	if w.Code != http.StatusNotFound {
		t.Errorf("bob leaving alice's source = %d, want 404", w.Code)
	}
}

// Test helpers shared with admin_sources_test.go.
func mustOK(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	if w.Code/100 != 2 {
		t.Fatalf("non-2xx: %d body=%s", w.Code, w.Body)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
