package storage

import (
	"context"
	"testing"
)

func TestListUsersWithPlayerIncludesDisplayName(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.CreateUser(ctx, "alice", "hash", false, nil); err != nil {
		t.Fatal(err)
	}
	alice, err := s.GetUserByUsername(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AdminSetUserDisplayName(ctx, alice.ID, "^1Neo", "neo"); err != nil {
		t.Fatal(err)
	}

	users, err := s.ListUsersWithPlayer(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0].DisplayName != "^1Neo" {
		t.Fatalf("ListUsersWithPlayer = %+v, want one user with DisplayName ^1Neo", users)
	}

	found, err := s.SearchUsersWithPlayer(ctx, "ali", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].DisplayName != "^1Neo" {
		t.Fatalf("SearchUsersWithPlayer = %+v, want one user with DisplayName ^1Neo", found)
	}
}
