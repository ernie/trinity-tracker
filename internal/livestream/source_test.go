// internal/livestream/source_test.go
package livestream

import (
	"bytes"
	"encoding/binary"
	"testing"
)

type fakeSink struct {
	segs  []Segment
	ended bool
}

func (f *fakeSink) Append(s Segment) { f.segs = append(f.segs, s) }
func (f *fakeSink) End()             { f.ended = true }

func TestConsumeSegments(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(Segment{KeyframeServerTime: 1, Payload: []byte("a")}.Marshal())
	buf.Write(Segment{KeyframeServerTime: 2, Payload: []byte("bb")}.Marshal())
	buf.WriteString(endMagic)

	sink := &fakeSink{}
	if err := ConsumeSegments(&buf, sink); err != nil {
		t.Fatalf("ConsumeSegments: %v", err)
	}
	if len(sink.segs) != 2 {
		t.Fatalf("got %d segments, want 2", len(sink.segs))
	}
	if sink.segs[1].KeyframeServerTime != 2 || string(sink.segs[1].Payload) != "bb" {
		t.Fatalf("second segment wrong: %+v", sink.segs[1])
	}
	if !sink.ended {
		t.Fatalf("sink not ended")
	}
}

func TestConsumeSegmentsAbruptEOFStillEnds(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(Segment{KeyframeServerTime: 1, Payload: []byte("a")}.Marshal())
	// no end marker — connection just drops

	sink := &fakeSink{}
	if err := ConsumeSegments(&buf, sink); err != nil {
		t.Fatalf("ConsumeSegments: %v", err)
	}
	if len(sink.segs) != 1 || !sink.ended {
		t.Fatalf("got %d segs ended=%v, want 1 true", len(sink.segs), sink.ended)
	}
}

func TestReadSegmentPayloadTooLarge(t *testing.T) {
	// Build a segment header with a payload length exceeding maxSegmentBytes.
	// We do NOT supply the payload bytes — the guard must reject on the length alone.
	var buf bytes.Buffer
	buf.WriteString(segmentMagic) // "TVLs"
	var kf [4]byte
	buf.Write(kf[:]) // keyframe time = 0
	var lenBytes [4]byte
	binary.LittleEndian.PutUint32(lenBytes[:], uint32(maxSegmentBytes+1))
	buf.Write(lenBytes[:])
	// No payload bytes follow.

	_, err := ReadSegment(&buf)
	if err == nil {
		t.Fatal("ReadSegment: expected error for oversized payload, got nil")
	}
}

func TestConsumeSegmentsMidSegmentDropIsClean(t *testing.T) {
	// One complete segment followed by a truncated second segment
	// (marker present but payload is cut short — mid-segment drop).
	var buf bytes.Buffer
	buf.Write(Segment{KeyframeServerTime: 7, Payload: []byte("hello")}.Marshal())
	// Start a second segment marker + keyframe time, then only 2 of a promised 5 bytes.
	buf.WriteString(segmentMagic)
	var kf [4]byte
	buf.Write(kf[:])
	var lenBytes [4]byte
	binary.LittleEndian.PutUint32(lenBytes[:], 5)
	buf.Write(lenBytes[:])
	buf.Write([]byte("ab")) // only 2 of 5 payload bytes — truncated

	sink := &fakeSink{}
	if err := ConsumeSegments(&buf, sink); err != nil {
		t.Fatalf("ConsumeSegments: expected nil for mid-segment drop, got %v", err)
	}
	if len(sink.segs) != 1 {
		t.Fatalf("got %d segments, want 1", len(sink.segs))
	}
	if !sink.ended {
		t.Fatal("sink not ended")
	}
}

func TestSegmentNegativeKeyframeRoundTrip(t *testing.T) {
	s := Segment{KeyframeServerTime: -5000, Payload: []byte("x")}
	raw := s.Marshal()
	got, err := ReadSegment(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadSegment: %v", err)
	}
	if got.KeyframeServerTime != -5000 {
		t.Fatalf("KeyframeServerTime = %d, want -5000", got.KeyframeServerTime)
	}
}
