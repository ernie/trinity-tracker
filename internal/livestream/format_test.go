// internal/livestream/format_test.go
package livestream

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

func TestConstants(t *testing.T) {
	if Version != 1 {
		t.Fatalf("Version = %d, want 1", Version)
	}
	if streamMagic != "TVL1" || segmentMagic != "TVLs" || endMagic != "TVLe" {
		t.Fatalf("magics wrong: %q %q %q", streamMagic, segmentMagic, endMagic)
	}
}

func TestStreamHeaderRoundTrip(t *testing.T) {
	h := StreamHeader{SvFps: 20, MaxClients: 8, MapName: "q3dm6", Timestamp: "2026-05-31T00:00:00Z", GameName: "missionpack"}
	raw := h.Marshal()

	if string(raw[0:4]) != "TVL1" {
		t.Fatalf("magic = %q, want TVL1", raw[0:4])
	}

	got, err := ParseStreamHeader(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ParseStreamHeader: %v", err)
	}
	if got != h {
		t.Fatalf("round-trip = %+v, want %+v", got, h)
	}
}

func TestParseStreamHeaderBadMagic(t *testing.T) {
	raw := []byte("XXXX\x01\x00\x00\x00")
	if _, err := ParseStreamHeader(bytes.NewReader(raw)); err == nil {
		t.Fatalf("expected error on bad magic")
	}
}

func TestSegmentRoundTrip(t *testing.T) {
	s := Segment{KeyframeServerTime: 12345, Payload: []byte("opaque-zstd-bytes")}
	raw := s.Marshal()
	if string(raw[0:4]) != "TVLs" {
		t.Fatalf("marker = %q, want TVLs", raw[0:4])
	}

	got, err := ReadSegment(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadSegment: %v", err)
	}
	if got.KeyframeServerTime != s.KeyframeServerTime || !bytes.Equal(got.Payload, s.Payload) {
		t.Fatalf("round-trip = %+v, want %+v", got, s)
	}
}

func TestReadSegmentEndMarker(t *testing.T) {
	raw := []byte(endMagic)
	_, err := ReadSegment(bytes.NewReader(raw))
	if !errors.Is(err, io.EOF) {
		t.Fatalf("end marker should yield io.EOF, got %v", err)
	}
}

func TestReadSegmentCleanEOF(t *testing.T) {
	_, err := ReadSegment(bytes.NewReader(nil))
	if !errors.Is(err, io.EOF) {
		t.Fatalf("empty reader should yield io.EOF, got %v", err)
	}
}
