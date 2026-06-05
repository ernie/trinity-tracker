package storage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func newPATTestStore(t *testing.T) (*Store, int64) {
	t.Helper()
	store, err := New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	if err := store.CreateUser(ctx, "pat-user", "x", true, nil); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	user, err := store.GetUserByUsername(ctx, "pat-user")
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}
	return store, user.ID
}

func TestPATLifecycle(t *testing.T) {
	store, userID := newPATTestStore(t)
	ctx := context.Background()

	if err := store.CreatePAT(ctx, userID, "cli:test@box", "hash-1"); err != nil {
		t.Fatalf("CreatePAT: %v", err)
	}

	id, err := store.LookupPATByHash(ctx, "hash-1")
	if err != nil {
		t.Fatalf("LookupPATByHash: %v", err)
	}
	if id.UserID != userID || id.Username != "pat-user" || !id.IsAdmin {
		t.Errorf("identity mismatch: %+v", id)
	}

	// Unknown hash is not found.
	if _, err := store.LookupPATByHash(ctx, "no-such"); !errors.Is(err, ErrPATNotFound) {
		t.Errorf("unknown hash: want ErrPATNotFound, got %v", err)
	}

	// Revoke kills the lookup; second revoke reports not-found.
	if err := store.RevokePATByHash(ctx, "hash-1"); err != nil {
		t.Fatalf("RevokePATByHash: %v", err)
	}
	if _, err := store.LookupPATByHash(ctx, "hash-1"); !errors.Is(err, ErrPATNotFound) {
		t.Errorf("revoked lookup: want ErrPATNotFound, got %v", err)
	}
	if err := store.RevokePATByHash(ctx, "hash-1"); !errors.Is(err, ErrPATNotFound) {
		t.Errorf("double revoke: want ErrPATNotFound, got %v", err)
	}
}

func TestPATLookupReflectsLiveUserRow(t *testing.T) {
	store, userID := newPATTestStore(t)
	ctx := context.Background()

	if err := store.CreatePAT(ctx, userID, "", "hash-2"); err != nil {
		t.Fatalf("CreatePAT: %v", err)
	}
	if err := store.UpdateUserAdmin(ctx, userID, false); err != nil {
		t.Fatalf("UpdateUserAdmin: %v", err)
	}
	id, err := store.LookupPATByHash(ctx, "hash-2")
	if err != nil {
		t.Fatalf("LookupPATByHash: %v", err)
	}
	if id.IsAdmin {
		t.Error("lookup returned stale is_admin after demotion")
	}

	// Deleting the user takes the token with it (FK cascade + join).
	if err := store.DeleteUser(ctx, "pat-user"); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if _, err := store.LookupPATByHash(ctx, "hash-2"); !errors.Is(err, ErrPATNotFound) {
		t.Errorf("deleted user: want ErrPATNotFound, got %v", err)
	}
}

func TestPATTouchLastUsed(t *testing.T) {
	store, userID := newPATTestStore(t)
	ctx := context.Background()

	if err := store.CreatePAT(ctx, userID, "", "hash-3"); err != nil {
		t.Fatalf("CreatePAT: %v", err)
	}
	id, err := store.LookupPATByHash(ctx, "hash-3")
	if err != nil {
		t.Fatalf("LookupPATByHash: %v", err)
	}
	if err := store.TouchPATLastUsed(ctx, id.PATID); err != nil {
		t.Fatalf("TouchPATLastUsed: %v", err)
	}
	var lastUsed any
	if err := store.db.QueryRowContext(ctx,
		"SELECT last_used_at FROM pat WHERE id = ?", id.PATID).Scan(&lastUsed); err != nil {
		t.Fatalf("query last_used_at: %v", err)
	}
	if lastUsed == nil {
		t.Error("last_used_at still NULL after touch")
	}
}
