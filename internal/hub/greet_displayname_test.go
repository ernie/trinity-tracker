package hub

import (
	"context"
	"testing"
	"time"
)

func TestGreet_DisplayNamePopulatedFromAuthVerifiedUser(t *testing.T) {
	w, store := newTestWriter(t)
	ctx := context.Background()

	pg, err := store.UpsertPlayerGUID(ctx, "test-guid", "TestPlayer", "TestPlayer", time.Now().UTC(), false)
	if err != nil {
		t.Fatalf("UpsertPlayerGUID: %v", err)
	}

	// CreateUser derives DisplayName from the username.
	if err := store.CreateUser(ctx, "alice", "hash", false, &pg.PlayerID); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	user, err := store.GetUserByUsername(ctx, "alice")
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}

	if _, err := store.RotateGameToken(ctx, user.ID); err != nil {
		t.Fatalf("RotateGameToken: %v", err)
	}

	user, err = store.GetUserByUsername(ctx, "alice")
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}

	nonce := "test-nonce"
	tokenHash := sipHashHex(user.GameToken, nonce)

	reply, err := w.Greet(ctx, GreetRequest{
		ServerID:     0,
		GUID:         "test-guid",
		ClientName:   "Test Client",
		CleanName:    "TestPlayer",
		ClientEngine: "q3",
		Auth: &AuthProof{
			Username:  "alice",
			Nonce:     nonce,
			TokenHash: tokenHash,
		},
	})

	if err != nil {
		t.Fatalf("Greet: %v", err)
	}

	if reply.AuthResult != AuthVerified {
		t.Errorf("AuthResult = %q, want %q", reply.AuthResult, AuthVerified)
	}

	if reply.DisplayName == "" {
		t.Error("DisplayName is empty, should be populated from user's DisplayName")
	}

	if user.DisplayName != "" && reply.DisplayName != user.DisplayName {
		t.Errorf("DisplayName = %q, want %q (from user account)", reply.DisplayName, user.DisplayName)
	}
}

func TestGreet_DisplayNameFallsBackToUsername(t *testing.T) {
	w, store := newTestWriter(t)
	ctx := context.Background()

	pg, err := store.UpsertPlayerGUID(ctx, "test-guid-2", "TestPlayer2", "TestPlayer2", time.Now().UTC(), false)
	if err != nil {
		t.Fatalf("UpsertPlayerGUID: %v", err)
	}

	if err := store.CreateUser(ctx, "bob", "hash", false, &pg.PlayerID); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	user, err := store.GetUserByUsername(ctx, "bob")
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}

	if _, err := store.RotateGameToken(ctx, user.ID); err != nil {
		t.Fatalf("RotateGameToken: %v", err)
	}

	user, err = store.GetUserByUsername(ctx, "bob")
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}

	nonce := "test-nonce-2"
	tokenHash := sipHashHex(user.GameToken, nonce)

	reply, err := w.Greet(ctx, GreetRequest{
		ServerID:     0,
		GUID:         "test-guid-2",
		ClientName:   "Test Client",
		CleanName:    "TestPlayer2",
		ClientEngine: "q3",
		Auth: &AuthProof{
			Username:  "bob",
			Nonce:     nonce,
			TokenHash: tokenHash,
		},
	})

	if err != nil {
		t.Fatalf("Greet: %v", err)
	}

	if reply.AuthResult != AuthVerified {
		t.Errorf("AuthResult = %q, want %q", reply.AuthResult, AuthVerified)
	}

	if reply.DisplayName == "" {
		t.Errorf("DisplayName should not be empty; fallback to username should apply")
	}
	if reply.DisplayName != user.DisplayName && reply.DisplayName != "bob" {
		t.Errorf("DisplayName = %q, want either %q (derived) or %q (fallback)", reply.DisplayName, user.DisplayName, "bob")
	}
}

func TestGreet_DisplayNameNotPopulatedWithoutAuth(t *testing.T) {
	w, store := newTestWriter(t)
	ctx := context.Background()

	_, err := store.UpsertPlayerGUID(ctx, "test-guid-3", "TestPlayer3", "TestPlayer3", time.Now().UTC(), false)
	if err != nil {
		t.Fatalf("UpsertPlayerGUID: %v", err)
	}

	reply, err := w.Greet(ctx, GreetRequest{
		ServerID:     0,
		GUID:         "test-guid-3",
		ClientName:   "Test Client",
		CleanName:    "TestPlayer3",
		ClientEngine: "q3",
		Auth:         nil,
	})

	if err != nil {
		t.Fatalf("Greet: %v", err)
	}

	if reply.AuthResult != AuthUnauthenticated {
		t.Errorf("AuthResult = %q, want %q", reply.AuthResult, AuthUnauthenticated)
	}

	if reply.DisplayName != "" {
		t.Errorf("DisplayName = %q, want empty (no auth)", reply.DisplayName)
	}
}
