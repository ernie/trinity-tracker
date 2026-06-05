package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	flag "github.com/spf13/pflag"
	"golang.org/x/term"

	"github.com/ernie/trinity-tracker/internal/discord"
)

// consoleServer mirrors api.ConsoleServer.
type consoleServer struct {
	Source  string `json:"source"`
	Key     string `json:"key"`
	Address string `json:"address"`
	Active  bool   `json:"active"`
	Role    string `json:"role"`
}

// apiError mirrors the server's writeError envelope.
type apiError struct {
	Error string `json:"error"`
}

// cmdConsole is the server-control surface:
//
//	trinity console                 list servers you can control
//	trinity console ffa status      one-shot rcon
//	trinity console ffa             interactive console (REPL)
//	trinity console eu/ctf status   source-qualified for remote sources
func cmdConsole(args []string) {
	fs := flag.NewFlagSet("console", flag.ExitOnError)
	urlFlag := fs.String("url", "", "base URL override (must match the login URL)")
	colorMode := addColorFlag(fs)
	fs.Parse(args)
	applyColorMode(*colorMode)

	tok := requireCLIToken(*urlFlag)

	rest := fs.Args()
	if len(rest) == 0 {
		listConsoleServers(tok)
		return
	}

	source, key := resolveTarget(tok, rest[0])
	if len(rest) > 1 {
		// One-shot: everything after the target is the rcon command.
		output, err := consoleRcon(tok, source, key, strings.Join(rest[1:], " "))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		printRconOutput(os.Stdout, output)
		return
	}

	runConsoleREPL(tok, source, key)
}

// requireCLIToken loads the stored credential or exits with guidance.
// An explicit --url must match the URL the token was minted against —
// a token is not portable across hubs.
func requireCLIToken(urlFlag string) *cliToken {
	tok, err := loadCLIToken()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if tok == nil {
		fmt.Fprintln(os.Stderr, "Error: not logged in. Run: trinity login --url <hub>")
		os.Exit(1)
	}
	if urlFlag != "" && strings.TrimRight(urlFlag, "/") != tok.URL {
		fmt.Fprintf(os.Stderr, "Error: logged in to %s, not %s. Run: trinity login --url %s\n",
			tok.URL, urlFlag, urlFlag)
		os.Exit(1)
	}
	return tok
}

// consoleAPI issues an authenticated request and decodes into out.
// A 401 is terminal: the token is revoked or invalid.
func consoleAPI(tok *cliToken, method, path string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, tok.URL+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := cliHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		fmt.Fprintln(os.Stderr, "Error: token revoked or invalid. Run: trinity login")
		os.Exit(1)
	}
	if resp.StatusCode != http.StatusOK {
		var ae apiError
		if json.NewDecoder(resp.Body).Decode(&ae) == nil && ae.Error != "" {
			return fmt.Errorf("%s", ae.Error)
		}
		return fmt.Errorf("unexpected status %s", resp.Status)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func fetchConsoleServers(tok *cliToken) []consoleServer {
	var servers []consoleServer
	if err := consoleAPI(tok, http.MethodGet, "/api/console/servers", nil, &servers); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	return servers
}

func listConsoleServers(tok *cliToken) {
	servers := fetchConsoleServers(tok)
	if len(servers) == 0 {
		fmt.Println("No servers you can control. (Owners see their sources; hub admins see delegated servers.)")
		return
	}
	var src, key, addr, status, role column
	src.header, key.header, addr.header, status.header, role.header =
		"SOURCE", "KEY", "ADDRESS", "STATUS", "ROLE"
	for _, s := range servers {
		src.cells = append(src.cells, s.Source)
		key.cells = append(key.cells, cyan(s.Key))
		addr.cells = append(addr.cells, s.Address)
		if s.Active {
			status.cells = append(status.cells, green("active"))
		} else {
			status.cells = append(status.cells, dim("inactive"))
		}
		role.cells = append(role.cells, s.Role)
	}
	renderTable(os.Stdout, []column{src, key, addr, status, role})
}

// matchTarget resolves "key" or "source/key" against a server list.
// A qualified target passes through untouched (the rcon endpoint is
// the authority); a bare key must match exactly one server.
func matchTarget(servers []consoleServer, target string) (source, key string, matches []consoleServer) {
	if s, k, ok := strings.Cut(target, "/"); ok {
		return s, k, nil
	}
	for _, s := range servers {
		if strings.EqualFold(s.Key, target) {
			matches = append(matches, s)
		}
	}
	if len(matches) == 1 {
		return matches[0].Source, matches[0].Key, matches
	}
	return "", "", matches
}

// resolveTarget turns "key" or "source/key" into a concrete (source,
// key) pair, using the server list for bare-key disambiguation.
func resolveTarget(tok *cliToken, target string) (source, key string) {
	if s, k, ok := strings.Cut(target, "/"); ok {
		return s, k
	}
	source, key, matches := matchTarget(fetchConsoleServers(tok), target)
	if source != "" {
		return source, key
	}
	if len(matches) == 0 {
		fmt.Fprintf(os.Stderr, "Error: no controllable server with key %q (try: trinity console)\n", target)
	} else {
		fmt.Fprintf(os.Stderr, "Error: key %q is ambiguous; qualify it:\n", target)
		for _, m := range matches {
			fmt.Fprintf(os.Stderr, "  trinity console %s/%s\n", m.Source, m.Key)
		}
	}
	os.Exit(1)
	return "", ""
}

func consoleRcon(tok *cliToken, source, key, command string) (string, error) {
	var resp struct {
		Output string `json:"output"`
	}
	body := map[string]string{"source": source, "key": key, "command": command}
	if err := consoleAPI(tok, http.MethodPost, "/api/console/rcon", body, &resp); err != nil {
		return "", err
	}
	return resp.Output, nil
}

// printRconOutput renders Q3 color codes as ANSI when color is on,
// strips them when piping.
func printRconOutput(w io.Writer, output string) {
	if output == "" {
		return
	}
	if colorEnabled {
		output = discord.Q3ToANSI(output)
	} else {
		output = discord.StripQ3Colors(output)
	}
	if !strings.HasSuffix(output, "\n") {
		output += "\n"
	}
	io.WriteString(w, output)
}

// runConsoleREPL is the interactive console: each line is sent as rcon
// and the reply printed inline. On a TTY it uses x/term for raw-mode
// line editing + history; otherwise a plain line reader so it can be
// scripted. Exit with Ctrl-D or /quit.
func runConsoleREPL(tok *cliToken, source, key string) {
	prompt := source + "/" + key + "> "
	fmt.Printf("Connected to %s/%s on %s. Ctrl-D or /quit to exit.\n", source, key, tok.URL)

	send := func(line string, echo io.Writer) {
		line = strings.TrimSpace(line)
		if line == "" {
			return
		}
		output, err := consoleRcon(tok, source, key, line)
		if err != nil {
			fmt.Fprintf(echo, "Error: %v\n", err)
			return
		}
		printRconOutput(echo, output)
	}

	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.TrimSpace(line) == "/quit" {
				return
			}
			send(line, os.Stdout)
		}
		return
	}

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: raw mode: %v\n", err)
		os.Exit(1)
	}
	defer term.Restore(fd, oldState)

	t := term.NewTerminal(stdinStdout{}, prompt)
	for {
		line, err := t.ReadLine()
		if err != nil {
			// io.EOF on Ctrl-D; any terminal error ends the session.
			return
		}
		if strings.TrimSpace(line) == "/quit" {
			return
		}
		// t.Write handles \n -> \r\n while the tty is raw.
		send(line, t)
	}
}

// stdinStdout is the io.ReadWriter term.Terminal drives: reads from
// the raw stdin, writes to stdout.
type stdinStdout struct{}

func (stdinStdout) Read(p []byte) (int, error)  { return os.Stdin.Read(p) }
func (stdinStdout) Write(p []byte) (int, error) { return os.Stdout.Write(p) }
