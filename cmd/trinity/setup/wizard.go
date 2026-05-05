package setup

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// WizardOptions controls non-prompt-collected behavior of RunWizard:
// whether hub modes are unlocked, and which expert-mode skips are
// active (which can suppress prompts that would otherwise be unused).
type WizardOptions struct {
	AllowHub      bool
	SkipCert      bool
	SkipFirewall  bool
	SkipNginx     bool
	SkipLogrotate bool
}

// RunWizard walks the operator through every prompt the install
// needs and returns a populated Answers. mode-specific prompts
// branch off the leading mode question; per-server prompts repeat
// until the operator answers "no" to "add another?".
//
// opts.AllowHub controls whether the operator may pick a hub-bearing
// mode. The default front door (`scripts/install.sh`, `trinity init`)
// runs collector-only — Trinity is a network of collectors joining a
// shared hub at trinity.run, and extra hubs are easy to stand up by
// accident and hard to consolidate later. Hub installs are an expert
// path: `trinity init --allow-hub`, documented in
// distributed-deployment.md.
//
// The Skip* fields are copied onto the returned Answers so Apply can
// act on them; they also gate prompts the wizard would otherwise ask
// (e.g. AdminEmail is unused when SkipCert or SkipNginx is set).
func RunWizard(p Prompter, out io.Writer, opts WizardOptions) (*Answers, error) {
	fmt.Fprintln(out, "Welcome to Trinity setup.")
	fmt.Fprintln(out)

	a := &Answers{
		ServiceUser:   "quake",
		ListenAddr:    "127.0.0.1",
		HTTPPort:      8080,
		DatabasePath:  "/var/lib/trinity/trinity.db",
		StaticDir:     "/var/lib/trinity/web",
		Quake3Dir:     "/usr/lib/quake3",
		SkipCert:      opts.SkipCert,
		SkipFirewall:  opts.SkipFirewall,
		SkipNginx:     opts.SkipNginx,
		SkipLogrotate: opts.SkipLogrotate,
	}

	if !opts.AllowHub {
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
		if err := promptHubPublic(p, a, out); err != nil {
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
	// JWT secret isn't operator-tunable — silently mint one. Pre-set
	// values (e.g. tests) pass through untouched.
	if a.JWTSecret == "" {
		a.JWTSecret = GenerateJWTSecret()
	}
	return nil
}

// promptHubPublic gathers the public-facing knobs hub modes need:
// the SPA's public URL (also the LE cert hostname), the admin email
// for renewal alerts, and — for Combined mode — the local source ID
// the in-process collector publishes events under. Asks once whether
// remote collectors will connect, which decides whether the embedded
// NATS broker binds externally with TLS or only on loopback.
func promptHubPublic(p Prompter, a *Answers, out io.Writer) error {
	hostInput, err := promptValidated(p, out,
		"Public hostname for this hub, e.g. trinity.example.org (must already resolve here)",
		"", validatePublicHostname)
	if err != nil {
		return err
	}
	host, _ := normalizePublicHostname(hostInput)
	a.PublicURL = "https://" + host
	if err := confirmDNSPointsHere(p, out, a.PublicURL); err != nil {
		return err
	}
	// AdminEmail is for Let's Encrypt renewal alerts only — skip the
	// prompt when the operator has opted out of either the certbot run
	// or nginx altogether.
	if !a.SkipCert && !a.SkipNginx {
		if a.AdminEmail, err = promptValidated(p, out,
			"Email for Let's Encrypt renewal alerts",
			"", validateAdminEmail); err != nil {
			return err
		}
	}
	if a.Mode == ModeCombined {
		if a.SourceID, err = promptValidated(p, out,
			"Local source ID (the local collector publishes events under this name)",
			"hub", validateSourceID); err != nil {
			return err
		}
	}
	if a.RemoteCollectorsExpected, err = p.YesNo(
		"Expect remote collectors to connect to this hub?", false); err != nil {
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
	fmt.Fprintln(out, "  4. A public hostname          (must already resolve to this box in DNS,")
	fmt.Fprintln(out, "                                 plus a dl.<hostname> A/AAAA record on the")
	fmt.Fprintln(out, "                                 same host for the Quake 3 fastdl vhost)")
	fmt.Fprintln(out, "  5. An email address           (for your Let's Encrypt SSL cert)")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Optional (saves a path-prompt during pak install): copy your retail")
	fmt.Fprintln(out, "Quake 3 pak0.pk3 into the current directory as q3-pak0.pk3, and")
	fmt.Fprintln(out, "mp-pak0.pk3 if you'll run Team Arena gametypes. The wizard will")
	fmt.Fprintln(out, "find them by name and place them without prompting.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "The wizard will install nginx, obtain a Let's Encrypt SAN cert covering both")
	fmt.Fprintln(out, "<hostname> and dl.<hostname>, and open the firewall ports it needs (80/443/tcp")
	fmt.Fprintln(out, "+ 27960-28000/udp on ufw or firewalld). Both DNS records must resolve to this")
	fmt.Fprintln(out, "box BEFORE you run, or the cert fetch will time out and the wizard will fail")
	fmt.Fprintln(out, "mid-apply.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "If your hub has a public web UI, log in there and click \"Add Servers\" — an admin")
	fmt.Fprintln(out, "will approve your request and the drawer that appears gives you the source ID")
	fmt.Fprintln(out, "and a .creds download. Otherwise (or if your hub admin prefers it), ask them")
	fmt.Fprintln(out, "directly — for trinity.run, the Trinity Discord works")
	fmt.Fprintln(out, "(https://discord.gg/uJ5EPAXKfs).")
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
	hostInput, err := promptValidated(p, out,
		"Public hostname for this host, e.g. q3.example.com (must already resolve here)",
		"", validatePublicHostname)
	if err != nil {
		return err
	}
	// validatePublicHostname has already accepted; normalize can't error.
	host, _ := normalizePublicHostname(hostInput)
	a.PublicURL = "https://" + host
	if err := confirmDNSPointsHere(p, out, a.PublicURL); err != nil {
		return err
	}
	if !a.SkipCert && !a.SkipNginx {
		if a.AdminEmail, err = promptValidated(p, out,
			"Email for Let's Encrypt renewal alerts",
			"", validateAdminEmail); err != nil {
			return err
		}
	}
	filled, err := tryAutoFillCreds(p, out, a)
	if err != nil {
		return err
	}
	if !filled {
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
	}
	return nil
}

// credsCandidate is one usable .creds file discovered in the wizard's
// CWD. The hub UI saves the file as "<source-id>.creds", so the stem
// gives us the source ID for free — provided the operator didn't
// hand-rename it to something the hub won't accept.
type credsCandidate struct {
	path     string
	sourceID string
}

// credsRejection is a *.creds file we found but can't auto-fill from —
// either unreadable, or its stem isn't a valid source ID. Surfaced
// to the operator so they're not left wondering why a file they
// can see in `ls` was ignored.
type credsRejection struct {
	path   string
	reason string
}

// discoverCredsIn globs *.creds in dir and partitions them into
// usable candidates (stem passes validateSourceID, file is readable)
// and rejections (everything else, with a human-readable reason).
// The wizard prints rejections so an operator with a hand-renamed
// file or a stale candidate sees why it was skipped.
func discoverCredsIn(dir string) (valid []credsCandidate, rejected []credsRejection) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.creds"))
	if err != nil {
		return nil, nil
	}
	for _, m := range matches {
		if err := validateCredsFile(m); err != nil {
			rejected = append(rejected, credsRejection{path: m, reason: err.Error()})
			continue
		}
		stem := strings.TrimSuffix(filepath.Base(m), ".creds")
		if err := validateSourceID(stem); err != nil {
			rejected = append(rejected, credsRejection{
				path:   m,
				reason: fmt.Sprintf("stem %q is not a valid source ID (%v)", stem, err),
			})
			continue
		}
		valid = append(valid, credsCandidate{path: m, sourceID: stem})
	}
	return valid, rejected
}

// tryAutoFillCreds looks for a usable .creds file in the operator's
// CWD and offers to skip the SourceID/CredsFile prompts. Returns true
// when a.SourceID and a.CredsFile have been populated. The hub-issued
// .creds filename is the source name, so a single match lets the
// wizard collapse two prompts into one Y/n confirmation.
func tryAutoFillCreds(p Prompter, out io.Writer, a *Answers) (bool, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return false, nil
	}
	candidates, rejected := discoverCredsIn(cwd)
	for _, r := range rejected {
		fmt.Fprintf(out, "\nIgnoring %s: %s\n", filepath.Base(r.path), r.reason)
	}
	switch len(candidates) {
	case 0:
		return false, nil
	case 1:
		c := candidates[0]
		fmt.Fprintf(out, "\nFound %s in this directory.\n", filepath.Base(c.path))
		ok, err := p.YesNo(fmt.Sprintf("Use source ID %q with this file?", c.sourceID), true)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
		a.SourceID = c.sourceID
		a.CredsFile = c.path
		return true, nil
	default:
		// Multi-match: numbered choose menu, with a tail option that
		// drops back to the manual prompts so the operator isn't trapped.
		opts := make([]string, len(candidates)+1)
		for i, c := range candidates {
			opts[i] = fmt.Sprintf("%s  (source: %s)", filepath.Base(c.path), c.sourceID)
		}
		opts[len(candidates)] = "(none — enter manually)"
		idx, err := p.Choose("Multiple .creds files found in this directory:", opts, 0)
		if err != nil {
			return false, err
		}
		if idx == len(candidates) {
			return false, nil
		}
		a.SourceID = candidates[idx].sourceID
		a.CredsFile = candidates[idx].path
		return true, nil
	}
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

// validateSourceID mirrors storage.ValidateSource on shape (alnum +
// '_' + '-') but with the project-wide 16-char ceiling. Looser on the
// floor than the owner-request modal's 3-char minimum
// (`api/owner_sources.go`) — admin-provisioned sources bypass that
// modal and may be shorter, and the wizard has to accept anything the
// hub already issued a .creds for. Catching the shape here turns an
// opaque NATS auth failure later into a clear "fix this answer now"
// prompt.
func validateSourceID(s string) error {
	if len(s) < 1 || len(s) > 16 {
		return fmt.Errorf("source ID must be 1-16 characters")
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

// normalizePublicHostname accepts either a bare hostname
// ("q3.example.com") or a pasted URL ("https://q3.example.com/foo"),
// strips any scheme/path/port, and returns the hostname alone. Errors
// if the result isn't shaped like a fully-qualified hostname.
func normalizePublicHostname(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("hostname is required")
	}
	h := strings.TrimPrefix(strings.TrimPrefix(s, "https://"), "http://")
	if i := strings.Index(h, "/"); i >= 0 {
		h = h[:i]
	}
	if i := strings.Index(h, ":"); i >= 0 {
		h = h[:i]
	}
	if !strings.Contains(h, ".") {
		return "", fmt.Errorf("need a fully-qualified hostname with at least one dot (e.g. q3.example.com)")
	}
	for _, r := range h {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '.':
		default:
			return "", fmt.Errorf("invalid character %q in hostname", r)
		}
	}
	return h, nil
}

// validatePublicHostname is the prompt-time loop variant — re-runs
// normalize and returns just its error so promptValidated can re-ask.
func validatePublicHostname(s string) error {
	_, err := normalizePublicHostname(s)
	return err
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
	// Per-server opt-in for hub-admin RCON delegation. Defaults off
	// — operators must explicitly grant the keys. Stored alongside the
	// rcon password and re-published to the hub on every heartbeat;
	// flipping it later is a config edit + service restart away.
	if s.AllowHubAdminRcon, err = p.YesNo("  Allow hub admins to RCON this server in your absence?", false); err != nil {
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

// GenerateJWTSecret returns a base64-encoded 32-byte random string
// (43 chars). Used as the HMAC key for hub auth tokens.
func GenerateJWTSecret() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
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
