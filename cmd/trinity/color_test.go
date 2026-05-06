package main

import (
	"testing"

	flag "github.com/spf13/pflag"
)

// withColor temporarily forces colorEnabled to v and restores it on
// cleanup. Tests that exercise rendering paths use it instead of
// touching env vars (which would race with parallel tests).
func withColor(t *testing.T, v bool) {
	t.Helper()
	prev := colorEnabled
	colorEnabled = v
	t.Cleanup(func() { colorEnabled = prev })
}

// addColorFlag wires three distinguishable states onto pflag, mirroring
// GNU coreutils. Regressing the NoOptDefVal silently would turn
// `trinity status --color` into a flag-parse error, so we lock all
// three forms in.
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
	// Unknown values fall back to auto. We can't assert the exact
	// value (depends on TTY of the test runner) — just that it
	// matches autoDetect.
	applyColorMode("garbage")
	if colorEnabled != autoDetectColor() {
		t.Error("unknown --color value should fall back to auto")
	}
}

// Helpers must be no-ops when color is disabled — that's the whole
// point of the wrapper. Without this the `tabwriter` alternative
// would be measuring zero-width escapes everywhere.
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

// displayName is the bridge from API rows (which carry both colored
// and clean variants) to terminal output. Honors colorEnabled so the
// same call site produces sensible output in both modes.
func TestDisplayName_FollowsColorEnabled(t *testing.T) {
	withColor(t, false)
	if got := displayName("^1ernie", "ernie"); got != "ernie" {
		t.Errorf("disabled: got %q, want plain clean_name", got)
	}
	withColor(t, true)
	// displayName uses q3ToANSI (bright variant 91m) so terminals
	// render true bright red rather than dark/amber red.
	if got := displayName("^1ernie", "ernie"); got != "\x1b[91mernie\x1b[0m" {
		t.Errorf("enabled: got %q, want q3ToANSI output", got)
	}
}
