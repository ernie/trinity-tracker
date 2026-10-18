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
	// No end marker: the connection just drops.

	sink := &fakeSink{}
	if err := ConsumeSegments(&buf, sink); err != nil {
		t.Fatalf("ConsumeSegments: %v", err)
	}
	if len(sink.segs) != 1 || !sink.ended {
		t.Fatalf("got %d segs ended=%v, want 1 true", len(sink.segs), sink.ended)
	}
}

func TestReadSegmentPayloadTooLarge(t *testing.T) {
	// The payload bytes are omitted, so the length alone must trip the guard.
	var buf bytes.Buffer
	buf.WriteString(segmentMagic) // "TVLs"
	var kf [4]byte
	buf.Write(kf[:]) // keyframe time = 0
	var lenBytes [4]byte
	binary.LittleEndian.PutUint32(lenBytes[:], uint32(maxSegmentBytes+1))
	buf.Write(lenBytes[:])

	_, err := ReadSegment(&buf)
	if err == nil {
		t.Fatal("ReadSegment: expected error for oversized payload, got nil")
	}
}

func TestConsumeSegmentsMidSegmentDropIsClean(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(Segment{KeyframeServerTime: 7, Payload: []byte("hello")}.Marshal())
	// A second segment that promises 5 payload bytes and delivers 2.
	buf.WriteString(segmentMagic)
	var kf [4]byte
	buf.Write(kf[:])
	var lenBytes [4]byte
	binary.LittleEndian.PutUint32(lenBytes[:], 5)
	buf.Write(lenBytes[:])
	buf.Write([]byte("ab"))

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
