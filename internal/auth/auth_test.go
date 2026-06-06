package auth

import (
	"testing"
	"time"
)

const testSecret = "test-secret-do-not-ship"

func TestNoExpiryTokenValidates(t *testing.T) {
	s := NewService(testSecret, 0)

	tok, err := s.GenerateToken(1, "ernie", true, nil, false, 3)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	claims, err := s.ValidateToken(tok)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if claims.ExpiresAt != nil {
		t.Errorf("duration 0 must mint without expiry, got %v", claims.ExpiresAt)
	}
	if claims.TokenVersion != 3 {
		t.Errorf("TokenVersion: want 3, got %d", claims.TokenVersion)
	}
	if claims.Username != "ernie" || claims.UserID != 1 || !claims.IsAdmin {
		t.Errorf("identity claims mangled: %+v", claims)
	}
}

func TestExplicitDurationStillExpires(t *testing.T) {
	// Positive duration: stamped and valid while fresh.
	s := NewService(testSecret, time.Hour)
	tok, err := s.GenerateToken(1, "ernie", false, nil, false, 1)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	claims, err := s.ValidateToken(tok)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if claims.ExpiresAt == nil {
		t.Fatal("positive duration must stamp an expiry")
	}

	// A token that has outlived its duration is rejected.
	s = NewService(testSecret, time.Millisecond)
	tok, err = s.GenerateToken(1, "ernie", false, nil, false, 1)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if _, err := s.ValidateToken(tok); err == nil {
		t.Error("expired token validated")
	}
}
