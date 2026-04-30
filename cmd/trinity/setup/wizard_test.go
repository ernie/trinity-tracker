package setup

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// scriptedPrompter is a deterministic Prompter for tests. It pops
// answers off a slice; "" means "use the default". Test failure on
// unknown call types catches drift between the wizard's prompts and
// the test's expectations.
type scriptedPrompter struct {
	answers []string
	t       *testing.T
}

func (s *scriptedPrompter) pop() string {
	s.t.Helper()
	if len(s.answers) == 0 {
		s.t.Fatalf("scriptedPrompter ran out of answers")
	}
	a := s.answers[0]
	s.answers = s.answers[1:]
	return a
}

func (s *scriptedPrompter) Line(prompt, def string) (string, error) {
	a := s.pop()
	if a == "" {
		if def == "" {
			s.t.Fatalf("Line(%q): empty answer with no default", prompt)
		}
		return def, nil
	}
	return a, nil
}

func (s *scriptedPrompter) Optional(prompt, def string) (string, error) {
	a := s.pop()
	if a == "" {
		return def, nil
	}
	return a, nil
}

func (s *scriptedPrompter) Int(prompt string, def, min, max int) (int, error) {
	a := s.pop()
	if a == "" {
		return def, nil
	}
	var n int
	if _, err := fmt.Sscanf(a, "%d", &n); err != nil {
		s.t.Fatalf("Int(%q): bad answer %q", prompt, a)
	}
	return n, nil
}

func (s *scriptedPrompter) YesNo(prompt string, def bool) (bool, error) {
	a := strings.ToLower(s.pop())
	switch a {
	case "":
		return def, nil
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	}
	s.t.Fatalf("YesNo(%q): bad answer %q", prompt, a)
	return false, nil
}

func (s *scriptedPrompter) Choose(prompt string, options []string, def int) (int, error) {
	a := s.pop()
	if a == "" {
		return def, nil
	}
	var n int
	if _, err := fmt.Sscanf(a, "%d", &n); err != nil {
		s.t.Fatalf("Choose(%q): bad answer %q", prompt, a)
	}
	return n - 1, nil
}

func (s *scriptedPrompter) Password(prompt string, allowEmpty bool) (string, error) {
	a := s.pop()
	return a, nil
}

func (s *scriptedPrompter) Pause(prompt string) error {
	// Pause is acknowledgment-only — discard whatever's at the top of
	// the answer queue so the test scripts can be explicit about the
	// pause without caring what it returns.
	s.pop()
	return nil
}

func TestRunWizard_CombinedDefaults(t *testing.T) {
	// allowHub=true so the mode prompt is exposed. Pick option 1 (combined)
	// since the new default is collector.
	p := &scriptedPrompter{
		t: t,
		answers: []string{
			"1",                 // mode → ModeCombined (override the new collector default)
			"",                  // service user → quake
			"",                  // database path → default
			"",                  // static dir → default
			"hub.example.com", // public hostname (we prepend https://)
			"y",                 // continue past DNS warning (host does not resolve)
			"ops@example.com",   // admin email
			"",                  // local source id → "hub" (default)
			"y",                 // expect remote collectors
			"y",                 // install engine
			"",                  // quake3 dir → default
			"y",                 // add a server now
			"",                  // gametype → FFA (default)
			"",                  // server key → ffa (default for FFA)
			"",                  // address → default 127.0.0.1:27960
			"",                  // rcon password → generate
			"",                  // log path → default
			"n",                 // add another? → no
		},
	}
	var buf bytes.Buffer
	a, err := RunWizard(p, &buf, WizardOptions{AllowHub: true})
	if err != nil {
		t.Fatalf("RunWizard: %v", err)
	}
	if a.Mode != ModeCombined {
		t.Errorf("mode: got %v", a.Mode)
	}
	if a.ServiceUser != "quake" {
		t.Errorf("service user: %q", a.ServiceUser)
	}
	if a.PublicURL != "https://hub.example.com" {
		t.Errorf("public url: %q", a.PublicURL)
	}
	if a.AdminEmail != "ops@example.com" {
		t.Errorf("admin email: %q", a.AdminEmail)
	}
	if a.SourceID != "hub" {
		t.Errorf("source id should default to %q, got %q", "hub", a.SourceID)
	}
	if !a.RemoteCollectorsExpected {
		t.Errorf("RemoteCollectorsExpected should be true")
	}
	if !a.InstallEngine {
		t.Errorf("expected install engine")
	}
	if len(a.Servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(a.Servers))
	}
	s := a.Servers[0]
	if s.Key != "ffa" {
		t.Errorf("server key: %q", s.Key)
	}
	if s.Gametype != GametypeFFA {
		t.Errorf("gametype: %v", s.Gametype)
	}
	if s.Port != 27960 {
		t.Errorf("port: %d", s.Port)
	}
	if s.RconPassword == "" {
		t.Errorf("expected generated rcon password")
	}
	if err := a.Validate(); err != nil {
		t.Errorf("answers should validate: %v", err)
	}
}

func TestSuggestKey(t *testing.T) {
	cases := []struct {
		name     string
		g        Gametype
		useMP    bool
		existing []ServerAnswers
		want     string
	}{
		{"FFA fresh", GametypeFFA, false, nil, "ffa"},
		{"TDM fresh", GametypeTDM, false, nil, "tdm"},
		{"TDM-TA fresh", GametypeTDM, true, nil, "tdm-ta"},
		{"OneFlag uses 1fctf stem", GametypeOneFlag, true, nil, "1fctf"},
		{"second FFA gets -2", GametypeFFA, false, []ServerAnswers{{Key: "ffa"}}, "ffa-2"},
		{"third FFA gets -3", GametypeFFA, false, []ServerAnswers{{Key: "ffa"}, {Key: "ffa-2"}}, "ffa-3"},
		{"case-insensitive collision", GametypeFFA, false, []ServerAnswers{{Key: "FFA"}}, "ffa-2"},
		{"TA variant doesn't collide with baseq3", GametypeTDM, true, []ServerAnswers{{Key: "tdm"}}, "tdm-ta"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := suggestKey(tc.g, tc.useMP, tc.existing); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRunWizard_CollectorOnly(t *testing.T) {
	// Default front door: allowHub=false, mode is locked to collector
	// with no prompt — answers list does NOT include a mode response.
	credsPath := writeFakeCreds(t)
	p := &scriptedPrompter{
		t: t,
		answers: []string{
			"y",                      // creds prereqs in hand
			"",                       // service user → quake
			"",                       // install engine? → yes (default)
			"",                       // quake3 dir → default
			"trinity.example.com", // hub host
			"q3.example.com",      // public hostname (we prepend https://)
			"y",                   // continue past DNS warning (q3.example.com does not resolve)
			"ops@example.com",        // admin email
			"mygame",                 // source ID
			credsPath,                // creds file (exists)
			"y",                      // add a server now
			"",                       // gametype → FFA (default)
			"ffa",                    // server key
			"",                       // address → default q3.example.com:27960
			"hunter2",                // rcon password (provided, not generated)
			"",                       // log path → default
			"n",                      // no more servers
		},
	}
	var buf bytes.Buffer
	a, err := RunWizard(p, &buf, WizardOptions{})
	if err != nil {
		t.Fatalf("RunWizard: %v", err)
	}
	if a.Mode != ModeCollector {
		t.Fatalf("mode: %v", a.Mode)
	}
	if a.HubHost != "trinity.example.com" {
		t.Errorf("hub host: %q", a.HubHost)
	}
	if a.PublicURL != "https://q3.example.com" {
		t.Errorf("public url: %q", a.PublicURL)
	}
	if a.SourceID != "mygame" {
		t.Errorf("source id: %q", a.SourceID)
	}
	if len(a.Servers) != 1 || a.Servers[0].Address != "q3.example.com:27960" {
		t.Errorf("server address (should pull host from public URL): %+v", a.Servers)
	}
	if err := a.Validate(); err != nil {
		t.Errorf("answers should validate: %v", err)
	}
}

// writeFakeCreds drops a placeholder .creds file in t.TempDir() so
// the wizard's CredsFile validator (which checks readability) is
// satisfied. Tests that exercise actual creds parsing should use
// real fixtures instead.
func writeFakeCreds(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.creds")
	if err := os.WriteFile(path, []byte("placeholder\n"), 0o600); err != nil {
		t.Fatalf("writeFakeCreds: %v", err)
	}
	return path
}

// TestRunWizard_NoCredsExits verifies that an operator who answers
// "no" to the up-front prereq gate gets ErrMissingPrereqs and is not
// asked any further questions. This protects the user-experience
// contract: don't make people answer ten questions before telling
// them they're missing a credential.
func TestRunWizard_NoCredsExits(t *testing.T) {
	p := &scriptedPrompter{
		t: t,
		answers: []string{
			"n", // creds prereqs NOT in hand
		},
	}
	var buf bytes.Buffer
	a, err := RunWizard(p, &buf, WizardOptions{})
	if !errors.Is(err, ErrMissingPrereqs) {
		t.Fatalf("expected ErrMissingPrereqs, got %v", err)
	}
	if a != nil {
		t.Errorf("expected nil answers when bailing out, got %+v", a)
	}
	if len(p.answers) != 0 {
		t.Errorf("wizard kept asking after the gate failed; remaining answers: %q", p.answers)
	}
	if !strings.Contains(buf.String(), ".creds file") {
		t.Errorf("expected guidance about the .creds file in output; got %q", buf.String())
	}
}

// TestRunWizard_LockedCollector_NoModePrompt is the regression guard
// for the default front door: with allowHub=false the wizard never
// asks about mode and never lets the operator end up in a hub
// install. If this test ever needs a mode answer, the lock is broken.
func TestRunWizard_LockedCollector_NoModePrompt(t *testing.T) {
	credsPath := writeFakeCreds(t)
	p := &scriptedPrompter{
		t: t,
		answers: []string{
			"y",                   // creds prereqs in hand
			"",                    // service user
			"",                    // install engine
			"",                    // quake3 dir
			"trinity.run",         // hub host
			"https://example.com", // public URL
			"y",                   // confirm resolved IP is this box
			"ops@example.com",     // admin email
			"src1",                // source ID
			credsPath,             // creds file (exists)
			"n",                   // no servers
		},
	}
	var buf bytes.Buffer
	a, err := RunWizard(p, &buf, WizardOptions{})
	if err != nil {
		t.Fatalf("RunWizard: %v", err)
	}
	if a.Mode != ModeCollector {
		t.Fatalf("locked wizard must yield ModeCollector, got %v", a.Mode)
	}
	if len(p.answers) != 0 {
		t.Errorf("scripted prompter has %d unused answers — wizard asked fewer questions than expected:\n  remaining: %q", len(p.answers), p.answers)
	}
}

// TestRunWizard_CredsAutoDiscovery checks that when the operator runs
// the wizard from a directory containing a single *.creds file, both
// SourceID and CredsFile are filled from the filename — collapsing two
// prompts into one Y/n confirmation. Lifted into its own test (rather
// than folding it into the main collector test) so the existing
// manual-path coverage stays intact.
func TestRunWizard_CredsAutoDiscovery(t *testing.T) {
	dir := t.TempDir()
	credsPath := filepath.Join(dir, "myserver.creds")
	if err := os.WriteFile(credsPath, []byte("placeholder\n"), 0o600); err != nil {
		t.Fatalf("write creds: %v", err)
	}
	t.Chdir(dir)
	p := &scriptedPrompter{
		t: t,
		answers: []string{
			"y",                   // creds prereqs in hand
			"",                    // service user → quake
			"",                    // install engine → yes
			"",                    // quake3 dir → default
			"trinity.example.com", // hub host
			"q3.example.com",      // public hostname
			"y",                   // continue past DNS warning
			"ops@example.com",     // admin email
			"y",                   // YES, use the auto-discovered creds
			"n",                   // no servers
		},
	}
	var buf bytes.Buffer
	a, err := RunWizard(p, &buf, WizardOptions{})
	if err != nil {
		t.Fatalf("RunWizard: %v", err)
	}
	if a.SourceID != "myserver" {
		t.Errorf("source ID: got %q, want %q", a.SourceID, "myserver")
	}
	if a.CredsFile != credsPath {
		t.Errorf("creds file: got %q, want %q", a.CredsFile, credsPath)
	}
	if !strings.Contains(buf.String(), "myserver.creds") {
		t.Errorf("expected the discovery line to mention myserver.creds; got %q", buf.String())
	}
}

func TestRunWizard_HubOnly_NoServerPrompts(t *testing.T) {
	// Hub mode requires allowHub=true.
	p := &scriptedPrompter{
		t: t,
		answers: []string{
			"2",                 // mode → ModeHubOnly
			"",                  // service user → quake
			"",                  // database path → default
			"",                  // static dir → default
			"hub.example.com", // public hostname
			"y",                 // continue past DNS warning
			"ops@example.com",   // admin email
			"n",                 // expect remote collectors → no
		},
	}
	var buf bytes.Buffer
	a, err := RunWizard(p, &buf, WizardOptions{AllowHub: true})
	if err != nil {
		t.Fatalf("RunWizard: %v", err)
	}
	if a.Mode != ModeHubOnly {
		t.Fatalf("mode: %v", a.Mode)
	}
	if a.ListenAddr != "127.0.0.1" {
		t.Errorf("listen should default to 127.0.0.1 (nginx fronts), got %q", a.ListenAddr)
	}
	if a.HTTPPort != 8080 {
		t.Errorf("http port should default to 8080, got %d", a.HTTPPort)
	}
	if a.PublicURL != "https://hub.example.com" {
		t.Errorf("public url: %q", a.PublicURL)
	}
	if a.AdminEmail != "ops@example.com" {
		t.Errorf("admin email: %q", a.AdminEmail)
	}
	if a.SourceID != "" {
		t.Errorf("hub-only should not set SourceID (no local collector), got %q", a.SourceID)
	}
	if a.RemoteCollectorsExpected {
		t.Errorf("RemoteCollectorsExpected should be false")
	}
	if len(a.Servers) != 0 {
		t.Errorf("hub-only should not prompt for servers, got %+v", a.Servers)
	}
	if err := a.Validate(); err != nil {
		t.Errorf("answers should validate: %v", err)
	}
}
