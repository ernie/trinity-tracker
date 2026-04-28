package setup

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"
)

// Prompter abstracts user input so the wizard is testable without a
// real TTY. The default Prompter wraps os.Stdin/Stderr and
// term.ReadPassword for masked input.
type Prompter interface {
	// Line reads a non-empty line, falling back to def if the operator
	// just hits enter. If def is empty and the operator gives no
	// answer, it re-prompts.
	Line(prompt, def string) (string, error)

	// Optional reads a line that's allowed to be empty.
	Optional(prompt, def string) (string, error)

	// Int reads an integer in [min, max], falling back to def.
	Int(prompt string, def, min, max int) (int, error)

	// YesNo returns the operator's yes/no choice. If def is true, "Y/n"
	// is shown; if false, "y/N".
	YesNo(prompt string, def bool) (bool, error)

	// Choose presents a numbered list and returns the chosen index.
	Choose(prompt string, options []string, def int) (int, error)

	// Password reads a line with echo disabled. allowEmpty controls
	// whether an empty answer is accepted (used to mean "generate one
	// for me").
	Password(prompt string, allowEmpty bool) (string, error)
}

// IsTTY reports whether stdin is connected to a real terminal.
// `trinity init` uses this to decide between interactive prompts and
// strict-flag mode.
func IsTTY() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// NewStdPrompter wraps stdin (line reader) and stderr (prompt
// display) with optional masked-password support against the stdin
// fd. Output goes to stderr so the wizard can be re-run with stdout
// captured (for piping the rendered config etc.) without prompts
// landing in the captured stream.
func NewStdPrompter() Prompter {
	return &stdPrompter{
		in:    bufio.NewReader(os.Stdin),
		out:   os.Stderr,
		fd:    int(os.Stdin.Fd()),
		isTTY: IsTTY(),
	}
}

type stdPrompter struct {
	in    *bufio.Reader
	out   io.Writer
	fd    int
	isTTY bool
}

func (p *stdPrompter) readLine() (string, error) {
	line, err := p.in.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func (p *stdPrompter) Line(prompt, def string) (string, error) {
	for {
		if def != "" {
			fmt.Fprintf(p.out, "%s [%s]: ", prompt, def)
		} else {
			fmt.Fprintf(p.out, "%s: ", prompt)
		}
		line, err := p.readLine()
		if err != nil {
			return "", err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			if def != "" {
				return def, nil
			}
			fmt.Fprintln(p.out, "  (required)")
			continue
		}
		return line, nil
	}
}

func (p *stdPrompter) Optional(prompt, def string) (string, error) {
	if def != "" {
		fmt.Fprintf(p.out, "%s [%s]: ", prompt, def)
	} else {
		fmt.Fprintf(p.out, "%s (blank to skip): ", prompt)
	}
	line, err := p.readLine()
	if err != nil {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return def, nil
	}
	return line, nil
}

func (p *stdPrompter) Int(prompt string, def, min, max int) (int, error) {
	for {
		fmt.Fprintf(p.out, "%s [%d]: ", prompt, def)
		line, err := p.readLine()
		if err != nil {
			return 0, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			return def, nil
		}
		n, err := strconv.Atoi(line)
		if err != nil {
			fmt.Fprintf(p.out, "  not a number: %v\n", err)
			continue
		}
		if n < min || n > max {
			fmt.Fprintf(p.out, "  out of range [%d, %d]\n", min, max)
			continue
		}
		return n, nil
	}
}

func (p *stdPrompter) YesNo(prompt string, def bool) (bool, error) {
	hint := "y/N"
	if def {
		hint = "Y/n"
	}
	for {
		fmt.Fprintf(p.out, "%s [%s]: ", prompt, hint)
		line, err := p.readLine()
		if err != nil {
			return false, err
		}
		line = strings.ToLower(strings.TrimSpace(line))
		if line == "" {
			return def, nil
		}
		switch line {
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		}
		fmt.Fprintln(p.out, "  please answer yes or no")
	}
}

func (p *stdPrompter) Choose(prompt string, options []string, def int) (int, error) {
	if def < 0 || def >= len(options) {
		def = 0
	}
	for {
		fmt.Fprintln(p.out, prompt)
		for i, opt := range options {
			marker := " "
			if i == def {
				marker = "*"
			}
			fmt.Fprintf(p.out, "  %s %d) %s\n", marker, i+1, opt)
		}
		fmt.Fprintf(p.out, "Choice [%d]: ", def+1)
		line, err := p.readLine()
		if err != nil {
			return 0, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			return def, nil
		}
		n, err := strconv.Atoi(line)
		if err != nil || n < 1 || n > len(options) {
			fmt.Fprintf(p.out, "  enter 1-%d\n", len(options))
			continue
		}
		return n - 1, nil
	}
}

func (p *stdPrompter) Password(prompt string, allowEmpty bool) (string, error) {
	for {
		hint := ""
		if allowEmpty {
			hint = " (blank to generate)"
		}
		fmt.Fprintf(p.out, "%s%s: ", prompt, hint)
		var (
			pw  string
			err error
		)
		if p.isTTY {
			b, perr := term.ReadPassword(p.fd)
			fmt.Fprintln(p.out)
			if perr != nil {
				return "", perr
			}
			pw = string(b)
		} else {
			pw, err = p.readLine()
			if err != nil {
				return "", err
			}
		}
		pw = strings.TrimSpace(pw)
		if pw == "" && !allowEmpty {
			fmt.Fprintln(p.out, "  (required)")
			continue
		}
		return pw, nil
	}
}
