package storage

import (
	"context"
	"testing"
)

func TestWriteSourceAudit_OwnerActor(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	uid := mustCreateUser(t, s, "alice")
	mustReq(t, s, ctx, CreateSourceRequestArgs{
		Source: "alice-q3", OwnerUserID: uid, RequestedPurpose: "",
	})

	if err := s.WriteSourceAudit(ctx, "alice-q3", &uid, "requested", "from header"); err != nil {
		t.Fatalf("write audit: %v", err)
	}

	rows, err := s.ListSourceAudit(ctx, "alice-q3", 10)
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0].Action != "requested" || rows[0].Detail != "from header" {
		t.Errorf("got %+v", rows[0])
	}
	if !rows[0].ActorUserID.Valid || rows[0].ActorUserID.Int64 != uid {
		t.Errorf("actor_user_id = %v, want %d", rows[0].ActorUserID, uid)
	}
}

func TestWriteSourceAudit_SystemActor(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	uid := mustCreateUser(t, s, "alice")
	mustReq(t, s, ctx, CreateSourceRequestArgs{
		Source: "alice-q3", OwnerUserID: uid, RequestedPurpose: "",
	})

	if err := s.WriteSourceAudit(ctx, "alice-q3", nil, "system", "boot revoke"); err != nil {
		t.Fatalf("write audit: %v", err)
	}
	rows, _ := s.ListSourceAudit(ctx, "alice-q3", 10)
	if len(rows) != 1 || rows[0].ActorUserID.Valid {
		t.Errorf("system actor should be NULL, got %+v", rows[0])
	}
}

func TestListSourceAudit_NewestFirst(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	uid := mustCreateUser(t, s, "alice")
	mustReq(t, s, ctx, CreateSourceRequestArgs{
		Source: "alice-q3", OwnerUserID: uid, RequestedPurpose: "",
	})
	must(t, s.WriteSourceAudit(ctx, "alice-q3", &uid, "requested", ""))
	must(t, s.WriteSourceAudit(ctx, "alice-q3", &uid, "approved", ""))
	must(t, s.WriteSourceAudit(ctx, "alice-q3", &uid, "downloaded", ""))

	rows, err := s.ListSourceAudit(ctx, "alice-q3", 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
	// Newest first: downloaded, approved, requested.
	if rows[0].Action != "downloaded" || rows[1].Action != "approved" || rows[2].Action != "requested" {
		t.Errorf("order wrong: %v %v %v", rows[0].Action, rows[1].Action, rows[2].Action)
	}
}
