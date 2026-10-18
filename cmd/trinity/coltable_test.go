package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderTable_AlignsThroughANSI(t *testing.T) {
	withColor(t, true)
	cols := []column{
		{header: "RANK", align: alignRight, cells: []string{"1", "2"}},
		{header: "PLAYER", cells: []string{
			"\x1b[31malice\x1b[0m",
			"rocketeer",
		}},
		{header: "FRAGS", align: alignRight, cells: []string{"412", "318"}},
	}
	var buf bytes.Buffer
	renderTable(&buf, cols)
	got := stripANSI(buf.String())

	// PLAYER is 9 wide, from "rocketeer"; the colored "alice" mustn't
	// widen it.
	wantLines := []string{
		"RANK  PLAYER     FRAGS",
		"----  ---------  -----",
		"   1  alice        412",
		"   2  rocketeer    318",
	}
	gotLines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(gotLines) != len(wantLines) {
		t.Fatalf("line count: got %d, want %d\n%s", len(gotLines), len(wantLines), got)
	}
	for i, want := range wantLines {
		if gotLines[i] != want {
			t.Errorf("line %d:\n  got  %q\n  want %q", i, gotLines[i], want)
		}
	}
}

func TestRenderTable_EmitsEscapesWhenEnabled(t *testing.T) {
	withColor(t, true)
	var buf bytes.Buffer
	renderTable(&buf, []column{{header: "X", cells: []string{"a"}}})
	out := buf.String()
	if !strings.Contains(out, "\x1b[1m") {
		t.Errorf("expected bold header escape; got %q", out)
	}
	if !strings.Contains(out, "\x1b[2m") {
		t.Errorf("expected dim separator escape; got %q", out)
	}
}

func TestRenderTable_NoEscapesWhenDisabled(t *testing.T) {
	withColor(t, false)
	var buf bytes.Buffer
	renderTable(&buf, []column{{header: "X", cells: []string{"a"}}})
	if strings.ContainsRune(buf.String(), 0x1b) {
		t.Errorf("expected no ESC bytes when disabled; got %q", buf.String())
	}
}

func TestRenderTable_EmptyHandled(t *testing.T) {
	var buf bytes.Buffer
	renderTable(&buf, nil)
	if buf.Len() != 0 {
		t.Errorf("empty table should emit nothing; got %q", buf.String())
	}
}

func stripANSI(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) {
				c := s[j]
				j++
				if c >= '@' && c <= '~' {
					break
				}
			}
			i = j
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
