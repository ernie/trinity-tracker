package main

import (
	"os"

	flag "github.com/spf13/pflag"
	"golang.org/x/term"

	"github.com/ernie/trinity-tracker/internal/q3color"
)

// colorEnabled gates every color-emitting helper in this binary. It's
// set once per subcommand by applyColorMode; "auto" honors NO_COLOR
// (https://no-color.org) and FORCE_COLOR, then falls back to TTY
// detection.
var colorEnabled = autoDetectColor()

func autoDetectColor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	// For piping through `less -R` or capturing colored output.
	if os.Getenv("FORCE_COLOR") != "" {
		return true
	}
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// addColorFlag adds --color=auto|always|never to a subcommand, following
// GNU coreutils: absent means auto, a bare --color means always.
// NoOptDefVal is what lets the bare form parse.
func addColorFlag(fs *flag.FlagSet) *string {
	p := fs.String("color", "auto", "color output: auto|always|never (bare --color = always)")
	fs.Lookup("color").NoOptDefVal = "always"
	return p
}

// applyColorMode treats an unknown --color value as auto.
func applyColorMode(mode string) {
	switch mode {
	case "always":
		colorEnabled = true
	case "never":
		colorEnabled = false
	default:
		colorEnabled = autoDetectColor()
	}
}

// ANSI SGR sequences.
const (
	sgrReset  = "\x1b[0m"
	sgrBold   = "\x1b[1m"
	sgrFaint  = "\x1b[2m"
	sgrRed    = "\x1b[31m"
	sgrGreen  = "\x1b[32m"
	sgrYellow = "\x1b[33m"
	sgrCyan   = "\x1b[36m"
)

// wrap is the one place that decides whether to emit escape sequences.
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

// dim marks secondary text such as separators and metadata.
func dim(s string) string { return faint(s) }

// displayName shows the Q3-colored name when color is on, else clean_name.
func displayName(name, cleanName string) string {
	if colorEnabled {
		return q3color.ToANSI(name)
	}
	return cleanName
}
