// internal/livestream/source.go
package livestream

import (
	"encoding/binary"
	"fmt"
	"io"
)

// maxSegmentBytes caps one segment's payload. It guards against a corrupt or
// hostile length prefix triggering a huge allocation. Sized well above any
// plausible compressed keyframe+deltas segment (engine frames are <=256 KiB
// uncompressed; a segment is a few seconds of compressed frames).
const maxSegmentBytes = 16 << 20 // 16 MiB

func ParseStreamHeader(r io.Reader) (StreamHeader, error) {
	var magic [4]byte
	if _, err := io.ReadFull(r, magic[:]); err != nil {
		return StreamHeader{}, err
	}
	if string(magic[:]) != streamMagic {
		return StreamHeader{}, fmt.Errorf("livestream: bad magic %q", magic[:])
	}
	ver, err := readU32(r)
	if err != nil {
		return StreamHeader{}, err
	}
	if ver != Version {
		return StreamHeader{}, fmt.Errorf("livestream: unsupported version %d", ver)
	}
	var h StreamHeader
	if h.SvFps, err = readU32(r); err != nil {
		return StreamHeader{}, err
	}
	if h.MaxClients, err = readU32(r); err != nil {
		return StreamHeader{}, err
	}
	if h.MapName, err = readStr(r); err != nil {
		return StreamHeader{}, err
	}
	if h.Timestamp, err = readStr(r); err != nil {
		return StreamHeader{}, err
	}
	if h.GameName, err = readStr(r); err != nil {
		return StreamHeader{}, err
	}
	return h, nil
}

// ReadSegment reads one Segment. It returns io.EOF on a clean end:
// either the "TVLe" end marker or a real EOF before any byte of a marker.
// A mid-segment drop yields io.ErrUnexpectedEOF.
func ReadSegment(r io.Reader) (Segment, error) {
	var marker [4]byte
	if _, err := io.ReadFull(r, marker[:]); err != nil {
		return Segment{}, err // io.EOF on clean end, io.ErrUnexpectedEOF on partial
	}
	switch string(marker[:]) {
	case endMagic:
		return Segment{}, io.EOF
	case segmentMagic:
		// fall through
	default:
		return Segment{}, fmt.Errorf("livestream: bad segment marker %q", marker[:])
	}

	kf, err := readU32(r)
	if err != nil {
		return Segment{}, err
	}
	n, err := readU32(r)
	if err != nil {
		return Segment{}, err
	}
	if n > maxSegmentBytes {
		return Segment{}, fmt.Errorf("livestream: segment payload too large (%d bytes)", n)
	}
	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		return Segment{}, err
	}
	return Segment{KeyframeServerTime: int32(kf), Payload: payload}, nil
}

// segmentSink receives parsed segments and an end signal. *Buffer implements it.
type segmentSink interface {
	Append(Segment)
	End()
}

// ConsumeSegments reads segments from r until a clean end (end marker, EOF,
// or abrupt mid-segment EOF) and feeds them to sink. A clean end is not an
// error. It always calls sink.End() before returning (even on a transport
// error) so viewers unblock.
func ConsumeSegments(r io.Reader, sink segmentSink) (err error) {
	defer sink.End()
	for {
		seg, e := ReadSegment(r)
		if e == io.EOF || e == io.ErrUnexpectedEOF {
			return nil
		}
		if e != nil {
			return e
		}
		sink.Append(seg)
	}
}

func readU32(r io.Reader) (uint32, error) {
	var b [4]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(b[:]), nil
}

func readStr(r io.Reader) (string, error) {
	var lb [2]byte
	if _, err := io.ReadFull(r, lb[:]); err != nil {
		return "", err
	}
	n := binary.LittleEndian.Uint16(lb[:])
	if n == 0 {
		return "", nil
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}
