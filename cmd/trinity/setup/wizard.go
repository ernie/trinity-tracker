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
	"regexp"
	"strings"
	"time"
)

// discordWebhookRE matches Discord's webhook URL shape. It must match
// the regex in internal/config's validateDiscord, which is unexported.
var discordWebhookRE = regexp.MustCompile(`^https://discord\.com/api/webhooks/\d+/[A-Za-z0-9_-]+$`)

// WizardOptions sets what RunWizard doesn't prompt for: whether hub
// modes are unlocked, and the expert-mode skips.
type WizardOptions struct {
	AllowHub      bool
	SkipCert      bool
	SkipFirewall  bool
	SkipNginx     bool
	SkipLogrotate bool
}

// RunWizard walks the operator through every prompt the install
// needs and returns a populated Answers. Mode-specific prompts branch
// off the first question; server prompts repeat until the operator
// declines to add another.
//
// opts.AllowHub unlocks the hub modes (`trinity init --allow-hub`,
// documented in distributed-deployment.md). The default is
// collector-only: Trinity is a network of collectors joining the shared
// hub at trinity.run, and extra hubs are easy to start by accident and
// hard to merge later.
//
// The Skip* fields carry onto the returned Answers for Apply and skip
// the prompts they make unused (AdminEmail under SkipCert or SkipNginx).
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
		// Collector stays the default; a hub has to be chosen.
		modeIdx, err := p.Choose("What kind of install?", []string{
			ModeCombined.String() + "  (single machine)",
			ModeHubOnly.String() + "  (central UI; remote collectors report in)",
			ModeCollector.String() + "  (watches local game servers; reports to a remote hub)",
		}, int(ModeCollector))
		if err != nil {
			return nil, err
		}
		a.Mode = Mode(modeIdx)
	}

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
		if err := promptDiscord(p, a, out); err != nil {
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
	// ListenAddr and HTTPPort keep RunWizard's 127.0.0.1:8080 because
	// nginx or another reverse proxy fronts every public path.
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
	// Generated unless already set, as tests do.
	if a.JWTSecret == "" {
		a.JWTSecret = GenerateJWTSecret()
	}
	return nil
}

// promptHubPublic gathers what hub modes need to face the public: the
// SPA's public URL (also the certificate's hostname), the admin email
// for renewal alerts, and in combined mode the source ID the local
// collector publishes under. Whether remote collectors will connect
// decides if the embedded NATS broker binds externally with TLS or
// only on loopback.
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
	if a.InstallEngine, err = p.YesNo("Install the latest Trinity engine release (game server and mod)?", true); err != nil {
		return err
	}
	if a.Quake3Dir, err = p.Line("Quake III install directory", a.Quake3Dir); err != nil {
		return err
	}
	return nil
}

// ErrMissingPrereqs is returned by RunWizard when the operator doesn't
// yet have what a collector install needs. cmdInit exits non-zero on
// it without a failure message, leaving the hint as the last output.
var ErrMissingPrereqs = fmt.Errorf("collector prerequisites not in hand")

// confirmCollectorPrereqs lists what a collector install needs and
// returns ErrMissingPrereqs if the operator doesn't have it yet. It runs
// first, so nobody answers every question only to find something missing.
func confirmCollectorPrereqs(p Prompter, out io.Writer) error {
	fmt.Fprintln(out, "This installer connects your Quake III Arena host to a Trinity hub as a")
	fmt.Fprintln(out, "collector. You will need:")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "  1. The hub hostname           (such as trinity.run)")
	fmt.Fprintln(out, "  2. A source ID                (your approved name from \"My servers\" on the hub)")
	fmt.Fprintln(out, "  3. A .creds file              (download it from \"My servers\" after approval)")
	fmt.Fprintln(out, "  4. A public hostname          (must already resolve to this box in DNS, plus")
	fmt.Fprintln(out, "                                 a dl.<hostname> A or AAAA record for the same")
	fmt.Fprintln(out, "                                 box, which serves fast downloads)")
	fmt.Fprintln(out, "  5. An email address           (for your Let's Encrypt certificate)")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Optional: to skip a path prompt during the pak install, copy your retail")
	fmt.Fprintln(out, "Quake III Arena pak0.pk3 into the current directory as q3-pak0.pk3, and your")
	fmt.Fprintln(out, "Team Arena pak0.pk3 as mp-pak0.pk3 if you'll run Team Arena game types. The")
	fmt.Fprintln(out, "wizard finds them by name and puts them in place without asking.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "The wizard installs nginx, gets a Let's Encrypt certificate covering both")
	fmt.Fprintln(out, "<hostname> and dl.<hostname>, and opens the firewall ports it needs (80/443/tcp")
	fmt.Fprintln(out, "and 27960-28000/udp, with ufw or firewalld). Both DNS records must resolve to")
	fmt.Fprintln(out, "this box BEFORE you run it, or the certificate request times out and the")
	fmt.Fprintln(out, "wizard fails partway through.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "To get a source ID and .creds file, log in on the hub's website and click")
	fmt.Fprintln(out, "\"Add servers\" (or \"My servers\", then \"Add another source\"). Once an admin")
	fmt.Fprintln(out, "approves your request, your source in \"My servers\" shows its name and a")
	fmt.Fprintln(out, ".creds download. For trinity.run, ask for help in the Trinity Discord")
	fmt.Fprintln(out, "(https://discord.gg/uJ5EPAXKfs).")
	fmt.Fprintln(out)
	ok, err := p.YesNo("Do you have these now?", false)
	if err != nil {
		return err
	}
	if !ok {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Run the install again once you have the source ID, .creds file and a public")
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
			"Source ID (from \"My servers\" on the hub)",
			"", validateSourceID); err != nil {
			return err
		}
		if a.CredsFile, err = promptValidated(p, out,
			"Path to your .creds file",
			"", validateCredsFile); err != nil {
			return err
		}
	}
	return nil
}

// credsCandidate is one usable .creds file in the working directory.
// The site saves the file as "<source-id>.creds", so its stem is the
// source ID.
type credsCandidate struct {
	path     string
	sourceID string
}

// credsRejection is a *.creds file that's unreadable or whose stem
// isn't a valid source ID. The wizard prints the reason.
type credsRejection struct {
	path   string
	reason string
}

// discoverCredsIn splits the *.creds files in dir into usable
// candidates (readable, with a stem that passes validateSourceID) and
// rejections with a reason.
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

// tryAutoFillCreds looks for a usable .creds file in the working
// directory and offers it in place of the SourceID and CredsFile
// prompts. It returns true when it filled a.SourceID and a.CredsFile.
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
		// The last choice falls back to the manual prompts.
		opts := make([]string, len(candidates)+1)
		for i, c := range candidates {
			opts[i] = fmt.Sprintf("%s  (source: %s)", filepath.Base(c.path), c.sourceID)
		}
		opts[len(candidates)] = "(none: I'll type them in)"
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

// promptValidated wraps p.Line in a loop that prints each validation
// error and asks again, so a typo doesn't end the wizard.
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

// validateSourceID mirrors storage.ValidateSource's characters
// (letters, digits, _ and -) with the 16-character limit. It accepts
// names under the request form's 3-character minimum
// (`api/owner_sources.go`), since admin-created sources can be shorter
// and the wizard must accept any name the hub issued .creds for.
func validateSourceID(s string) error {
	if len(s) < 1 || len(s) > 16 {
		return fmt.Errorf("source ID must be 1–16 characters")
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-':
		default:
			return fmt.Errorf("source ID can use only letters, digits, _ and -")
		}
	}
	return nil
}

// normalizePublicHostname accepts either a bare hostname
// ("q3.example.com") or a pasted URL ("https://q3.example.com/foo"),
// and returns the hostname alone. It errors when the result isn't a
// fully qualified hostname.
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
		return "", fmt.Errorf("need a full hostname with at least one dot, such as q3.example.com")
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

// validatePublicHostname returns normalizePublicHostname's error for
// promptValidated.
func validatePublicHostname(s string) error {
	_, err := normalizePublicHostname(s)
	return err
}

// confirmDNSPointsHere looks up publicURL's hostname before the long
// apply phase, whose certbot step needs that name to resolve to this
// box's public IP:
//
//   - one of the resolved IPs is ours: it passes without asking
//   - none match, or our IP is unknown: it lists the IPs and asks,
//     defaulting to yes, since a box behind NAT or a CDN can be right
//   - the name doesn't resolve: it explains and asks, defaulting to no
func confirmDNSPointsHere(p Prompter, out io.Writer, publicURL string) error {
	host := HostFromURL(publicURL)
	if host == "" {
		return nil
	}
	ips, err := net.LookupHost(host)
	if err != nil || len(ips) == 0 {
		fmt.Fprintf(out, "\n  WARNING: DNS for %q does not resolve.\n", host)
		fmt.Fprintln(out, "  Let's Encrypt checks that hostname over HTTP. Unless DNS points at this")
		fmt.Fprintln(out, "  box's public IP, the certificate request times out and the wizard fails")
		fmt.Fprintln(out, "  partway through.")
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
		fmt.Fprintln(out, "  DNS doesn't point at this box, so the Let's Encrypt check will fail.")
		ok, perr := p.YesNo("Continue anyway?", false)
		if perr != nil {
			return perr
		}
		if !ok {
			return ErrMissingPrereqs
		}
		return nil
	}
	fmt.Fprintln(out, "  Couldn't find this box's public IP. Make sure one of those addresses is")
	fmt.Fprintln(out, "  this box, or the Let's Encrypt check will fail.")
	ok, perr := p.YesNo("Continue?", true)
	if perr != nil {
		return perr
	}
	if !ok {
		return ErrMissingPrereqs
	}
	return nil
}

// publicIP asks an echo service for our Internet-facing IP, since a
// cloud VM behind 1:1 NAT has only a private address locally. It
// returns "" if both services fail.
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

// promptDiscord asks whether to enable the weekly Discord leaderboard
// digest. Accepting collects a webhook URL and a systemd OnCalendar=
// schedule (default "Mon 00:00", host local time).
func promptDiscord(p Prompter, a *Answers, out io.Writer) error {
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Optional: post a weekly leaderboard digest to a Discord channel through a")
	fmt.Fprintln(out, "webhook. You can skip this and set it up later by editing")
	fmt.Fprintln(out, "/etc/trinity/config.yml and running `systemctl enable --now trinity-digest.timer`.")

	enable, err := p.YesNo("Enable weekly Discord digest?", false)
	if err != nil {
		return err
	}
	if !enable {
		return nil
	}
	url, err := promptValidated(p, out,
		"Discord webhook URL (https://discord.com/api/webhooks/{id}/{token})",
		"", validateDiscordWebhookURL)
	if err != nil {
		return err
	}
	sched, err := p.Line("Schedule (systemd OnCalendar= value)", "Mon 00:00")
	if err != nil {
		return err
	}
	a.DiscordEnabled = true
	a.DiscordWebhookURL = url
	a.DiscordSchedule = sched
	return nil
}

// validateDiscordWebhookURL checks the URL at the prompt with the regex
// config.Load applies later.
func validateDiscordWebhookURL(s string) error {
	if !discordWebhookRE.MatchString(s) {
		return fmt.Errorf("not a Discord webhook URL (expected https://discord.com/api/webhooks/{id}/{token})")
	}
	return nil
}

// validateAdminEmail catches typos at the prompt, well before certbot
// would reject the address. Any address with one '@' and a dot in the
// domain passes.
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

// validateCredsFile checks that the path is a readable regular file
// before Apply starts, so a typo can't leave a half-applied install.
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
	fmt.Fprintln(out, "Set up the game servers that run on this host. Add as many as you like.")
	add, err := p.YesNo("Add a game server now?", true)
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

// PromptServer collects a single ServerAnswers, for the wizard and
// `trinity server add`.
func PromptServer(p Prompter, a *Answers, index int) (ServerAnswers, error) {
	defaultPort := 27960 + index

	var s ServerAnswers
	var err error

	// Gametype first so the key default can match it.
	gtIdx, err := p.Choose("  Game type:", gametypeLabels(), 0)
	if err != nil {
		return s, err
	}
	choice := GametypeChoices[gtIdx]
	s.Gametype = choice.Gametype
	s.UseMissionpack = choice.UseMissionpack

	defaultKey := suggestKey(s.Gametype, s.UseMissionpack, a.Servers)
	if s.Key, err = p.Line("  Server key (lowercase letters, digits, _ and -)", defaultKey); err != nil {
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
	// Off unless granted. The collector sends it to the hub on every
	// heartbeat, so changing it later is a config edit and a restart.
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
// (24 chars, no padding): long enough to resist brute force, short
// enough to type into an RCON client.
func GenerateRCONPassword() string {
	var b [18]byte
	if _, err := rand.Read(b[:]); err != nil {
		// A weak password is worse than a crash.
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
