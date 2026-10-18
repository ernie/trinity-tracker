// Package livestream defines the wire format for watching a match live
// and the collector's relay, which fans a match in progress out to many
// browser viewers on a configurable delay.
//
// The format is little-endian: a StreamHeader, then Segments. Each
// Segment starts with a clear-text marker, keyframe time and payload
// length, so the relay can find segment boundaries without
// decompressing the zstd payload.
package livestream

import (
	"encoding/binary"
)

const (
	streamMagic  = "TVL1"
	segmentMagic = "TVLs"
	endMagic     = "TVLe"

	Version uint32 = 1
)

// StreamHeader is the once-per-stream preamble.
type StreamHeader struct {
	SvFps      uint32
	MaxClients uint32
	MapName    string
	Timestamp  string
	// GameName is fs_game, such as "missionpack"; empty for baseq3.
	GameName string
}

// Marshal encodes the header, magic + version first.
func (h StreamHeader) Marshal() []byte {
	var b []byte
	b = append(b, streamMagic...)
	b = le32(b, Version)
	b = le32(b, h.SvFps)
	b = le32(b, h.MaxClients)
	b = leStr(b, h.MapName)
	b = leStr(b, h.Timestamp)
	b = leStr(b, h.GameName)
	return b
}

// Segment is one keyframe-led unit of the stream. Payload is opaque (zstd).
type Segment struct {
	KeyframeServerTime int32
	Payload            []byte
}

// Marshal encodes the segment including its "TVLs" marker and clear-text
// length, so a relay can skip it without decompressing.
func (s Segment) Marshal() []byte {
	b := make([]byte, 0, 12+len(s.Payload))
	b = append(b, segmentMagic...)
	b = le32(b, uint32(s.KeyframeServerTime))
	b = le32(b, uint32(len(s.Payload)))
	return append(b, s.Payload...)
}

func le32(b []byte, v uint32) []byte {
	var tmp [4]byte
	binary.LittleEndian.PutUint32(tmp[:], v)
	return append(b, tmp[:]...)
}

func leStr(b []byte, s string) []byte {
	var tmp [2]byte
	binary.LittleEndian.PutUint16(tmp[:], uint16(len(s)))
	b = append(b, tmp[:]...)
	return append(b, s...)
}
