package setup

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RunWizard walks the operator through every prompt the install
// needs and returns a populated Answers. mode-specific prompts
// branch off the leading mode question; per-server prompts repeat
// until the operator answers "no" to "add another?".
//
// allowHub controls whether the operator may pick a hub-bearing mode.
// The default front door (`scripts/install.sh`, `trinity init`) runs
// collector-only — Trinity is a network of collectors joining a shared
// hub at trinity.run, and extra hubs are easy to stand up by accident
// and hard to consolidate later. Hub installs are an expert path:
// `trinity init --allow-hub`, documented in distributed-deployment.md.
func RunWizard(p Prompter, out io.Writer, allowHub bool) (*Answers, error) {
	fmt.Fprintln(out, "Welcome to Trinity setup.")
	fmt.Fprintln(out)

	a := &Answers{
		ServiceUser:  "quake",
		ListenAddr:   "127.0.0.1",
		HTTPPort:     8080,
		DatabasePath: "/var/lib/trinity/trinity.db",
		StaticDir:    "/var/lib/trinity/web",
		Quake3Dir:    "/usr/lib/quake3",
	}

	if !allowHub {
		a.Mode = ModeCollector
	} else {
		// When hub modes are unlocked we still default to collector —
		// the operator has to actively choose to stand up a hub.
		modeIdx, err := p.Choose("What kind of install?", []string{
			ModeCombined.String() + "  (single machine)",
			ModeHubOnly.String() + "  (central UI; remote collectors report in)",
			ModeCollector.String() + "  (watches local q3 servers; reports to a remote hub)",
		}, int(ModeCollector))
		if err != nil {
			return nil, err
		}
		a.Mode = Mode(modeIdx)
	}

	// Collector installs need three things from the hub admin before
	// they can do anything useful. Gate up front so an operator who
	// doesn't have them yet bails out before answering ten questions.
	if a.Mode == ModeCollector {
		if err := confirmCollectorPrereqs(p, out); err != nil {
			return nil, err
		}
	}

	if err := promptCommon(p, a); err != nil {
		return nil, err
	}

	if a.HasHubFields() {
		if err := promptHub(p, a); err != nil {
			return nil, err
		}
	}

	if a.HasCollectorFields() {
		if err := promptCollectorCommon(p, a); err != nil {
			return nil, err
		}
	}

	if a.Mode == ModeCollector {
		if err := promptCollectorOnly(p, a, out); err != nil {
			return nil, err
		}
	}

	if a.RunsLocalServers() {
		if err := promptServers(p, a, out); err != nil {
			return nil, err
		}
	}

	return a, nil
}

func promptCommon(p Prompter, a *Answers) error {
	var err error
	if a.ServiceUser, err = p.Line("Service user", a.ServiceUser); err != nil {
		return err
	}
	// ListenAddr and HTTPPort are not prompted: collectors auto-install
	// nginx (which fronts every public path), and even hub mode is
	// expected to sit behind a reverse proxy. Operators who really
	// want the Go binary directly on the wire can edit
	// /etc/trinity/config.yml. Defaults from RunWizard
	// (127.0.0.1:8080) are kept as-is.
	return nil
}

func promptHub(p Prompter, a *Answers) error {
	var err error
	if a.DatabasePath, err = p.Line("Database path", a.DatabasePath); err != nil {
		return err
	}
	if a.StaticDir, err = p.Line("Web assets path", a.StaticDir); err != nil {
		return err
	}
	return nil
}

func promptCollectorCommon(p Prompter, a *Answers) error {
	var err error
	if a.InstallEngine, err = p.YesNo("Install latest trinity-engine release (q3 server binary + mod)?", true); err != nil {
		return err
	}
	if a.Quake3Dir, err = p.Line("Quake3 install dir", a.Quake3Dir); err != nil {
		return err
	}
	return nil
}

// ErrMissingPrereqs is returned by RunWizard when the operator says
// they don't yet have the credentials a collector install needs.
// cmdInit treats it as a clean exit (informational, non-zero status)
// so the user sees the hint rather than a stack of "wizard failed"
// noise.
var ErrMissingPrereqs = fmt.Errorf("collector prerequisites not in hand")

// confirmCollectorPrereqs explains what the operator needs from the
// hub admin and bails out (with ErrMissingPrereqs) if they don't have
// it yet. Lives at the very top of the collector wizard so nobody
// answers ten questions only to discover they're missing a credential.
func confirmCollectorPrereqs(p Prompter, out io.Writer) error {
	fmt.Fprintln(out, "This installer joins your Quake 3 host to a Trinity hub as a collector.")
	fmt.Fprintln(out, "You will need:")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "  1. The hub hostname           (e.g. trinity.run)")
	fmt.Fprintln(out, "  2. A source ID                (approved name from \"My Servers\" on the hub)")
	fmt.Fprintln(out, "  3. A .creds file              (download from \"My Servers\" after approval)")
	fmt.Fprintln(out, "  4. A public hostname          (must already resolve to this box in DNS)")
	fmt.Fprintln(out, "  5. An email address           (for your Let's Encrypt SSL cert)")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "The wizard will install nginx, obtain a Let's Encrypt cert for your hostname,")
	fmt.Fprintln(out, "and open the firewall ports it needs (80/443/27970/tcp + 27960-28000/udp on")
	fmt.Fprintln(out, "ufw or firewalld). DNS for the hostname must resolve to this box BEFORE you")
	fmt.Fprintln(out, "run, or the cert fetch will time out and the wizard will fail mid-apply.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "If your hub has a public web UI, log in there and click \"Add Servers\" — an admin")
	fmt.Fprintln(out, "will approve your request and the drawer that appears gives you the source ID")
	fmt.Fprintln(out, "and a .creds download. Otherwise (or if your hub admin prefers it), ask them")
	fmt.Fprintln(out, "directly — for trinity.run, the Trinity Discord works.")
	fmt.Fprintln(out)
	ok, err := p.YesNo("Do you have these in hand now?", false)
	if err != nil {
		return err
	}
	if !ok {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Re-run the install once you have the source ID, .creds file, and a public")
		fmt.Fprintln(out, "hostname pointed at this box.")
		return ErrMissingPrereqs
	}
	fmt.Fprintln(out)
	return nil
}

func promptCollectorOnly(p Prompter, a *Answers, out io.Writer) error {
	var err error
	if a.HubHost, err = p.Line("Hub hostname (e.g. trinity.run)", "trinity.run"); err != nil {
		return err
	}
	if a.PublicURL, err = promptValidated(p, out,
		"Public HTTPS URL where this host is reachable from the Internet (e.g. https://q3.example.com)",
		"", validatePublicURL); err != nil {
		return err
	}
	if err := confirmDNSPointsHere(p, out, a.PublicURL); err != nil {
		return err
	}
	if a.AdminEmail, err = promptValidated(p, out,
		"Email for Let's Encrypt renewal alerts",
		"", validateAdminEmail); err != nil {
		return err
	}
	if a.SourceID, err = promptValidated(p, out,
		"Source ID (from \"My Servers\" on the hub)",
		"", validateSourceID); err != nil {
		return err
	}
	if a.CredsFile, err = promptValidated(p, out,
		"Path to .creds file from hub admin",
		"", validateCredsFile); err != nil {
		return err
	}
	return nil
}

// promptValidated wraps p.Line with a validation loop. Validation
// errors print to out and re-prompt rather than failing the wizard,
// so an operator can fix a typo without restarting.
func promptValidated(p Prompter, out io.Writer, prompt, def string, validate func(string) error) (string, error) {
	for {
		v, err := p.Line(prompt, def)
		if err != nil {
			return "", err
		}
		if vErr := validate(v); vErr != nil {
			fmt.Fprintf(out, "  %s\n", vErr)
			continue
		}
		return v, nil
	}
}

// validateSourceID enforces the same shape the hub's
// owner-source-request flow accepts (`api/owner_sources.go`):
// 3-32 chars, alnum + '_' + '-'. Catching this in the wizard
// turns an opaque NATS auth failure later into a clear "fix this
// answer now" prompt.
func validateSourceID(s string) error {
	if len(s) < 3 || len(s) > 32 {
		return fmt.Errorf("source ID must be 3-32 characters")
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-':
		default:
			return fmt.Errorf("source ID may only contain letters, digits, '_' or '-'")
		}
	}
	return nil
}

// validatePublicURL mirrors Answers.Validate's URL check (answers.go)
// but runs at prompt time so a typo doesn't blow up mid-install.
func validatePublicURL(s string) error {
	u, err := url.Parse(s)
	if err != nil {
		return fmt.Errorf("not a valid URL: %v", err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("URL must start with https:// (or http://)")
	}
	if u.Hostname() == "" {
		return fmt.Errorf("URL must include a hostname")
	}
	return nil
}

// confirmDNSPointsHere does an active DNS lookup on the hostname from
// publicURL and surfaces the result before the wizard sinks the
// operator into a multi-minute apply phase whose final step (certbot
// HTTP-01) cannot succeed unless the hostname already resolves to
// this box's public IP. Three outcomes:
//
//   - DNS resolves and at least one of the returned IPs matches our
//     outbound IP: silent pass.
//   - DNS resolves but none of the returned IPs match (or we can't
//     determine our outbound IP): print resolved IPs + ask the
//     operator to confirm one of them is this box. Defaults to yes —
//     they may be behind NAT/CDN and know better than we do.
//   - DNS does not resolve at all: print the bad-news message and
//     default to no — DNS-not-set-up is almost always a real mistake
//     the operator should fix before proceeding, not power through.
func confirmDNSPointsHere(p Prompter, out io.Writer, publicURL string) error {
	host := HostFromURL(publicURL)
	if host == "" {
		return nil
	}
	ips, err := net.LookupHost(host)
	if err != nil || len(ips) == 0 {
		fmt.Fprintf(out, "\n  WARNING: DNS for %q does not resolve.\n", host)
		fmt.Fprintln(out, "  Let's Encrypt validates over HTTP-01 to that hostname; without DNS")
		fmt.Fprintln(out, "  pointing at this box's public IP, the cert fetch will time out and")
		fmt.Fprintln(out, "  the wizard will fail mid-apply.")
		ok, perr := p.YesNo("Continue anyway?", false)
		if perr != nil {
			return perr
		}
		if !ok {
			return ErrMissingPrereqs
		}
		return nil
	}
	mine := publicIP()
	if mine != "" {
		for _, ip := range ips {
			if ip == mine {
				return nil
			}
		}
	}
	fmt.Fprintf(out, "\n  %q resolves to: %s\n", host, strings.Join(ips, ", "))
	if mine != "" {
		fmt.Fprintf(out, "  This box's public IP is:  %s\n", mine)
		fmt.Fprintln(out, "  DNS doesn't point at this box. Let's Encrypt HTTP-01 validation will fail.")
		ok, perr := p.YesNo("Continue anyway?", false)
		if perr != nil {
			return perr
		}
		if !ok {
			return ErrMissingPrereqs
		}
		return nil
	}
	fmt.Fprintln(out, "  Couldn't determine this box's public IP. Make sure that address points")
	fmt.Fprintln(out, "  at it, or Let's Encrypt HTTP-01 validation will fail.")
	ok, perr := p.YesNo("Continue?", true)
	if perr != nil {
		return perr
	}
	if !ok {
		return ErrMissingPrereqs
	}
	return nil
}

// publicIP asks an echo service for our Internet-facing IP. Cloud VMs
// behind 1:1 NAT see only a private local address, so the local
// interface IP wouldn't match what DNS resolves to. Returns "" if both
// services fail.
func publicIP() string {
	services := []string{
		"https://api.ipify.org",
		"https://icanhazip.com",
	}
	client := &http.Client{Timeout: 2 * time.Second}
	for _, svc := range services {
		resp, err := client.Get(svc)
		if err != nil {
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64))
		resp.Body.Close()
		ip := strings.TrimSpace(string(body))
		if net.ParseIP(ip) != nil {
			return ip
		}
	}
	return ""
}

// validateAdminEmail rejects obviously-bad addresses so the wizard
// catches typos before certbot does (its rejection comes after the
// `apt install` and stage-1 nginx config write — much later in the
// flow). Liberal: anything with a single '@' and a dot in the domain
// passes; we don't try to do RFC-5321 here.
func validateAdminEmail(s string) error {
	at := strings.IndexByte(s, '@')
	if at < 1 || at == len(s)-1 || strings.Count(s, "@") != 1 {
		return fmt.Errorf("not a valid email address: %s", s)
	}
	if !strings.Contains(s[at+1:], ".") {
		return fmt.Errorf("email domain needs a '.' (e.g. user@example.com)")
	}
	return nil
}

// validateCredsFile checks that the path exists and is a readable
// regular file. Apply will copy it to /etc/trinity/source.creds later;
// catching a typo here means we don't leave a half-applied install on
// the operator's box.
func validateCredsFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("file does not exist: %s", path)
		}
		return fmt.Errorf("cannot stat %s: %v", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory, not a .creds file", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("cannot read %s: %v", path, err)
	}
	f.Close()
	return nil
}

func promptServers(p Prompter, a *Answers, out io.Writer) error {
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Configure q3 servers running on this host. Add as many as you like.")
	add, err := p.YesNo("Add a q3 server now?", true)
	if err != nil {
		return err
	}
	for add {
		s, err := PromptServer(p, a, len(a.Servers))
		if err != nil {
			return err
		}
		a.Servers = append(a.Servers, s)
		add, err = p.YesNo("Add another?", false)
		if err != nil {
			return err
		}
	}
	return nil
}

// PromptServer collects a single ServerAnswers. Exposed so
// `trinity server add` can reuse it.
func PromptServer(p Prompter, a *Answers, index int) (ServerAnswers, error) {
	defaultPort := 27960 + index

	var s ServerAnswers
	var err error

	// Gametype first so the key default can match it.
	gtIdx, err := p.Choose("  Gametype:", gametypeLabels(), 0)
	if err != nil {
		return s, err
	}
	choice := GametypeChoices[gtIdx]
	s.Gametype = choice.Gametype
	s.UseMissionpack = choice.UseMissionpack

	defaultKey := suggestKey(s.Gametype, s.UseMissionpack, a.Servers)
	if s.Key, err = p.Line("  Server key (lowercase/digits/underscore/hyphen)", defaultKey); err != nil {
		return s, err
	}
	s.Key = strings.ToLower(s.Key)

	defaultAddr := fmt.Sprintf("127.0.0.1:%d", defaultPort)
	if a.PublicURL != "" {
		host := HostFromURL(a.PublicURL)
		if host != "" {
			defaultAddr = fmt.Sprintf("%s:%d", host, defaultPort)
		}
	}
	if s.Address, err = p.Line("  Public address (host:port)", defaultAddr); err != nil {
		return s, err
	}
	s.Port = portFromAddress(s.Address)
	if s.Port == 0 {
		s.Port = defaultPort
	}
	if s.RconPassword, err = p.Password("  RCON password", true); err != nil {
		return s, err
	}
	if s.RconPassword == "" {
		s.RconPassword = GenerateRCONPassword()
	}
	defaultLog := fmt.Sprintf("/var/log/quake3/%s.log", s.Key)
	if s.LogPath, err = p.Line("  Log path", defaultLog); err != nil {
		return s, err
	}
	return s, nil
}

// suggestKey returns a default key matching the chosen gametype, with
// a -2/-3/... suffix if a same-stem key is already in use.
func suggestKey(g Gametype, useMP bool, existing []ServerAnswers) string {
	base := Stem(g, useMP)
	if base == "" {
		base = "server"
	}
	used := make(map[string]bool, len(existing))
	for _, s := range existing {
		used[strings.ToLower(s.Key)] = true
	}
	if !used[base] {
		return base
	}
	for i := 2; i < 100; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !used[candidate] {
			return candidate
		}
	}
	return base
}

func gametypeLabels() []string {
	out := make([]string, len(GametypeChoices))
	for i, c := range GametypeChoices {
		out[i] = c.Label()
	}
	return out
}

// portFromAddress parses the trailing :PORT off a host:port address.
// Returns 0 on parse failure (caller falls back to a default).
func portFromAddress(addr string) int {
	i := strings.LastIndex(addr, ":")
	if i < 0 {
		return 0
	}
	var p int
	if _, err := fmt.Sscanf(addr[i+1:], "%d", &p); err != nil {
		return 0
	}
	return p
}

// GenerateRCONPassword returns a base64-encoded 18-byte random string
// (24 chars, no padding). Long enough to brute-force-resist; short
// enough to type into an RCON-capable client if you ever need to.
func GenerateRCONPassword() string {
	var b [18]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failures on Linux are essentially impossible.
		// Falling back to a constant would be worse than panicking
		// — silently weak passwords are how you get owned.
		panic("setup: failed to read crypto/rand: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}

// FormatPath strips a redundant trailing slash for display.
func FormatPath(p string) string {
	if p == "" {
		return p
	}
	cleaned := filepath.Clean(p)
	return cleaned
}
