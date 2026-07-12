package storage

import (
	"context"
	"errors"
	"testing"
)

func TestCanonicalizeDisplayName(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Foo", "foo"},
		{"^1Foo", "foo"},
		{"^1Foo^7Bar", "foobar"},
		{"[VR] Foo", "foo"},
		{"^7[VR] ^1Foo", "foo"},
		{"Foo  Bar", "foo bar"},
		{"  Foo  Bar  ", "foo bar"},
		{"Foo Bar [VR]", "foo bar"},
		{"^1Foo^7   Bar^1", "foo bar"},
		{"", ""},
		{"[VR]", ""},
		// Case-insensitive: different casing, same canonical form
		{"Foo", "foo"},
		{"FOO", "foo"},
	}
	for _, c := range cases {
		if got := CanonicalizeDisplayName(c.in); got != c.want {
			t.Errorf("CanonicalizeDisplayName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func displayNameColumns(t *testing.T, s *Store, ctx context.Context, username string) (raw, canonical string) {
	t.Helper()
	err := s.db.QueryRowContext(ctx,
		`SELECT display_name, display_name_canonical FROM users WHERE username = ?`,
		username,
	).Scan(&raw, &canonical)
	if err != nil {
		t.Fatalf("read display name columns: %v", err)
	}
	return raw, canonical
}

func TestAdminSetUserDisplayName(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.CreateUser(ctx, "alice", "hash", false, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateUser(ctx, "bob", "hash", false, nil); err != nil {
		t.Fatal(err)
	}
	alice, err := s.GetUserByUsername(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	bob, err := s.GetUserByUsername(ctx, "bob")
	if err != nil {
		t.Fatal(err)
	}

	if err := s.AdminSetUserDisplayName(ctx, alice.ID, "^1Neo", "neo"); err != nil {
		t.Fatalf("set: %v", err)
	}
	raw, cano := displayNameColumns(t, s, ctx, "alice")
	if raw != "^1Neo" || cano != "neo" {
		t.Fatalf("columns = (%q, %q), want (^1Neo, neo)", raw, cano)
	}

	if err := s.AdminSetUserDisplayName(ctx, alice.ID, "^4Neo", "neo"); err != nil {
		t.Fatalf("recolor own name: %v", err)
	}
	raw, cano = displayNameColumns(t, s, ctx, "alice")
	if raw != "^4Neo" || cano != "neo" {
		t.Fatalf("columns = (%q, %q), want (^4Neo, neo)", raw, cano)
	}

	if err := s.AdminSetUserDisplayName(ctx, alice.ID, "Trinity", "trinity"); err != nil {
		t.Fatalf("change identity: %v", err)
	}

	if err := s.AdminSetUserDisplayName(ctx, bob.ID, "^7Trinity", "trinity"); !errors.Is(err, ErrDisplayNameTaken) {
		t.Fatalf("taken: err = %v, want ErrDisplayNameTaken", err)
	}
	if raw, _ := displayNameColumns(t, s, ctx, "bob"); raw != "" {
		t.Fatalf("bob display_name = %q, want unchanged empty", raw)
	}

	if err := s.AdminSetUserDisplayName(ctx, bob.ID, "Sarge", "sarge"); !errors.Is(err, ErrDisplayNameReserved) {
		t.Fatalf("reserved: err = %v, want ErrDisplayNameReserved", err)
	}

	if err := s.AdminSetUserDisplayName(ctx, 99999, "Ghost", "ghost"); err == nil {
		t.Fatal("unknown user: err = nil, want error")
	}

	// Every unset user holds display_name_canonical = ''.
	if err := s.AdminSetUserDisplayName(ctx, bob.ID, "", ""); err == nil || errors.Is(err, ErrDisplayNameTaken) {
		t.Fatalf("empty canonical: err = %v, want non-taken error", err)
	}
}
