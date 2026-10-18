package collector

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func appendLog(t *testing.T, path, s string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(s); err != nil {
		t.Fatal(err)
	}
}

func drainEvents(tl *LogTailer) []LogEvent {
	var out []LogEvent
	for {
		select {
		case ev := <-tl.Events:
			out = append(out, ev)
		default:
			return out
		}
	}
}

func TestReadNewContentKeepsPartialLineForNextRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "game.log")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	tl := NewLogTailer(path, nil)
	if _, err := tl.OpenFile(); err != nil {
		t.Fatal(err)
	}
	defer tl.file.Close()

	appendLog(t, path, "2026-06-12T10:00:00 ClientBegin: 5\n2026-06-12T10:00:01 ClientBe")
	if err := tl.readNewContent(); err != nil {
		t.Fatal(err)
	}
	appendLog(t, path, "gin: 6\n")
	if err := tl.readNewContent(); err != nil {
		t.Fatal(err)
	}

	evs := drainEvents(tl)
	if len(evs) != 2 {
		t.Fatalf("got %d events, want 2: %+v", len(evs), evs)
	}
	if want := time.Date(2026, 6, 12, 10, 0, 1, 0, time.UTC); !evs[1].Timestamp.Equal(want) {
		t.Errorf("second event at %v, want %v", evs[1].Timestamp, want)
	}
}

func TestReplayFromTimestampStopsBeforePartialLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "game.log")
	full := "2026-06-12T10:00:00 ClientBegin: 5\n"
	if err := os.WriteFile(path, []byte(full+"2026-06-12T10:00:01 ClientBe"), 0o644); err != nil {
		t.Fatal(err)
	}
	tl := NewLogTailer(path, nil)
	if _, err := tl.OpenFile(); err != nil {
		t.Fatal(err)
	}
	defer tl.file.Close()

	var n int
	if err := tl.ReplayFromTimestamp(time.Time{}, func(LogEvent, bool) { n++ }); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("replayed %d events, want 1", n)
	}
	if tl.position != int64(len(full)) {
		t.Fatalf("position %d, want %d", tl.position, len(full))
	}
}
