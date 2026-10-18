package main

import (
	"testing"

	flag "github.com/spf13/pflag"
)

// withColor sets colorEnabled for one test; env vars would race with
// parallel tests.
func withColor(t *testing.T, v bool) {
	t.Helper()
	prev := colorEnabled
	colorEnabled = v
	t.Cleanup(func() { colorEnabled = prev })
}

func TestAddColorFlag_ThreeStates(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"absent", []string{}, "auto"},
		{"bare", []string{"--color"}, "always"},
		{"explicit_always", []string{"--color=always"}, "always"},
		{"explicit_never", []string{"--color=never"}, "never"},
		{"explicit_auto", []string{"--color=auto"}, "auto"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			got := addColorFlag(fs)
			if err := fs.Parse(tc.args); err != nil {
				t.Fatalf("Parse(%v): %v", tc.args, err)
			}
			if *got != tc.want {
				t.Errorf("got %q, want %q (args=%v)", *got, tc.want, tc.args)
			}
		})
	}
}

func TestApplyColorMode(t *testing.T) {
	t.Cleanup(func() { colorEnabled = autoDetectColor() })

	applyColorMode("always")
	if !colorEnabled {
		t.Error("--color=always should enable")
	}
	applyColorMode("never")
	if colorEnabled {
		t.Error("--color=never should disable")
	}
	// The auto result depends on the runner's TTY.
	applyColorMode("garbage")
	if colorEnabled != autoDetectColor() {
		t.Error("unknown --color value should fall back to auto")
	}
}

func TestColorHelpers_NoOpWhenDisabled(t *testing.T) {
	withColor(t, false)
	cases := map[string]func(string) string{
		"bold":   bold,
		"faint":  faint,
		"red":    red,
		"green":  green,
		"yellow": yellow,
		"cyan":   cyan,
		"dim":    dim,
	}
	for name, fn := range cases {
		t.Run(name, func(t *testing.T) {
			if got := fn("hello"); got != "hello" {
				t.Errorf("%s should pass through when disabled, got %q", name, got)
			}
		})
	}
}

func TestColorHelpers_EmitWhenEnabled(t *testing.T) {
	withColor(t, true)
	if got := bold("x"); got != "\x1b[1mx\x1b[0m" {
		t.Errorf("bold: got %q", got)
	}
	if got := red("x"); got != "\x1b[31mx\x1b[0m" {
		t.Errorf("red: got %q", got)
	}
}

func TestDisplayName_FollowsColorEnabled(t *testing.T) {
	withColor(t, false)
	if got := displayName("^1alice", "alice"); got != "alice" {
		t.Errorf("disabled: got %q, want plain clean_name", got)
	}
	withColor(t, true)
	if got := displayName("^1alice", "alice"); got != "\x1b[91malice\x1b[0m" {
		t.Errorf("enabled: got %q, want q3ToANSI output", got)
	}
}
