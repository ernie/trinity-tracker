package main

import (
	"os"

	flag "github.com/spf13/pflag"
	"golang.org/x/term"
)

// colorEnabled gates every color-emitting helper in this binary.
// Resolved once per subcommand, after applyColorMode runs against the
// parsed --color flag. The default ("auto") consults NO_COLOR /
// FORCE_COLOR env vars (per https://no-color.org and the de-facto
// FORCE_COLOR convention) and falls back to TTY detection.
var colorEnabled = autoDetectColor()

func autoDetectColor() bool {
	// NO_COLOR wins outright. Any non-empty value disables.
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	// FORCE_COLOR overrides TTY detection — useful for piping
	// through `less -R` or capturing colored output to a file.
	if os.Getenv("FORCE_COLOR") != "" {
		return true
	}
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// addColorFlag wires --color=auto|always|never onto a subcommand's
// FlagSet. Returns the *string the caller passes to applyColorMode
// after flag parsing.
//
// Three states, matching GNU coreutils convention (`ls --color` etc.):
//   - flag absent: "auto" (the default — TTY/env-var driven)
//   - flag bare (`--color`): "always" (caller explicitly asked for it)
//   - flag with value (`--color=never`): that value
//
// pflag.NoOptDefVal is the mechanism that distinguishes "bare" from
// "absent" — without it pflag would error on the bare form.
func addColorFlag(fs *flag.FlagSet) *string {
	p := fs.String("color", "auto", "color output: auto|always|never (bare --color = always)")
	fs.Lookup("color").NoOptDefVal = "always"
	return p
}

// applyColorMode reconciles the --color flag with env-var / TTY
// detection. "auto" preserves the autoDetectColor result; "always"
// and "never" force the obvious. An unknown value is treated as
// auto rather than erroring — the flag is a hint, not load-bearing.
func applyColorMode(mode string) {
	switch mode {
	case "always":
		colorEnabled = true
	case "never":
		colorEnabled = false
	default: // "auto" or any unrecognized value
		colorEnabled = autoDetectColor()
	}
}

// SGR sequences. The integer codes are bog-standard ANSI:
// https://en.wikipedia.org/wiki/ANSI_escape_code#SGR
const (
	sgrReset  = "\x1b[0m"
	sgrBold   = "\x1b[1m"
	sgrFaint  = "\x1b[2m"
	sgrRed    = "\x1b[31m"
	sgrGreen  = "\x1b[32m"
	sgrYellow = "\x1b[33m"
	sgrCyan   = "\x1b[36m"
)

// wrap is the only place in this file that decides whether to emit
// escape sequences — every named helper goes through it. Call sites
// stay readable (`bold("ok")`) and the no-color path is a single
// branch.
func wrap(code, s string) string {
	if !colorEnabled {
		return s
	}
	return code + s + sgrReset
}

func bold(s string) string   { return wrap(sgrBold, s) }
func faint(s string) string  { return wrap(sgrFaint, s) }
func red(s string) string    { return wrap(sgrRed, s) }
func green(s string) string  { return wrap(sgrGreen, s) }
func yellow(s string) string { return wrap(sgrYellow, s) }
func cyan(s string) string   { return wrap(sgrCyan, s) }

// dim is an alias for faint — the SGR code is the same; some
// terminals render it as a dim grey, others as a true italic-faint.
// Spelled "dim" at call sites where the intent is "less prominent
// secondary text" (separators, metadata).
func dim(s string) string { return faint(s) }

// displayName picks between Q3-colored name and plain clean_name
// based on whether color is enabled. The leaderboard/players/matches
// subcommands all consume this — it's the bridge between API data
// (which carries both forms) and what the user sees.
func displayName(name, cleanName string) string {
	if colorEnabled {
		return q3ToANSI(name)
	}
	return cleanName
}
