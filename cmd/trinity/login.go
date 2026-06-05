package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/user"
	"strings"
	"time"

	flag "github.com/spf13/pflag"
	"golang.org/x/term"
)

// cliHTTPClient is shared by the token-authed CLI commands. Generous
// timeout: rcon round-trips through NATS can take a few seconds.
var cliHTTPClient = &http.Client{Timeout: 15 * time.Second}

// cliLoginBody mirrors api.CLILoginRequest.
type cliLoginBody struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

// cliLoginReply mirrors api.CLILoginResponse.
type cliLoginReply struct {
	Token    string `json:"token"`
	Username string `json:"username"`
}

// resolveLoginURL picks the hub URL for login: explicit flag, then the
// previously stored token's URL, then the local config (hub boxes).
func resolveLoginURL(flagURL, configPath string) string {
	if flagURL != "" {
		return strings.TrimRight(flagURL, "/")
	}
	if tok, err := loadCLIToken(); err == nil && tok != nil {
		return tok.URL
	}
	if cfg := loadCLIConfigFromFlags(configPath, ""); cfg != nil {
		return baseURL
	}
	return ""
}

func cmdLogin(args []string) {
	fs := flag.NewFlagSet("login", flag.ExitOnError)
	urlFlag := fs.String("url", "", "base URL of the trinity hub (e.g. https://trinity.run)")
	username := fs.String("username", "", "trinity username (prompted if omitted)")
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	fs.Parse(args)

	hubURL := resolveLoginURL(*urlFlag, *configPath)
	if hubURL == "" {
		fmt.Fprintln(os.Stderr, "Error: no hub URL. Use --url, e.g.: trinity login --url https://trinity.run")
		os.Exit(1)
	}
	warnPlaintextURL(hubURL)

	name := *username
	stdin := bufio.NewReader(os.Stdin)
	if name == "" {
		fmt.Print("Username: ")
		line, err := stdin.ReadString('\n')
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error: reading username")
			os.Exit(1)
		}
		name = strings.TrimSpace(line)
	}
	if name == "" {
		fmt.Fprintln(os.Stderr, "Error: username is required")
		os.Exit(1)
	}

	fmt.Print("Password: ")
	var password string
	if term.IsTerminal(int(os.Stdin.Fd())) {
		raw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error: reading password")
			os.Exit(1)
		}
		password = string(raw)
	} else {
		line, err := stdin.ReadString('\n')
		if err != nil && err != io.EOF {
			fmt.Fprintln(os.Stderr, "Error: reading password")
			os.Exit(1)
		}
		password = strings.TrimRight(line, "\r\n")
	}

	// A fresh login replaces any stored token for this URL — revoke the
	// old one first (best-effort) so casual re-logins don't accumulate
	// live PATs server-side.
	if old, err := loadCLIToken(); err == nil && old != nil && old.URL == hubURL {
		_ = revokeToken(old)
	}

	body, _ := json.Marshal(cliLoginBody{
		Username: name,
		Password: password,
		Name:     patName(),
	})
	resp, err := cliHTTPClient.Post(hubURL+"/api/auth/cli-login", "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// fall through to decode
	case http.StatusUnauthorized:
		fmt.Fprintln(os.Stderr, "Error: invalid credentials")
		os.Exit(1)
	case http.StatusForbidden:
		fmt.Fprintln(os.Stderr, "Error: password change required — log in to the web UI first, then retry")
		os.Exit(1)
	default:
		fmt.Fprintf(os.Stderr, "Error: login failed (%s)\n", resp.Status)
		os.Exit(1)
	}

	var reply cliLoginReply
	if err := json.NewDecoder(resp.Body).Decode(&reply); err != nil {
		fmt.Fprintf(os.Stderr, "Error: decoding response: %v\n", err)
		os.Exit(1)
	}
	if err := saveCLIToken(&cliToken{URL: hubURL, Username: reply.Username, Token: reply.Token}); err != nil {
		fmt.Fprintf(os.Stderr, "Error: saving token: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Logged in to %s as %s.\n", hubURL, reply.Username)
	fmt.Println("Token stored; revoke it any time with: trinity logout")
}

func cmdLogout(args []string) {
	fs := flag.NewFlagSet("logout", flag.ExitOnError)
	fs.Parse(args)

	tok, err := loadCLIToken()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if tok == nil {
		fmt.Println("Not logged in.")
		return
	}
	if err := revokeToken(tok); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not revoke server-side (%v); clearing local token anyway\n", err)
	}
	if err := deleteCLIToken(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Logged out of %s.\n", tok.URL)
}

// revokeToken asks the hub to revoke a PAT. A 401 means the token was
// already dead — success for our purposes.
func revokeToken(tok *cliToken) error {
	req, err := http.NewRequest(http.MethodDelete, tok.URL+"/api/auth/cli-login", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok.Token)
	resp, err := cliHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusUnauthorized {
		return fmt.Errorf("unexpected status %s", resp.Status)
	}
	return nil
}

// patName labels the minted token with where it lives.
func patName() string {
	who := "unknown"
	if u, err := user.Current(); err == nil {
		who = u.Username
	}
	host, err := os.Hostname()
	if err != nil {
		host = "unknown"
	}
	return "cli:" + who + "@" + host
}
