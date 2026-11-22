package assets

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func wavBytes(format uint16, rate, samples int) []byte {
	var b bytes.Buffer
	data := make([]byte, samples*2)
	for i := 0; i < samples; i++ {
		binary.LittleEndian.PutUint16(data[i*2:], uint16(int16(10000)))
	}
	w := func(v any) { _ = binary.Write(&b, binary.LittleEndian, v) }
	b.WriteString("RIFF")
	w(uint32(36 + len(data)))
	b.WriteString("WAVEfmt ")
	w(uint32(16))
	w(format)
	w(uint16(1))
	w(uint32(rate))
	w(uint32(rate * 2))
	w(uint16(2))
	w(uint16(16))
	b.WriteString("data")
	w(uint32(len(data)))
	b.Write(data)
	return b.Bytes()
}

func TestTrimWav(t *testing.T) {
	out, err := TrimWav(bytes.NewReader(wavBytes(1, 22050, 22050)), 0.55, 0.1)
	if err != nil {
		t.Fatal(err)
	}
	rate := 22050.0
	want := int(0.55 * rate)
	size := int(binary.LittleEndian.Uint32(out[40:44]))
	if size != want*2 || len(out) != 44+size {
		t.Fatalf("data size %d, file %d; want %d samples", size, len(out), want)
	}
	if last := int16(binary.LittleEndian.Uint16(out[len(out)-2:])); last != 0 {
		t.Fatalf("last sample %d, want 0 after the fade", last)
	}
	if first := int16(binary.LittleEndian.Uint16(out[44:46])); first != 10000 {
		t.Fatalf("first sample %d, want it untouched", first)
	}
}

func TestTrimWavRejectsFloat(t *testing.T) {
	if _, err := TrimWav(bytes.NewReader(wavBytes(3, 22050, 100)), 0.5, 0.1); err == nil {
		t.Fatal("float WAV trimmed")
	}
}
