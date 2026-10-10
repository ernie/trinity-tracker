package storage

import (
	"context"
	"testing"
	"time"

	"github.com/ernie/trinity-tracker/internal/domain"
)

func newTestSession(t *testing.T, s *Store, guid string, isVR bool) *domain.Session {
	t.Helper()
	ctx := context.Background()
	srv := &domain.Server{Key: "ffa", Address: "127.0.0.1:27960"}
	if err := s.UpsertServer(ctx, "test", srv); err != nil {
		t.Fatalf("UpsertServer: %v", err)
	}
	pg, err := s.UpsertPlayerGUID(ctx, guid, "Player", "Player", time.Now(), isVR)
	if err != nil {
		t.Fatalf("UpsertPlayerGUID: %v", err)
	}
	sess := &domain.Session{PlayerGUIDID: pg.ID, ServerID: srv.ID, JoinedAt: time.Now().UTC(), IsVR: isVR}
	if err := s.CreateSession(ctx, sess); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	return sess
}

func sessionPlatformVR(t *testing.T, s *Store, id int64) (string, bool) {
	t.Helper()
	var platform string
	var isVR bool
	if err := s.db.QueryRowContext(context.Background(),
		`SELECT platform, is_vr FROM sessions WHERE id = ?`, id).Scan(&platform, &isVR); err != nil {
		t.Fatalf("read session: %v", err)
	}
	return platform, isVR
}

func TestCreateSessionRecordsVR(t *testing.T) {
	s := newTestStore(t)
	sess := newTestSession(t, s, "GUIDVR", true)
	if _, isVR := sessionPlatformVR(t, s, sess.ID); !isVR {
		t.Error("session joined in VR has is_vr false")
	}
}

func TestMarkSessionVRIsSticky(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	sess := newTestSession(t, s, "GUIDFLAT", false)
	if _, isVR := sessionPlatformVR(t, s, sess.ID); isVR {
		t.Fatal("flat session starts with is_vr true")
	}
	if err := s.MarkSessionVR(ctx, sess.ID); err != nil {
		t.Fatalf("MarkSessionVR: %v", err)
	}
	if err := s.MarkSessionVR(ctx, sess.ID); err != nil {
		t.Fatalf("MarkSessionVR again: %v", err)
	}
	if _, isVR := sessionPlatformVR(t, s, sess.ID); !isVR {
		t.Error("is_vr false after MarkSessionVR")
	}
}

func TestUpdateSessionClientInfoRecordsPlatform(t *testing.T) {
	s := newTestStore(t)
	sess := newTestSession(t, s, "GUIDPLAT", false)
	if err := s.UpdateSessionClientInfo(context.Background(), sess.ID, "Trinity_Engine_v1.1.0", "v1.1.0", "pico"); err != nil {
		t.Fatalf("UpdateSessionClientInfo: %v", err)
	}
	if platform, _ := sessionPlatformVR(t, s, sess.ID); platform != "pico" {
		t.Errorf("platform = %q, want pico", platform)
	}
}

func TestGetPlatformStats(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	quest := newTestSession(t, s, "GUIDQ", true)
	pc := newTestSession(t, s, "GUIDPC", false)
	if err := s.UpdateSessionClientInfo(ctx, quest.ID, "e", "v", "quest"); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateSessionClientInfo(ctx, pc.ID, "e", "v", "windows"); err != nil {
		t.Fatal(err)
	}
	if err := s.EndSession(ctx, pc.ID, pc.JoinedAt.Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}

	stats, err := s.GetPlatformStats(ctx, nil, nil)
	if err != nil {
		t.Fatalf("GetPlatformStats: %v", err)
	}
	if stats.Players != 2 || stats.VRPlayers != 1 || stats.Sessions != 2 || stats.PlaytimeSeconds != 600 {
		t.Errorf("totals = %+v, want 2 players, 1 VR, 2 sessions, 600 s", stats)
	}
	got := map[string]bool{}
	for _, r := range stats.Rows {
		got[r.Platform] = r.IsVR
	}
	if len(stats.Rows) != 2 || !got["quest"] || got["windows"] {
		t.Errorf("rows = %+v, want quest VR and windows flat", stats.Rows)
	}

	future := time.Now().Add(time.Hour)
	stats, err = s.GetPlatformStats(ctx, &future, nil)
	if err != nil {
		t.Fatalf("GetPlatformStats since: %v", err)
	}
	if stats.Players != 0 || len(stats.Rows) != 0 {
		t.Errorf("future window = %+v, want empty", stats)
	}
}
