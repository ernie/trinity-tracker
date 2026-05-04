package storage

import (
	"context"
	"testing"
	"time"
)

func TestDirectoryRegistrationsRoundtrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	now := time.Unix(1_700_000_000, 0).UTC()
	rows := []DirectoryRegistration{
		{
			Addr: "10.0.0.1:27960", ServerID: 42, Protocol: 68,
			Gamename: "baseq3", Engine: "trinity-engine/0.4.2",
			Clients: 2, MaxClients: 16, Gametype: 4,
			ValidatedAt: now, ExpiresAt: now.Add(15 * time.Minute),
		},
		{
			Addr: "10.0.0.2:27961", ServerID: 43, Protocol: 68,
			Gamename: "missionpack", Engine: "trinity-engine/0.4.2",
			Clients: 0, MaxClients: 8, Gametype: 0,
			ValidatedAt: now.Add(-time.Minute), ExpiresAt: now.Add(14 * time.Minute),
		},
	}
	if err := s.ReplaceDirectoryRegistrations(ctx, rows); err != nil {
		t.Fatalf("replace: %v", err)
	}

	got, err := s.ListDirectoryRegistrations(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	byAddr := map[string]DirectoryRegistration{}
	for _, r := range got {
		byAddr[r.Addr] = r
	}
	for _, want := range rows {
		g, ok := byAddr[want.Addr]
		if !ok {
			t.Fatalf("missing addr %s", want.Addr)
		}
		if !g.ValidatedAt.Equal(want.ValidatedAt) || !g.ExpiresAt.Equal(want.ExpiresAt) {
			t.Errorf("%s: timestamps got=%v/%v want=%v/%v",
				want.Addr, g.ValidatedAt, g.ExpiresAt, want.ValidatedAt, want.ExpiresAt)
		}
		if g.ServerID != want.ServerID || g.Protocol != want.Protocol ||
			g.Gamename != want.Gamename || g.Engine != want.Engine ||
			g.Clients != want.Clients || g.MaxClients != want.MaxClients ||
			g.Gametype != want.Gametype {
			t.Errorf("%s: row mismatch got=%+v want=%+v", want.Addr, g, want)
		}
	}
}

func TestDirectoryRegistrationsReplaceClearsPrior(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()

	first := []DirectoryRegistration{{
		Addr: "10.0.0.1:27960", ServerID: 1, Protocol: 68,
		ValidatedAt: now, ExpiresAt: now.Add(15 * time.Minute),
	}}
	if err := s.ReplaceDirectoryRegistrations(ctx, first); err != nil {
		t.Fatalf("first: %v", err)
	}
	second := []DirectoryRegistration{{
		Addr: "10.0.0.2:27961", ServerID: 2, Protocol: 68,
		ValidatedAt: now, ExpiresAt: now.Add(15 * time.Minute),
	}}
	if err := s.ReplaceDirectoryRegistrations(ctx, second); err != nil {
		t.Fatalf("second: %v", err)
	}
	got, err := s.ListDirectoryRegistrations(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || got[0].Addr != "10.0.0.2:27961" {
		t.Errorf("after second replace got=%+v", got)
	}
}

func TestDirectoryRegistrationsClear(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()

	if err := s.ReplaceDirectoryRegistrations(ctx, []DirectoryRegistration{{
		Addr: "10.0.0.1:27960", ServerID: 1, Protocol: 68,
		ValidatedAt: now, ExpiresAt: now.Add(15 * time.Minute),
	}}); err != nil {
		t.Fatalf("replace: %v", err)
	}
	if err := s.ClearDirectoryRegistrations(ctx); err != nil {
		t.Fatalf("clear: %v", err)
	}
	got, err := s.ListDirectoryRegistrations(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("after clear got=%+v", got)
	}
}

func TestDirectoryRegistrationsReplaceEmpty(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()

	if err := s.ReplaceDirectoryRegistrations(ctx, []DirectoryRegistration{{
		Addr: "10.0.0.1:27960", ServerID: 1, Protocol: 68,
		ValidatedAt: now, ExpiresAt: now.Add(15 * time.Minute),
	}}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := s.ReplaceDirectoryRegistrations(ctx, nil); err != nil {
		t.Fatalf("replace empty: %v", err)
	}
	got, err := s.ListDirectoryRegistrations(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("after empty replace got=%+v", got)
	}
}
