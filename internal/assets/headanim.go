package assets

import (
	"bufio"
	"io"
	"math"
	"strconv"
	"strings"
)

// HeadFx is one headanim.cfg fx line: a run of drawings shown at the beak.
type HeadFx struct {
	Prefix string  `json:"prefix"`
	Frames int     `json:"frames"`
	Msec   int     `json:"msec"`
	Delay  int     `json:"delay"`
	Width  float32 `json:"width"`
	Height float32 `json:"height"`
	PivotX float32 `json:"pivotX"`
	PivotY float32 `json:"pivotY"`
	Angle  float32 `json:"angle"`
}

// HeadHonk is one honk: the start of the TAUNT clip and its first fx line.
type HeadHonk struct {
	First, Count, FPS int
	Fx                HeadFx
}

func clampf(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }

// ParseHeadHonk reads a headanim.cfg. A taunt with several fx lines honks
// several times, so one honk ends where the second fx line starts.
func ParseHeadHonk(r io.Reader) (HeadHonk, bool) {
	var honk HeadHonk
	var fx []HeadFx
	haveClip := false
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := sc.Text()
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		nums := func(from, n int) ([]float64, bool) {
			if len(f) < from+n {
				return nil, false
			}
			out := make([]float64, n)
			for i := range out {
				v, err := strconv.ParseFloat(f[from+i], 64)
				if err != nil {
					return nil, false
				}
				out[i] = v
			}
			return out, true
		}
		switch {
		case strings.EqualFold(f[0], "TAUNT"):
			if v, ok := nums(1, 4); ok {
				honk.First, honk.Count, honk.FPS = int(v[0]), int(v[1]), int(v[3])
				haveClip = honk.Count > 0 && honk.FPS > 0
			}
		case strings.EqualFold(f[0], "fx") && len(f) > 2 && strings.EqualFold(f[1], "TAUNT"):
			if v, ok := nums(3, 8); ok {
				fx = append(fx, HeadFx{
					Prefix: f[2],
					Frames: int(clampf(v[0], 1, 8)),
					Msec:   int(clampf(v[1], 50, 3000)),
					Delay:  int(clampf(v[2], 0, 3000)),
					Width:  float32(clampf(v[3], 0.5, 64)),
					Height: float32(clampf(v[4], 0.5, 64)),
					PivotX: float32(v[5]),
					PivotY: float32(v[6]),
					Angle:  float32(v[7]),
				})
			}
		}
	}
	if !haveClip || len(fx) == 0 {
		return HeadHonk{}, false
	}
	honk.Fx = fx[0]
	if len(fx) > 1 {
		if n := int(math.Ceil(float64(fx[1].Delay) * float64(honk.FPS) / 1000)); n > 0 && n < honk.Count {
			honk.Count = n
		}
	}
	return honk, true
}

// Clamped fits the honk to a model with frames frames, as the game clamps a
// clip that runs past the model's last frame.
func (h HeadHonk) Clamped(frames int) (HeadHonk, bool) {
	if h.First >= frames {
		return h, false
	}
	h.Count = min(h.Count, frames-h.First)
	return h, true
}
