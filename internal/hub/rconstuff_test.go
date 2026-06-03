package hub

import (
	"context"
	"testing"
	"time"

	"github.com/ernie/trinity-tracker/internal/crypto"
	"github.com/ernie/trinity-tracker/internal/domain"
	"github.com/ernie/trinity-tracker/internal/storage"
)

type rconFixture struct {
	w       *Writer
	store   *storage.Store
	ctx     context.Context
	ownerID int64
	adminID int64
	otherID int64
	server  *domain.Server // remote source server (alice-q3 / remote)
	local   *domain.Server // local source server (hub-q3 / main)
}

// seedRconFixture builds a database with two sources:
//   - "alice-q3": remote, owned by ownerID; servers: alice-server (no
//     delegation) and alice-deleg (delegation enabled)
//   - "hub-q3": local source (no owner row); server: hub-server
//
// Plus three users: owner (non-admin), admin (is_admin, not owner), other
// (non-admin, not owner). Each user has a unique GameToken seeded so
// authProof helpers can hash it.
func seedRconFixture(t *testing.T) *rconFixture {
	t.Helper()
	ctx := context.Background()
	w, store := newTestWriter(t)
	w.localSource = "hub-q3"

	// Players + users
	for _, name := range []string{"owner", "admin", "other"} {
		_, err := store.UpsertPlayerGUID(ctx, "guid-"+name, name, name, time.Now().UTC(), false)
		if err != nil {
			t.Fatalf("UpsertPlayerGUID %s: %v", name, err)
		}
	}

	// Find each player_id
	findPlayer := func(guid string) int64 {
		pg, err := store.GetPlayerGUIDByGUID(ctx, guid)
		if err != nil || pg == nil {
			t.Fatalf("lookup player_guid %s: %v", guid, err)
		}
		return pg.PlayerID
	}
	ownerPlayer := findPlayer("guid-owner")
	adminPlayer := findPlayer("guid-admin")
	otherPlayer := findPlayer("guid-other")

	mustUser := func(username string, isAdmin bool, playerID int64) int64 {
		t.Helper()
		if err := store.CreateUser(ctx, username, "hash", isAdmin, &playerID); err != nil {
			t.Fatalf("CreateUser %s: %v", username, err)
		}
		u, err := store.GetUserByUsername(ctx, username)
		if err != nil {
			t.Fatalf("GetUserByUsername %s: %v", username, err)
		}
		if _, err := store.RotateGameToken(ctx, u.ID); err != nil {
			t.Fatalf("RotateGameToken %s: %v", username, err)
		}
		return u.ID
	}
	ownerID := mustUser("owner", false, ownerPlayer)
	adminID := mustUser("admin", true, adminPlayer)
	otherID := mustUser("other", false, otherPlayer)

	// Sources
	if err := store.UpsertLocalSource(ctx, "hub-q3"); err != nil {
		t.Fatalf("UpsertLocalSource: %v", err)
	}
	if err := store.CreateSource(ctx, "alice-q3", true, &ownerID); err != nil {
		t.Fatalf("CreateSource alice-q3: %v", err)
	}

	// Servers — remote with delegation off
	srv := &domain.Server{Key: "remote", Address: "10.0.0.1:27960"}
	if err := store.UpsertServer(ctx, "alice-q3", srv); err != nil {
		t.Fatalf("UpsertServer remote: %v", err)
	}
	// Local hub server
	local := &domain.Server{Key: "main", Address: "127.0.0.1:27960"}
	if err := store.UpsertServer(ctx, "hub-q3", local); err != nil {
		t.Fatalf("UpsertServer local: %v", err)
	}

	server, err := store.GetServerByID(ctx, srv.ID)
	if err != nil {
		t.Fatalf("GetServerByID remote: %v", err)
	}
	localServer, err := store.GetServerByID(ctx, local.ID)
	if err != nil {
		t.Fatalf("GetServerByID local: %v", err)
	}

	return &rconFixture{
		w: w, store: store, ctx: ctx,
		ownerID: ownerID, adminID: adminID, otherID: otherID,
		server: server, local: localServer,
	}
}

// reloadUser refetches the user with its current GameToken populated.
func (f *rconFixture) reloadUser(t *testing.T, id int64) *storage.User {
	t.Helper()
	u, err := f.store.GetUserByID(f.ctx, id)
	if err != nil {
		t.Fatalf("GetUserByID %d: %v", id, err)
	}
	return u
}

func TestEvaluateRconStuff_OwnerOnTheirOwnRemoteSource(t *testing.T) {
	f := seedRconFixture(t)
	u := f.reloadUser(t, f.ownerID)

	got := f.w.evaluateRconStuff(f.ctx, u, f.server.ID)
	if got == nil {
		t.Fatal("expected RconStuff for owner; got nil")
	}
	if got.Role != string(RconRoleOwner) {
		t.Errorf("role = %q, want %q", got.Role, RconRoleOwner)
	}
	if got.EpochNonce == ([crypto.RconsetNonceLen]byte{}) {
		t.Error("EpochNonce is zero — RNG not wired up")
	}
	if got.Key == ([16]byte{}) {
		t.Error("Key is zero — DeriveRconsetKey not invoked")
	}

	expectedKey := crypto.DeriveRconsetKey(u.GameToken, got.EpochNonce)
	if got.Key != expectedKey {
		t.Errorf("Key mismatch: got %x, want %x", got.Key, expectedKey)
	}
}

func TestEvaluateRconStuff_AdminLocalShortcut(t *testing.T) {
	f := seedRconFixture(t)
	u := f.reloadUser(t, f.adminID)

	got := f.w.evaluateRconStuff(f.ctx, u, f.local.ID)
	if got == nil {
		t.Fatal("admin on local source should get RconStuff")
	}
	if got.Role != string(RconRoleOwner) {
		t.Errorf("local-admin role = %q, want %q", got.Role, RconRoleOwner)
	}
}

func TestEvaluateRconStuff_AdminRemoteNoDelegationDenied(t *testing.T) {
	f := seedRconFixture(t)
	u := f.reloadUser(t, f.adminID)

	// f.server has AdminDelegationEnabled = false by default
	got := f.w.evaluateRconStuff(f.ctx, u, f.server.ID)
	if got != nil {
		t.Errorf("admin on non-delegated remote source should get nil; got %+v", got)
	}
}

func TestEvaluateRconStuff_AdminRemoteWithDelegationGetsHubAdmin(t *testing.T) {
	f := seedRconFixture(t)
	if err := f.store.SetServerAdminDelegation(f.ctx, f.server.ID, true); err != nil {
		t.Fatalf("SetServerAdminDelegation: %v", err)
	}

	u := f.reloadUser(t, f.adminID)
	got := f.w.evaluateRconStuff(f.ctx, u, f.server.ID)
	if got == nil {
		t.Fatal("admin on delegated remote source should get RconStuff")
	}
	if got.Role != string(RconRoleHubAdmin) {
		t.Errorf("role = %q, want %q", got.Role, RconRoleHubAdmin)
	}
}

func TestEvaluateRconStuff_RandomUserDenied(t *testing.T) {
	f := seedRconFixture(t)
	u := f.reloadUser(t, f.otherID)

	got := f.w.evaluateRconStuff(f.ctx, u, f.server.ID)
	if got != nil {
		t.Errorf("non-owner non-admin should get nil; got %+v", got)
	}
}

func TestEvaluateRconStuff_NonAdminOwnerStillAuthorized(t *testing.T) {
	// User-added collector case: not is_admin, but owns the source.
	f := seedRconFixture(t)
	u := f.reloadUser(t, f.ownerID)
	if u.IsAdmin {
		t.Fatal("test setup error: owner should not be admin")
	}

	got := f.w.evaluateRconStuff(f.ctx, u, f.server.ID)
	if got == nil || got.Role != string(RconRoleOwner) {
		t.Fatalf("non-admin owner should still get owner role; got %+v", got)
	}
}

func TestEvaluateRconStuff_FreshNoncePerCall(t *testing.T) {
	f := seedRconFixture(t)
	u := f.reloadUser(t, f.ownerID)

	a := f.w.evaluateRconStuff(f.ctx, u, f.server.ID)
	b := f.w.evaluateRconStuff(f.ctx, u, f.server.ID)
	if a == nil || b == nil {
		t.Fatal("both calls should succeed")
	}
	if a.EpochNonce == b.EpochNonce {
		t.Error("expected distinct nonces per handshake")
	}
	if a.Key == b.Key {
		t.Error("expected distinct keys per handshake")
	}
}

func TestEvaluateRconStuff_AuditRowWritten(t *testing.T) {
	f := seedRconFixture(t)
	u := f.reloadUser(t, f.ownerID)
	_ = f.w.evaluateRconStuff(f.ctx, u, f.server.ID)

	rows, err := f.store.ListAllAudit(f.ctx, storage.AuditFilters{
		Source: f.server.Source,
		Action: "rcon.autoset",
	})
	if err != nil {
		t.Fatalf("ListAllAudit: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 audit row; got %d", len(rows))
	}
	if !rows[0].ActorUserID.Valid || rows[0].ActorUserID.Int64 != f.ownerID {
		t.Errorf("actor = %v, want %d", rows[0].ActorUserID, f.ownerID)
	}
}
