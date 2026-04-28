package storage

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

// mustCreateUser inserts a users row and returns its id. Tests use
// this so CreateSourceRequest's owner FK lookup has a target.
func mustCreateUser(t *testing.T, s *Store, username string) int64 {
	t.Helper()
	res, err := s.db.ExecContext(context.Background(), `
		INSERT INTO users (username, password_hash) VALUES (?, ?)
	`, username, "x")
	if err != nil {
		t.Fatalf("insert user %q: %v", username, err)
	}
	id, _ := res.LastInsertId()
	return id
}

func mustReq(t *testing.T, s *Store, ctx context.Context, args CreateSourceRequestArgs) {
	t.Helper()
	if err := s.CreateSourceRequest(ctx, args); err != nil {
		t.Fatalf("CreateSourceRequest(%+v): %v", args, err)
	}
}

func TestCreateSourceRequest_CreatesPending(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	uid := mustCreateUser(t, s, "alice")

	mustReq(t, s, ctx, CreateSourceRequestArgs{
		Source:           "alice-q3",
		OwnerUserID:      uid,
		RequestedPurpose: "for fun",
	})

	rows, err := s.ListSourcesByOwner(ctx, uid)
	if err != nil {
		t.Fatalf("ListSourcesByOwner: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	got := rows[0]
	if got.Status != "pending" {
		t.Errorf("status = %q, want pending", got.Status)
	}
	if got.Active {
		t.Error("pending row should have active=0")
	}
	if got.RequestedPurpose != "for fun" {
		t.Errorf("purpose = %q", got.RequestedPurpose)
	}
}

func TestCreateSourceRequest_DuplicatePendingBlocked(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	uid := mustCreateUser(t, s, "alice")

	mustReq(t, s, ctx, CreateSourceRequestArgs{
		Source: "first", OwnerUserID: uid,
	})
	err := s.CreateSourceRequest(ctx, CreateSourceRequestArgs{
		Source: "second", OwnerUserID: uid,
	})
	if !errors.Is(err, ErrPendingRequestExists) {
		t.Fatalf("got %v, want ErrPendingRequestExists", err)
	}
}

func TestCreateSourceRequest_AfterApproveAllowsAnother(t *testing.T) {
	// Multi-source-per-owner is supported: once the first pending is
	// approved (or rejected), the user can submit a second request.
	s := newTestStore(t)
	ctx := context.Background()
	uid := mustCreateUser(t, s, "alice")

	mustReq(t, s, ctx, CreateSourceRequestArgs{Source: "host-a", OwnerUserID: uid})
	must(t, s.ApproveSource(ctx, "host-a"))

	// User now has one active. A second request should succeed.
	mustReq(t, s, ctx, CreateSourceRequestArgs{Source: "host-b", OwnerUserID: uid})

	rows, err := s.ListSourcesByOwner(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2: %+v", len(rows), rows)
	}
	// ListSourcesByOwner orders active before pending.
	if rows[0].Source != "host-a" || rows[0].Status != "active" {
		t.Errorf("row 0 = %+v", rows[0])
	}
	if rows[1].Source != "host-b" || rows[1].Status != "pending" {
		t.Errorf("row 1 = %+v", rows[1])
	}
}

func TestCreateSourceRequest_PendingBlocksEvenWithActive(t *testing.T) {
	// Once a pending request is in flight, a third can't be added —
	// this is the "no flooding the queue" guard.
	s := newTestStore(t)
	ctx := context.Background()
	uid := mustCreateUser(t, s, "alice")

	mustReq(t, s, ctx, CreateSourceRequestArgs{Source: "host-a", OwnerUserID: uid})
	must(t, s.ApproveSource(ctx, "host-a"))
	mustReq(t, s, ctx, CreateSourceRequestArgs{Source: "host-b", OwnerUserID: uid})
	// host-b is now pending.

	err := s.CreateSourceRequest(ctx, CreateSourceRequestArgs{
		Source: "host-c", OwnerUserID: uid,
	})
	if !errors.Is(err, ErrPendingRequestExists) {
		t.Fatalf("got %v, want ErrPendingRequestExists", err)
	}
}

func TestCreateSourceRequest_NameTakenByOther(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	a := mustCreateUser(t, s, "alice")
	b := mustCreateUser(t, s, "bob")

	mustReq(t, s, ctx, CreateSourceRequestArgs{Source: "shared", OwnerUserID: a})
	err := s.CreateSourceRequest(ctx, CreateSourceRequestArgs{
		Source: "shared", OwnerUserID: b,
	})
	if !errors.Is(err, ErrSourceNameTaken) {
		t.Fatalf("got %v, want ErrSourceNameTaken", err)
	}
}

func TestCreateSourceRequest_NameTakenByLeftOther(t *testing.T) {
	// A 'left' row owned by another user still claims the name —
	// they can come back later. New requesters get ErrSourceNameTaken.
	s := newTestStore(t)
	ctx := context.Background()
	a := mustCreateUser(t, s, "alice")
	b := mustCreateUser(t, s, "bob")

	mustReq(t, s, ctx, CreateSourceRequestArgs{Source: "shared", OwnerUserID: a})
	must(t, s.ApproveSource(ctx, "shared"))
	must(t, s.LeaveSource(ctx, "shared"))

	err := s.CreateSourceRequest(ctx, CreateSourceRequestArgs{
		Source: "shared", OwnerUserID: b,
	})
	if !errors.Is(err, ErrSourceNameTaken) {
		t.Fatalf("got %v, want ErrSourceNameTaken", err)
	}
}

func TestApproveSource_FlipsToActive(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	uid := mustCreateUser(t, s, "alice")
	mustReq(t, s, ctx, CreateSourceRequestArgs{Source: "alice-q3", OwnerUserID: uid})

	must(t, s.ApproveSource(ctx, "alice-q3"))
	got, _ := s.GetSourceByName(ctx, "alice-q3")
	if got.Status != "active" {
		t.Errorf("status = %q, want active", got.Status)
	}
	if !got.Active {
		t.Error("active should be true")
	}

	// Idempotency: approving again is a no-op (status != pending).
	if err := s.ApproveSource(ctx, "alice-q3"); !errors.Is(err, ErrSourceNotPending) {
		t.Errorf("re-approve = %v, want ErrSourceNotPending", err)
	}
}

func TestRejectSource_StoresReason(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	uid := mustCreateUser(t, s, "alice")
	mustReq(t, s, ctx, CreateSourceRequestArgs{Source: "alice-q3", OwnerUserID: uid})

	must(t, s.RejectSource(ctx, "alice-q3", "name conflict on the network"))
	got, _ := s.GetSourceByName(ctx, "alice-q3")
	if got.Status != "rejected" {
		t.Errorf("status = %q, want rejected", got.Status)
	}
	if got.RejectionReason != "name conflict on the network" {
		t.Errorf("reason = %q", got.RejectionReason)
	}
	if got.Active {
		t.Error("rejected row should have active=0")
	}
}

func TestRejectSource_RequiresReason(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	uid := mustCreateUser(t, s, "alice")
	mustReq(t, s, ctx, CreateSourceRequestArgs{Source: "alice-q3", OwnerUserID: uid})
	if err := s.RejectSource(ctx, "alice-q3", "   "); err == nil {
		t.Fatal("blank reason should error")
	}
}

func TestLeaveSource_FromActive(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	uid := mustCreateUser(t, s, "alice")
	mustReq(t, s, ctx, CreateSourceRequestArgs{Source: "alice-q3", OwnerUserID: uid})
	must(t, s.ApproveSource(ctx, "alice-q3"))

	must(t, s.LeaveSource(ctx, "alice-q3"))
	got, _ := s.GetSourceByName(ctx, "alice-q3")
	if got.Status != "left" || got.Active {
		t.Errorf("after leave got %+v", got)
	}
}

func TestLeaveSource_OneSourceDoesNotAffectOthers(t *testing.T) {
	// Owner has two actives; leaving one keeps the other intact.
	s := newTestStore(t)
	ctx := context.Background()
	uid := mustCreateUser(t, s, "alice")

	mustReq(t, s, ctx, CreateSourceRequestArgs{Source: "host-a", OwnerUserID: uid})
	must(t, s.ApproveSource(ctx, "host-a"))
	mustReq(t, s, ctx, CreateSourceRequestArgs{Source: "host-b", OwnerUserID: uid})
	must(t, s.ApproveSource(ctx, "host-b"))

	must(t, s.LeaveSource(ctx, "host-a"))

	a, _ := s.GetSourceByName(ctx, "host-a")
	b, _ := s.GetSourceByName(ctx, "host-b")
	if a.Status != "left" {
		t.Errorf("host-a status = %q, want left", a.Status)
	}
	if b.Status != "active" {
		t.Errorf("host-b status = %q, want active (untouched)", b.Status)
	}
}

func TestLeaveSource_RejectedDoesNotApply(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	uid := mustCreateUser(t, s, "alice")
	mustReq(t, s, ctx, CreateSourceRequestArgs{Source: "alice-q3", OwnerUserID: uid})
	must(t, s.RejectSource(ctx, "alice-q3", "no"))
	if err := s.LeaveSource(ctx, "alice-q3"); !errors.Is(err, ErrSourceNotActive) {
		t.Errorf("leave from rejected = %v, want ErrSourceNotActive", err)
	}
}

func TestRejoin_FromLeft_ReusesRowAndAutoApproves(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	uid := mustCreateUser(t, s, "alice")
	mustReq(t, s, ctx, CreateSourceRequestArgs{Source: "alice-q3", OwnerUserID: uid, RequestedPurpose: "old"})
	must(t, s.ApproveSource(ctx, "alice-q3"))
	must(t, s.LeaveSource(ctx, "alice-q3"))

	// Same owner re-submits with the same name → auto-approved.
	mustReq(t, s, ctx, CreateSourceRequestArgs{
		Source: "alice-q3", OwnerUserID: uid, RequestedPurpose: "rejoining",
	})
	rows, _ := s.ListSourcesByOwner(ctx, uid)
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1 (rejoin reuses)", len(rows))
	}
	if rows[0].Status != "active" {
		t.Errorf("rejoin should be active, got %q", rows[0].Status)
	}
	if rows[0].RequestedPurpose != "rejoining" {
		t.Errorf("rejoin should refresh purpose, got %q", rows[0].RequestedPurpose)
	}
}

func TestListPendingRequests_OnlyPendingWithUsername(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	a := mustCreateUser(t, s, "alice")
	b := mustCreateUser(t, s, "bob")

	mustReq(t, s, ctx, CreateSourceRequestArgs{
		Source: "alice-q3", OwnerUserID: a, RequestedPurpose: "fun",
	})
	mustReq(t, s, ctx, CreateSourceRequestArgs{
		Source: "bob-q3", OwnerUserID: b,
	})
	must(t, s.ApproveSource(ctx, "alice-q3"))

	rows, err := s.ListPendingRequests(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0].Source != "bob-q3" {
		t.Errorf("source = %q, want bob-q3", rows[0].Source)
	}
	if rows[0].OwnerUsername != "bob" {
		t.Errorf("owner_username = %q, want bob", rows[0].OwnerUsername)
	}
}

func TestListSourcesByOwner_EmptyForUserWithNone(t *testing.T) {
	s := newTestStore(t)
	uid := mustCreateUser(t, s, "alice")
	rows, err := s.ListSourcesByOwner(context.Background(), uid)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("got %d rows, want 0", len(rows))
	}
}

func TestListSourcesByOwner_OrdersActiveFirstPendingSecond(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	uid := mustCreateUser(t, s, "alice")

	// Reject one (drops to back), approve another (front), pending in middle.
	mustReq(t, s, ctx, CreateSourceRequestArgs{Source: "rej", OwnerUserID: uid})
	must(t, s.RejectSource(ctx, "rej", "no"))
	mustReq(t, s, ctx, CreateSourceRequestArgs{Source: "act", OwnerUserID: uid})
	must(t, s.ApproveSource(ctx, "act"))
	mustReq(t, s, ctx, CreateSourceRequestArgs{Source: "pen", OwnerUserID: uid})

	rows, err := s.ListSourcesByOwner(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
	want := []string{"act", "pen", "rej"}
	for i, w := range want {
		if rows[i].Source != w {
			t.Errorf("row %d = %q, want %q", i, rows[i].Source, w)
		}
	}
}

func TestRenamePendingSource_RewritesAllReferences(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	uid := mustCreateUser(t, s, "alice")

	mustReq(t, s, ctx, CreateSourceRequestArgs{Source: "old-name", OwnerUserID: uid, RequestedPurpose: "fix me"})
	must(t, s.WriteSourceAudit(ctx, "old-name", &uid, "requested", "fix me"))

	must(t, s.RenamePendingSource(ctx, "old-name", "new-name"))

	got, err := s.GetSourceByName(ctx, "new-name")
	if err != nil {
		t.Fatalf("get after rename: %v", err)
	}
	if got.Status != "pending" || got.RequestedPurpose != "fix me" {
		t.Errorf("after rename: %+v", got)
	}
	// Old name is gone.
	if _, err := s.GetSourceByName(ctx, "old-name"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("old-name still exists: %v", err)
	}
	// Audit rows carry across.
	rows, _ := s.ListSourceAudit(ctx, "new-name", 10)
	if len(rows) != 1 || rows[0].Action != "requested" {
		t.Errorf("audit lost: %+v", rows)
	}
}

func TestRenamePendingSource_NotPending(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	uid := mustCreateUser(t, s, "alice")
	mustReq(t, s, ctx, CreateSourceRequestArgs{Source: "host", OwnerUserID: uid})
	must(t, s.ApproveSource(ctx, "host"))

	if err := s.RenamePendingSource(ctx, "host", "renamed"); !errors.Is(err, ErrSourceNotPending) {
		t.Errorf("rename of active = %v, want ErrSourceNotPending", err)
	}
}

func TestRenamePendingSource_NameTaken(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	a := mustCreateUser(t, s, "alice")
	b := mustCreateUser(t, s, "bob")
	mustReq(t, s, ctx, CreateSourceRequestArgs{Source: "alice-q3", OwnerUserID: a})
	must(t, s.ApproveSource(ctx, "alice-q3"))
	mustReq(t, s, ctx, CreateSourceRequestArgs{Source: "bob-q3", OwnerUserID: b})

	// Trying to rename bob's pending request to "alice-q3" (taken).
	if err := s.RenamePendingSource(ctx, "bob-q3", "alice-q3"); !errors.Is(err, ErrSourceNameTaken) {
		t.Errorf("rename to taken = %v, want ErrSourceNameTaken", err)
	}
}

func TestRenamePendingSource_InvalidName(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	uid := mustCreateUser(t, s, "alice")
	mustReq(t, s, ctx, CreateSourceRequestArgs{Source: "old", OwnerUserID: uid})
	if err := s.RenamePendingSource(ctx, "old", "ab"); err == nil {
		t.Error("rename to too-short name should error")
	}
	if err := s.RenamePendingSource(ctx, "old", "with space"); err == nil {
		t.Error("rename to invalid format should error")
	}
}

func TestRevokeSourceStatus_Idempotent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	uid := mustCreateUser(t, s, "alice")
	mustReq(t, s, ctx, CreateSourceRequestArgs{Source: "alice-q3", OwnerUserID: uid})
	must(t, s.ApproveSource(ctx, "alice-q3"))

	must(t, s.RevokeSourceStatus(ctx, "alice-q3"))
	got, _ := s.GetSourceByName(ctx, "alice-q3")
	if got.Status != "revoked" {
		t.Errorf("status = %q", got.Status)
	}
	must(t, s.RevokeSourceStatus(ctx, "alice-q3"))
}

func TestListServersForSource(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	uid := mustCreateUser(t, s, "alice")
	mustReq(t, s, ctx, CreateSourceRequestArgs{Source: "alice-q3", OwnerUserID: uid})
	must(t, s.ApproveSource(ctx, "alice-q3"))

	rows, err := s.ListServersForSource(ctx, "alice-q3")
	if err != nil {
		t.Fatalf("list servers: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("just-approved source should have 0 servers, got %d", len(rows))
	}
}

func TestGetSourceByName_NoRowReturnsErrNoRows(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetSourceByName(context.Background(), "missing")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("got %v, want sql.ErrNoRows", err)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
