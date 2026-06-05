package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
)

// cliToken is the stored CLI credential: a PAT minted by
// `trinity login` plus the hub URL it was minted against.
type cliToken struct {
	URL      string `json:"url"`
	Username string `json:"username"`
	Token    string `json:"token"`
}

// tokenPath returns ~/.config/trinity/token.json.
func tokenPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolving config dir: %w", err)
	}
	return filepath.Join(dir, "trinity", "token.json"), nil
}

// loadCLIToken reads the stored credential. Returns (nil, nil) when no
// token is stored — absent is not an error, it's a state.
func loadCLIToken() (*cliToken, error) {
	path, err := tokenPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var tok cliToken
	if err := json.Unmarshal(data, &tok); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if tok.URL == "" || tok.Token == "" {
		return nil, fmt.Errorf("parsing %s: missing url or token", path)
	}
	return &tok, nil
}

// saveCLIToken writes the credential 0600 under a 0700 dir.
func saveCLIToken(tok *cliToken) error {
	path, err := tokenPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(tok, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// deleteCLIToken removes the stored credential; missing file is fine.
func deleteCLIToken() error {
	path, err := tokenPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// warnPlaintextURL nags when a credential would travel over cleartext
// HTTP to somewhere that isn't this machine.
func warnPlaintextURL(rawURL string) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "http" {
		return
	}
	host := u.Hostname()
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return
	}
	fmt.Fprintf(os.Stderr, "Warning: sending credentials over plaintext http to %s\n", host)
}
