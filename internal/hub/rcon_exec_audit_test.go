package hub

import (
	"testing"

	"github.com/ernie/trinity-tracker/internal/domain"
	"github.com/ernie/trinity-tracker/internal/storage"
)

func TestHandleRconExec_KnownGuidAttributedToUser(t *testing.T) {
	f := seedRconFixture(t)
	// owner's player has a GUID seeded as "guid-owner" by the fixture
	f.w.handleRconExec(f.ctx, f.server.ID, domain.RconExecData{
		ClientAddr: "1.2.3.4:27960",
		GUID:       "guid-owner",
		Command:    "status",
	})

	rows, err := f.store.ListAllAudit(f.ctx, storage.AuditFilters{
		Source: f.server.Source,
		Action: "rcon.exec",
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
	if rows[0].Detail != "ingame: status" {
		t.Errorf("detail = %q, want %q", rows[0].Detail, "ingame: status")
	}
}

func TestHandleRconExec_UnknownGuidIsSkipped(t *testing.T) {
	// Unlinked GUID (engine matched a connected client but that GUID
	// isn't tied to a user). Skip — same rule as "no GUID at all".
	f := seedRconFixture(t)
	f.w.handleRconExec(f.ctx, f.server.ID, domain.RconExecData{
		ClientAddr: "10.0.0.5:54321",
		GUID:       "guid-not-in-db",
		Command:    "kick badname",
	})

	rows, err := f.store.ListAllAudit(f.ctx, storage.AuditFilters{
		Source: f.server.Source,
		Action: "rcon.exec",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Errorf("expected 0 rows (unknown GUID = no attribution = no row); got %d", len(rows))
	}
}

func TestHandleRconExec_NoGuidIsSkipped(t *testing.T) {
	// Empty GUID — collector hairpin or external operator. Skip.
	// Successful rcon already has its own attributed audit row written
	// by internal/api/rcon.go for the web-UI path.
	f := seedRconFixture(t)
	f.w.handleRconExec(f.ctx, f.server.ID, domain.RconExecData{
		ClientAddr: "10.0.0.5:54321",
		GUID:       "",
		Command:    "map q3dm17",
	})

	rows, err := f.store.ListAllAudit(f.ctx, storage.AuditFilters{
		Source: f.server.Source,
		Action: "rcon.exec",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Errorf("expected 0 rows (no GUID = no row); got %d", len(rows))
	}
}

func TestHandleRconDenied_WritesNullActorRow(t *testing.T) {
	f := seedRconFixture(t)
	f.w.handleRconDenied(f.ctx, f.server.ID, domain.RconDeniedData{
		ClientAddr: "203.0.113.99:51234",
	})

	rows, err := f.store.ListAllAudit(f.ctx, storage.AuditFilters{
		Source: f.server.Source,
		Action: "rcon.denied",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row; got %d", len(rows))
	}
	if rows[0].ActorUserID.Valid {
		t.Errorf("denied row should always be null-actor; got %v", rows[0].ActorUserID)
	}
	want := "bad rcon from 203.0.113.99:51234"
	if rows[0].Detail != want {
		t.Errorf("detail = %q, want %q", rows[0].Detail, want)
	}
}
