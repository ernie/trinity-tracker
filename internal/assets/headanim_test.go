package assets

import (
	"strings"
	"testing"
)

const gooseHeadAnim = `// name	first	count	looping	fps
TALK	1	5	0	20
TAUNT	6	39	0	20
PAIN	45	6	0	20
DEATH1	51	32	0	20
fx	TAUNT	gfx/geese/honk	5	300	0	45	36	0.28	0.34	136
fx	TAUNT	gfx/geese/honk	5	300	550	45	36	0.28	0.34	136
fx	TAUNT	gfx/geese/honk	5	300	1100	45	36	0.28	0.34	136
fx	TAUNT	gfx/geese/honk	5	300	1650	45	36	0.28	0.34	136
fxview	TAUNT	8	16	24	0.5
`

func TestParseHeadHonkGoose(t *testing.T) {
	got, ok := ParseHeadHonk(strings.NewReader(gooseHeadAnim))
	if !ok {
		t.Fatal("no honk parsed")
	}
	want := HeadHonk{First: 6, Count: 11, FPS: 20, Fx: HeadFx{Prefix: "gfx/geese/honk", Frames: 5, Msec: 300, Delay: 0,
		Width: 45, Height: 36, PivotX: 0.28, PivotY: 0.34, Angle: 136}}
	if got != want {
		t.Fatalf("ParseHeadHonk = %+v, want %+v", got, want)
	}
}

func TestParseHeadHonkOneFxLine(t *testing.T) {
	got, ok := ParseHeadHonk(strings.NewReader("TAUNT 6 39 0 20\nfx taunt gfx/geese/honk 5 300 0 45 36 0.28 0.34 136\n"))
	if !ok || got.Count != 39 {
		t.Fatalf("ParseHeadHonk = %+v, %v; want Count 39", got, ok)
	}
}

func TestParseHeadHonkNoFx(t *testing.T) {
	if _, ok := ParseHeadHonk(strings.NewReader("TAUNT 6 39 0 20\nfxview TAUNT 8 16 24 0.5\n")); ok {
		t.Fatal("honk parsed without an fx line")
	}
}

func TestParseHeadHonkNoTaunt(t *testing.T) {
	if _, ok := ParseHeadHonk(strings.NewReader("TALK 1 5 0 20\nfx TALK gfx/geese/honk 5 300 0 45 36 0.28 0.34 136\n")); ok {
		t.Fatal("honk parsed without a TAUNT clip")
	}
}

func TestHeadHonkClampedToModelFrames(t *testing.T) {
	h := HeadHonk{First: 6, Count: 11, FPS: 20}
	if got, ok := h.Clamped(211); !ok || got.Count != 11 {
		t.Fatalf("Clamped(211) = %+v, %v; want Count 11", got, ok)
	}
	if got, ok := h.Clamped(10); !ok || got.Count != 4 {
		t.Fatalf("Clamped(10) = %+v, %v; want Count 4", got, ok)
	}
	if _, ok := h.Clamped(6); ok {
		t.Fatal("Clamped(6) kept a honk that starts past the last frame")
	}
}
