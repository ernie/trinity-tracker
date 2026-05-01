package setup

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newDryPlan() (*Plan, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	return &Plan{DryRun: true, UseSystemd: true, Out: buf}, buf
}

// assertDoesntExist fails the test if path was created on disk —
// the whole point of dry-run is that no host state changes.
func assertDoesntExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); err == nil {
		t.Errorf("dry-run created %s on disk", path)
	}
}

func TestPlan_DryRun_MkdirAll(t *testing.T) {
	plan, buf := newDryPlan()
	target := filepath.Join(t.TempDir(), "should-not-exist")
	if err := plan.MkdirAll(target, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	assertDoesntExist(t, target)
	out := buf.String()
	if !strings.HasPrefix(out, "[DRY] ") {
		t.Errorf("expected [DRY] prefix; got %q", out)
	}
	if !strings.Contains(out, "mkdir -p "+target) {
		t.Errorf("expected mkdir line; got %q", out)
	}
}

func TestPlan_DryRun_WriteFile(t *testing.T) {
	plan, buf := newDryPlan()
	target := filepath.Join(t.TempDir(), "should-not-exist.cfg")
	if err := plan.WriteFile(target, []byte("hello"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	assertDoesntExist(t, target)
	if !strings.Contains(buf.String(), "would write "+target) {
		t.Errorf("expected write line; got %q", buf.String())
	}
}

func TestPlan_DryRun_Symlink(t *testing.T) {
	plan, buf := newDryPlan()
	link := filepath.Join(t.TempDir(), "link")
	if err := plan.Symlink("/nonexistent/target", link); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	assertDoesntExist(t, link)
	if !strings.Contains(buf.String(), "symlink "+link) {
		t.Errorf("expected symlink line; got %q", buf.String())
	}
}

func TestPlan_DryRun_Systemctl(t *testing.T) {
	plan, buf := newDryPlan()
	if err := plan.Systemctl("daemon-reload"); err != nil {
		t.Fatalf("Systemctl: %v", err)
	}
	if !strings.Contains(buf.String(), "would systemctl daemon-reload") {
		t.Errorf("expected systemctl line; got %q", buf.String())
	}
}

func TestPlan_Systemctl_NoOpWhenDisabled(t *testing.T) {
	buf := &bytes.Buffer{}
	plan := &Plan{DryRun: true, UseSystemd: false, Out: buf}
	if err := plan.Systemctl("enable", "trinity.service"); err != nil {
		t.Fatalf("Systemctl: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected silent no-op when UseSystemd=false; got %q", buf.String())
	}
}

func TestPlan_DryRun_EnsureUser_NewUser(t *testing.T) {
	// Use an obscure name unlikely to exist on any test host.
	plan, buf := newDryPlan()
	uid, gid, err := plan.EnsureUser("trinity-dryrun-test-user-xyzzy")
	if err != nil {
		t.Fatalf("EnsureUser: %v", err)
	}
	if uid != 0 || gid != 0 {
		t.Errorf("expected (0,0) placeholder ids in dry-run; got (%d,%d)", uid, gid)
	}
	if !strings.Contains(buf.String(), "would useradd") {
		t.Errorf("expected useradd line; got %q", buf.String())
	}
}

func TestPlan_DryRun_RemoveAll(t *testing.T) {
	plan, buf := newDryPlan()
	// Make a real file to verify it isn't removed.
	dir := t.TempDir()
	canary := filepath.Join(dir, "canary")
	if err := os.WriteFile(canary, []byte("x"), 0644); err != nil {
		t.Fatalf("setup canary: %v", err)
	}
	if err := plan.RemoveAll(dir); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}
	if _, err := os.Stat(canary); err != nil {
		t.Errorf("dry-run removed real file: %v", err)
	}
	if !strings.Contains(buf.String(), "would rm -rf "+dir) {
		t.Errorf("expected rm -rf line; got %q", buf.String())
	}
}

func TestPlan_RealMode_NoPrefix(t *testing.T) {
	buf := &bytes.Buffer{}
	plan := &Plan{DryRun: false, UseSystemd: false, Out: buf}
	dir := filepath.Join(t.TempDir(), "real")
	if err := plan.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("real-mode mkdir didn't create %s: %v", dir, err)
	}
	// In real mode helpers don't chatter — only operations Apply
	// explicitly logs (via plan.say) do.
	if buf.Len() != 0 {
		t.Errorf("expected silent real-mode mkdir; got %q", buf.String())
	}
}

// TestApply_DryRun_HubOnly covers the wider plumbing: a hub-only
// Apply in dry-run touches no filesystem state and emits a [DRY]
// line for each step.
func TestApply_DryRun_HubOnly(t *testing.T) {
	tmp := t.TempDir()
	staticDir := filepath.Join(tmp, "web")
	dbDir := filepath.Join(tmp, "var")

	a := &Answers{
		Mode:         ModeHubOnly,
		ServiceUser:  "quake",
		ListenAddr:   "127.0.0.1",
		HTTPPort:     8080,
		DatabasePath: filepath.Join(dbDir, "trinity.db"),
		StaticDir:    staticDir,
		PublicURL:    "https://hub.example.com",
		AdminEmail:   "ops@example.com",
	}
	if err := a.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	buf := &bytes.Buffer{}
	cfgPath := filepath.Join(tmp, "config.yml")
	err := Apply(a, ApplyOptions{
		ConfigPath: cfgPath,
		UseSystemd: true,
		DryRun:     true,
		Out:        buf,
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if _, err := os.Stat(cfgPath); err == nil {
		t.Errorf("dry-run created config at %s", cfgPath)
	}
	if _, err := os.Stat(staticDir); err == nil {
		t.Errorf("dry-run created static dir at %s", staticDir)
	}
	if _, err := os.Stat(dbDir); err == nil {
		t.Errorf("dry-run created db dir at %s", dbDir)
	}

	out := buf.String()
	for _, want := range []string{
		"[DRY] would mkdir -p /etc/trinity",
		"[DRY] would mkdir -p /var/lib/trinity",
		"[DRY] would write " + cfgPath,
		"[DRY] would write /etc/systemd/system/trinity.service",
		"[DRY] would systemctl daemon-reload",
		"[DRY] would systemctl enable trinity.service",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in dry-run output\n--- output ---\n%s", want, out)
		}
	}
	// Hub-only must NOT install quake3 unit or logrotate.
	for _, unwanted := range []string{
		"quake3-server@.service",
		"quake3-servers.target",
		"/etc/logrotate.d/quake3",
	} {
		if strings.Contains(out, unwanted) {
			t.Errorf("hub-only dry-run referenced %q\n--- output ---\n%s", unwanted, out)
		}
	}
	// Hub-only with PublicURL+AdminEmail set must announce nginx install.
	if !strings.Contains(out, "would render hub nginx config for hub.example.com") {
		t.Errorf("expected hub nginx render announcement\n--- output ---\n%s", out)
	}
	if !strings.Contains(out, "4222/tcp") {
		t.Errorf("hub nginx firewall plan missing 4222/tcp\n--- output ---\n%s", out)
	}
	// RemoteCollectorsExpected was false → no TLS symlinks should be planned.
	if strings.Contains(out, "/etc/trinity/tls/fullchain.pem") {
		t.Errorf("dry-run mentioned TLS symlinks despite RemoteCollectorsExpected=false\n%s", out)
	}
}

// TestApply_DryRun_Combined_RemoteCollectors asserts that combined mode
// with RemoteCollectorsExpected creates the /etc/trinity/tls/ symlinks
// that the embedded NATS broker reads cert/key from. These are stamped
// as symlinks (not files) so cert renewals propagate without any extra
// orchestration.
func TestApply_DryRun_Combined_RemoteCollectors(t *testing.T) {
	tmp := t.TempDir()
	a := &Answers{
		Mode:                     ModeCombined,
		ServiceUser:              "quake",
		ListenAddr:               "127.0.0.1",
		HTTPPort:                 8080,
		DatabasePath:             filepath.Join(tmp, "var", "trinity.db"),
		StaticDir:                filepath.Join(tmp, "web"),
		Quake3Dir:                filepath.Join(tmp, "q3"),
		PublicURL:                "https://hub.example.com",
		AdminEmail:               "ops@example.com",
		SourceID:                 "hub",
		RemoteCollectorsExpected: true,
		Servers: []ServerAnswers{
			{Key: "ffa", Gametype: GametypeFFA, Address: "127.0.0.1:27960", Port: 27960, RconPassword: "secret", LogPath: "/var/log/quake3/ffa.log"},
		},
	}
	if err := a.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	buf := &bytes.Buffer{}
	if err := Apply(a, ApplyOptions{
		ConfigPath: filepath.Join(tmp, "config.yml"),
		UseSystemd: true,
		DryRun:     true,
		Out:        buf,
	}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"would render hub nginx config for hub.example.com",
		"4222/tcp",
		"/etc/trinity/tls/fullchain.pem",
		"/etc/trinity/tls/privkey.pem",
		"/etc/letsencrypt/live/hub.example.com/fullchain.pem",
		"/etc/letsencrypt/live/hub.example.com/privkey.pem",
		// ACLs grant the service user traversal of /etc/letsencrypt
		// and read on the archived cert files (incl. a default ACL
		// on archive/<host>/ for future renewals).
		"would setfacl -m u:quake:rx /etc/letsencrypt/archive /etc/letsencrypt/live",
		"would setfacl -d -m u:quake:r /etc/letsencrypt/archive/hub.example.com",
		"would setfacl -m u:quake:r /etc/letsencrypt/archive/hub.example.com/*.pem",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("combined+remote-collectors dry-run missing %q\n--- output ---\n%s", want, out)
		}
	}
}

// TestApply_DryRun_Combined_SkipCert asserts the --skip-cert path is
// announced in dry-run when SkipCert=true.
func TestApply_DryRun_Combined_SkipCert(t *testing.T) {
	tmp := t.TempDir()
	a := &Answers{
		Mode:         ModeCombined,
		ServiceUser:  "quake",
		ListenAddr:   "127.0.0.1",
		HTTPPort:     8080,
		DatabasePath: filepath.Join(tmp, "var", "trinity.db"),
		StaticDir:    filepath.Join(tmp, "web"),
		Quake3Dir:    filepath.Join(tmp, "q3"),
		PublicURL:    "https://hub.example.com",
		AdminEmail:   "ops@example.com",
		SourceID:     "hub",
		SkipCert:     true,
		Servers: []ServerAnswers{
			{Key: "ffa", Gametype: GametypeFFA, Address: "127.0.0.1:27960", Port: 27960, RconPassword: "secret", LogPath: "/var/log/quake3/ffa.log"},
		},
	}
	if err := a.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	buf := &bytes.Buffer{}
	if err := Apply(a, ApplyOptions{
		ConfigPath: filepath.Join(tmp, "config.yml"),
		UseSystemd: true,
		DryRun:     true,
		Out:        buf,
	}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "skip-cert: caller has staged /etc/letsencrypt/") {
		t.Errorf("expected skip-cert announcement\n--- output ---\n%s", out)
	}
	if strings.Contains(out, "obtain a Let's Encrypt cert") {
		t.Errorf("skip-cert path should not announce a cert obtain step\n%s", out)
	}
}
