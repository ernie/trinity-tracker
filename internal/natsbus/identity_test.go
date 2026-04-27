package natsbus

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func TestLoadOrCreateSourceUUIDCreatesOnFirstRun(t *testing.T) {
	dir := t.TempDir()
	first, err := LoadOrCreateSourceUUID(dir)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, err := uuid.Parse(first); err != nil {
		t.Fatalf("returned value is not a UUID: %q", first)
	}

	// File was persisted with trailing newline.
	body, err := os.ReadFile(filepath.Join(dir, sourceUUIDFilename))
	if err != nil {
		t.Fatalf("read persisted: %v", err)
	}
	if got := string(body); got != first+"\n" {
		t.Errorf("persisted = %q, want %q", got, first+"\n")
	}
}

func TestLoadOrCreateSourceUUIDStableAcrossCalls(t *testing.T) {
	dir := t.TempDir()
	first, err := LoadOrCreateSourceUUID(dir)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := LoadOrCreateSourceUUID(dir)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if first != second {
		t.Errorf("second call changed UUID: %q -> %q", first, second)
	}
}

func TestLoadOrCreateSourceUUIDRejectsInvalid(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, sourceUUIDFilename), []byte("not-a-uuid"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := LoadOrCreateSourceUUID(dir); err == nil {
		t.Error("expected error for invalid persisted UUID")
	}
}

func TestLoadOrCreateSourceUUIDRequiresDir(t *testing.T) {
	if _, err := LoadOrCreateSourceUUID(""); err == nil {
		t.Error("expected error for empty dir")
	}
}
