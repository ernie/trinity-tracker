package assets

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

// TrimWav cuts a 16-bit PCM WAV to its first seconds, fading the last
// fadeSeconds to silence so the cut doesn't click.
func TrimWav(r io.Reader, seconds, fadeSeconds float64) ([]byte, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if len(raw) < 12 || string(raw[0:4]) != "RIFF" || string(raw[8:12]) != "WAVE" {
		return nil, fmt.Errorf("not a WAV file")
	}
	var fmtChunk, data []byte
	for p := 12; p+8 <= len(raw); {
		id, n := string(raw[p:p+4]), int(binary.LittleEndian.Uint32(raw[p+4:p+8]))
		body := raw[p+8 : min(len(raw), p+8+n)]
		switch id {
		case "fmt ":
			fmtChunk = body
		case "data":
			data = body
		}
		p += 8 + n + n%2
	}
	if len(fmtChunk) < 16 || data == nil {
		return nil, fmt.Errorf("WAV missing fmt or data chunk")
	}
	format := binary.LittleEndian.Uint16(fmtChunk[0:2])
	channels := int(binary.LittleEndian.Uint16(fmtChunk[2:4]))
	rate := int(binary.LittleEndian.Uint32(fmtChunk[4:8]))
	bits := binary.LittleEndian.Uint16(fmtChunk[14:16])
	if format != 1 || bits != 16 || channels < 1 || rate < 1 {
		return nil, fmt.Errorf("WAV is not 16-bit PCM (format %d, %d bits)", format, bits)
	}
	frameBytes := channels * 2
	frames := min(len(data)/frameBytes, int(seconds*float64(rate)))
	fade := min(frames, int(fadeSeconds*float64(rate)))
	out := make([]byte, frames*frameBytes)
	copy(out, data)
	for i := 0; i < fade; i++ {
		frame := frames - fade + i
		gain := float64(fade-1-i) / float64(max(1, fade-1))
		for c := 0; c < channels; c++ {
			o := frame*frameBytes + c*2
			s := float64(int16(binary.LittleEndian.Uint16(out[o:])))
			binary.LittleEndian.PutUint16(out[o:], uint16(int16(s*gain)))
		}
	}
	var b bytes.Buffer
	w := func(v any) { _ = binary.Write(&b, binary.LittleEndian, v) }
	b.WriteString("RIFF")
	w(uint32(36 + len(out)))
	b.WriteString("WAVEfmt ")
	w(uint32(16))
	b.Write(fmtChunk[:16])
	b.WriteString("data")
	w(uint32(len(out)))
	b.Write(out)
	return b.Bytes(), nil
}
