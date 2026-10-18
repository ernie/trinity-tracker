package setup

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ernie/trinity-tracker/internal/config"
	"gopkg.in/yaml.v3"
)

// ApplyOptions controls install-time side effects.
type ApplyOptions struct {
	ConfigPath string // /etc/trinity/config.yml
	UseSystemd bool   // install systemd units + enable services
	DryRun     bool   // print what would happen instead of doing it
	Out        io.Writer
}

// Apply runs every install-time side effect implied by the answers, in
// this order: user, dirs, config, engine, systemd, logrotate,
// trinity.cfg and per-server cfgs, then enabling services. Each step
// prints what it did; in DryRun mode each prints with a `[DRY]` prefix
// and touches nothing.
func Apply(a *Answers, opts ApplyOptions) error {
	if opts.Out == nil {
		opts.Out = os.Stderr
	}
	if opts.ConfigPath == "" {
		opts.ConfigPath = "/etc/trinity/config.yml"
	}
	if err := a.Validate(); err != nil {
		return fmt.Errorf("answers invalid: %w", err)
	}

	cfg := a.ToConfig()
	if err := config.ValidateForSave(cfg); err != nil {
		return fmt.Errorf("config validation: %w", err)
	}

	plan := &Plan{
		DryRun:     opts.DryRun,
		UseSystemd: opts.UseSystemd,
		Out:        opts.Out,
	}

	uid, gid, err := plan.EnsureUser(a.ServiceUser)
	if err != nil {
		return err
	}

	if err := ensureDirs(plan, a, uid, gid); err != nil {
		return err
	}

	if err := writeConfig(plan, cfg, opts.ConfigPath, gid); err != nil {
		return err
	}

	if a.Mode == ModeCollector {
		if err := installCreds(plan, a.CredsFile, gid); err != nil {
			return err
		}
	}

	if a.RunsLocalServers() && a.InstallEngine {
		plan.Say("Downloading the latest Trinity engine release for %s…", runtimeArch())
		engineTag, err := InstallEngine(plan, "", a.Quake3Dir, "/var/log/quake3", a.StaticDir, uid, gid)
		if err != nil {
			return err
		}
		// Extraction ran as the service user, but the directories and
		// symlinks this process created are root-owned.
		if err := chownRecursive(plan, a.Quake3Dir, uid, gid); err != nil {
			return fmt.Errorf("chown %s: %w", a.Quake3Dir, err)
		}
		plan.Say("Engine installed at %s (%s)", a.Quake3Dir, engineTag)

		// The game servers aren't running yet on a fresh init, so a
		// newer mod can be renamed into place.
		modTag, err := ResolveLatestTag(modRepo)
		if err != nil {
			plan.Say("Warning: could not check for newer mod release: %v", err)
		} else {
			bundled, _ := DetectModVersion(a.Quake3Dir, "baseq3")
			if CompareVersions(bundled, modTag) == StateBehind {
				plan.Say("Mod %s is newer than the engine's bundled %q; installing it over that…", modTag, bundled)
				if _, err := InstallMod(plan, modTag, a.Quake3Dir, uid, gid); err != nil {
					return err
				}
			}
		}
	}

	if opts.UseSystemd {
		if err := installSystemdUnits(plan, a); err != nil {
			return err
		}
		if err := plan.Systemctl("daemon-reload"); err != nil {
			return err
		}
		if err := plan.Systemctl("enable", "trinity.service"); err != nil {
			return err
		}
		plan.Say("Enabled: trinity.service")
		if a.RunsLocalServers() {
			if err := plan.Systemctl("enable", "quake3-servers.target"); err != nil {
				return err
			}
			plan.Say("Enabled: quake3-servers.target")
		}
		if a.DiscordEnabled {
			// The timer pulls in the one-shot service; `--now` schedules
			// the next run right away.
			if err := plan.Systemctl("enable", "--now", "trinity-digest.timer"); err != nil {
				return err
			}
			plan.Say("Enabled: trinity-digest.timer")
		}
	}

	if a.RunsLocalServers() && !a.SkipLogrotate {
		if err := installLogrotate(plan); err != nil {
			return err
		}
	} else if a.SkipLogrotate {
		plan.Say("Skipping /etc/logrotate.d/quake3 (--skip-logrotate)")
	}

	if a.RunsLocalServers() && len(a.Servers) > 0 {
		if err := installPerServerFiles(plan, a, uid, gid); err != nil {
			return err
		}
	}

	if a.SkipNginx {
		plan.Say("Skipping nginx install (--skip-nginx); operator manages reverse proxy")
	} else if (a.Mode == ModeCollector || a.HasHubFields()) && a.PublicURL != "" {
		mode := NginxModeCollector
		if a.HasHubFields() {
			mode = NginxModeHub
		}
		if err := InstallNginx(plan, mode, a.PublicURL, a.AdminEmail, a.StaticDir, a.Quake3Dir, a.RunsLocalServers(), a.SkipCert, a.SkipFirewall); err != nil {
			return err
		}
	}
	// The symlinks point at Let's Encrypt's standard layout, whoever
	// runs nginx.
	if a.RemoteCollectorsExpected {
		if err := installNATSTLSSymlinks(plan, a.PublicURL, a.ServiceUser); err != nil {
			return err
		}
	}

	return nil
}

func runtimeArch() string {
	out, err := exec.Command("uname", "-m").Output()
	if err == nil {
		return strings.TrimSpace(string(out))
	}
	return "unknown"
}

// installNATSTLSSymlinks creates /etc/trinity/tls/{fullchain,privkey}.pem
// as symlinks into /etc/letsencrypt/live/<publicHost>/. The embedded NATS
// broker reads from these paths when remote collectors are expected, and
// renewals reach it through the symlinks.
//
// Certbot's layout (archive/ and live/ at 0700 root:root, privkey.pem at
// 0600) keeps the service user from reading the key. POSIX ACLs grant
// access and leave certbot's ownership alone.
func installNATSTLSSymlinks(plan *Plan, publicURL, serviceUser string) error {
	host := HostFromURL(publicURL)
	if host == "" {
		return fmt.Errorf("installNATSTLSSymlinks: empty hostname from %q", publicURL)
	}
	if serviceUser == "" {
		return fmt.Errorf("installNATSTLSSymlinks: empty service user")
	}
	tlsDir := "/etc/trinity/tls"
	if err := plan.MkdirAll(tlsDir, 0o755); err != nil {
		return err
	}
	pairs := []struct{ name, target string }{
		{"fullchain.pem", "/etc/letsencrypt/live/" + host + "/fullchain.pem"},
		{"privkey.pem", "/etc/letsencrypt/live/" + host + "/privkey.pem"},
	}
	for _, p := range pairs {
		linkPath := filepath.Join(tlsDir, p.name)
		if err := plan.Symlink(p.target, linkPath); err != nil {
			return err
		}
	}
	return grantNATSTLSACLs(plan, host, serviceUser)
}

// grantNATSTLSACLs sets POSIX ACLs that let the service user traverse
// /etc/letsencrypt/{archive,live} and read the current and future
// privkey/fullchain files for the given host.
func grantNATSTLSACLs(plan *Plan, host, serviceUser string) error {
	traversal := "u:" + serviceUser + ":rx"
	if err := plan.Setfacl("-m", traversal, "/etc/letsencrypt/archive", "/etc/letsencrypt/live"); err != nil {
		return err
	}
	archiveDir := "/etc/letsencrypt/archive/" + host
	read := "u:" + serviceUser + ":r"
	// The default ACL carries to every privkeyN.pem and fullchainN.pem
	// a renewal writes.
	if err := plan.Setfacl("-d", "-m", read, archiveDir); err != nil {
		return err
	}
	// In a dry run the directory may not exist yet to glob.
	if plan.DryRun {
		plan.Say("would setfacl -m %s %s/*.pem", read, archiveDir)
		return nil
	}
	matches, err := filepath.Glob(filepath.Join(archiveDir, "*.pem"))
	if err != nil {
		return fmt.Errorf("globbing %s: %w", archiveDir, err)
	}
	if len(matches) == 0 {
		return fmt.Errorf("no *.pem files under %s; has certbot run yet?", archiveDir)
	}
	args := append([]string{"-m", read}, matches...)
	return plan.Setfacl(args...)
}

func ensureDirs(plan *Plan, a *Answers, uid, gid int) error {
	if err := plan.MkdirAll("/etc/trinity", 0750); err != nil {
		return err
	}
	if err := plan.Chown("/etc/trinity", 0, gid); err != nil {
		return err
	}
	plan.Say("Directory: /etc/trinity")

	// Holds the collector watermark and spillover, and the hub DB.
	if err := plan.MkdirChown("/var/lib/trinity", 0755, uid, gid); err != nil {
		return err
	}
	plan.Say("Directory: /var/lib/trinity")

	if a.HasHubFields() && a.StaticDir != "" {
		if err := plan.MkdirChown(a.StaticDir, 0755, uid, gid); err != nil {
			return err
		}
		plan.Say("Directory: %s", a.StaticDir)
		if err := stageWebAssets(plan, a.StaticDir, uid, gid); err != nil {
			return err
		}
	}
	if a.HasHubFields() && a.DatabasePath != "" {
		if err := plan.MkdirChown(filepath.Dir(a.DatabasePath), 0755, uid, gid); err != nil {
			return err
		}
	}
	if a.RunsLocalServers() {
		if err := plan.MkdirChown("/var/log/quake3", 0755, uid, gid); err != nil {
			return err
		}
		plan.Say("Directory: /var/log/quake3")
	}
	return nil
}

// stagedWebDir returns the path to the staged web frontend, or ""
// if install.sh didn't hand one off. install.sh stages under /tmp/ and
// names the directory in TRINITY_INIT_STAGE; its `web/` subdir holds the
// prebuilt frontend. Only paths under /tmp/ count, so a stray variable
// can't point elsewhere.
func stagedWebDir() string {
	root := os.Getenv("TRINITY_INIT_STAGE")
	if root == "" {
		return ""
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return ""
	}
	if !strings.HasPrefix(abs, "/tmp/") {
		return ""
	}
	web := filepath.Join(abs, "web")
	if info, err := os.Stat(web); err != nil || !info.IsDir() {
		return ""
	}
	return web
}

// CleanupStage removes the install.sh-staged tempdir, with
// stagedWebDir's /tmp/ guard.
func CleanupStage() {
	root := os.Getenv("TRINITY_INIT_STAGE")
	if root == "" {
		return
	}
	abs, err := filepath.Abs(root)
	if err != nil || !strings.HasPrefix(abs, "/tmp/") {
		return
	}
	_ = os.RemoveAll(abs)
}

// stageWebAssets copies the staged web frontend into the operator's
// StaticDir as the service user. With nothing staged (a from-source
// install) it prints a hint to copy `web/dist/.` by hand.
func stageWebAssets(plan *Plan, dest string, uid, gid int) error {
	src := stagedWebDir()
	if src == "" {
		fmt.Fprintf(plan.Out, "  NOTE: no staged web assets; copy your `web/dist/.` to %s manually.\n", dest)
		return nil
	}
	// install.sh makes its stage dir 0755 so the service user can read it.
	if err := plan.CopyTreeAs(uid, gid, src, dest); err != nil {
		return fmt.Errorf("staging web assets from %s: %w", src, err)
	}
	fmt.Fprintf(plan.Out, "Web assets: copied %s → %s\n", src, dest)
	return nil
}

func chownRecursive(plan *Plan, root string, uid, gid int) error {
	if plan.DryRun {
		plan.Say("would chown -R %d:%d %s", uid, gid, root)
		return nil
	}
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Lchown changes the engine install's logs symlinks themselves.
		return os.Lchown(path, uid, gid)
	})
}

func writeConfig(plan *Plan, cfg *config.Config, path string, gid int) error {
	// Encoded here, matching config.Save, so a dry run goes through Plan.
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(cfg); err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}
	_ = enc.Close()
	if err := plan.WriteFile(path, buf.Bytes(), 0640); err != nil {
		return err
	}
	if err := plan.Chown(path, 0, gid); err != nil {
		return err
	}
	plan.Say("Wrote: %s", path)
	return nil
}

func installCreds(plan *Plan, src string, gid int) error {
	dest := "/etc/trinity/source.creds"
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("reading creds %s: %w", src, err)
	}
	if err := plan.WriteFile(dest, data, 0640); err != nil {
		return err
	}
	if err := plan.Chown(dest, 0, gid); err != nil {
		return err
	}
	plan.Say("Installed creds: %s", dest)
	return nil
}

func installSystemdUnits(plan *Plan, a *Answers) error {
	units := []string{"trinity.service"}
	if a.RunsLocalServers() {
		units = append(units, "quake3-server@.service", "quake3-servers.target")
	}
	if a.DiscordEnabled {
		units = append(units, "trinity-digest.service", "trinity-digest.timer")
	}
	for _, name := range units {
		var (
			data []byte
			err  error
		)
		if name == "trinity-digest.timer" {
			data, err = SystemdUnitTemplated(name, a.ServiceUser, map[string]string{
				"schedule": a.DiscordSchedule,
			})
		} else {
			data, err = SystemdUnit(name, a.ServiceUser)
		}
		if err != nil {
			return err
		}
		dest := filepath.Join("/etc/systemd/system", name)
		if err := plan.WriteFile(dest, data, 0644); err != nil {
			return err
		}
		plan.Say("Installed unit: %s", dest)
	}
	return nil
}

// logrotateTemplate matches scripts/logrotate.quake3, with the `su`
// line's user and group filled in.
const logrotateTemplate = `/var/log/quake3/*.log {
    daily
    rotate 7
    compress
    su %s %s
    missingok
    notifempty
    copytruncate
}
`

func installLogrotate(plan *Plan) error {
	dest := "/etc/logrotate.d/quake3"
	// Arch ships no /etc/logrotate.d/ in its base install.
	if err := plan.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	// Logrotate must run as the user that holds the logs open.
	content := fmt.Sprintf(logrotateTemplate, "quake", "quake")
	if err := plan.WriteFile(dest, []byte(content), 0644); err != nil {
		return err
	}
	plan.Say("Installed: %s", dest)
	return nil
}

// installPerServerFiles writes trinity.cfg (the shared cvars), one
// <stem>.cfg per game type and mod, and each server's .env file with
// its port and +exec, then enables each systemd template instance.
//
// The rconpassword lives in <stem>.cfg, mode 0640 root:<service-user>,
// so it shows in neither `ps` nor a world-readable file. Servers that
// share a stem, such as two FFA servers, share the first one's
// password.
func installPerServerFiles(plan *Plan, a *Answers, uid, gid int) error {
	publicURL := a.PublicURL

	if len(a.Servers) > 0 {
		// The game looks up cfg files in fs_game first and then baseq3,
		// so one baseq3 copy of trinity.cfg and autoexec.cfg covers
		// missionpack servers too.
		trinityCfg, err := RenderTrinityCfg(publicURL)
		if err != nil {
			return err
		}
		path := filepath.Join(a.Quake3Dir, "baseq3", "trinity.cfg")
		if err := plan.MkdirChown(filepath.Dir(path), 0755, uid, gid); err != nil {
			return err
		}
		if err := plan.WriteFileAs(uid, gid, path, []byte(trinityCfg), 0644); err != nil {
			return err
		}
		plan.Say("Installed: %s", path)

		// A missionpack autoexec.cfg would shadow the operator's
		// baseq3 one.
		autoexec := filepath.Join(a.Quake3Dir, "baseq3", "autoexec.cfg")
		if _, err := os.Stat(autoexec); os.IsNotExist(err) {
			body := "// Generated by trinity init.\nexec trinity.cfg\n"
			if err := plan.WriteFileAs(uid, gid, autoexec, []byte(body), 0644); err != nil {
				return err
			}
			plan.Say("Installed: %s", autoexec)
		} else {
			fmt.Fprintf(plan.Out, "  NOTE: %s already exists. Add `exec trinity.cfg` to it manually.\n", autoexec)
		}
	}

	// Servers of the same game type and mod share one cfg and rotation;
	// a per-server override points the .env's +exec at another cfg.
	type modeKey struct {
		g     Gametype
		useMP bool
	}
	wroteMode := make(map[modeKey]bool)
	for _, s := range a.Servers {
		mk := modeKey{s.Gametype, s.UseMissionpack}
		if wroteMode[mk] {
			continue
		}
		wroteMode[mk] = true
		stem := Stem(s.Gametype, s.UseMissionpack)

		cfgPath := filepath.Join(a.Quake3Dir, s.ModFolder(), stem+".cfg")
		if err := plan.MkdirAll(filepath.Dir(cfgPath), 0755); err != nil {
			return err
		}
		body, err := RenderServerCfg(s.Gametype, s.UseMissionpack, s.RconPassword)
		if err != nil {
			return err
		}
		if err := plan.WriteFile(cfgPath, []byte(body), 0640); err != nil {
			return err
		}
		if err := plan.Chown(cfgPath, 0, gid); err != nil {
			return err
		}
		plan.Say("Wrote: %s", cfgPath)

		rotPath := filepath.Join(filepath.Dir(cfgPath), "rotation."+stem)
		rotBody, err := RenderRotation(s.Gametype, s.UseMissionpack)
		if err != nil {
			return err
		}
		if err := plan.WriteFileAs(uid, gid, rotPath, rotBody, 0644); err != nil {
			return err
		}
		plan.Say("Wrote: %s", rotPath)
	}

	for _, s := range a.Servers {
		stem := Stem(s.Gametype, s.UseMissionpack)
		envPath := filepath.Join("/etc/trinity", s.Key+".env")
		opts := fmt.Sprintf("+set net_port %d", s.Port)
		if s.RunsMissionpack() {
			opts += " +set fs_game missionpack"
		}
		opts += " +exec " + stem + ".cfg"
		if err := plan.WriteFile(envPath, []byte(fmt.Sprintf("SERVER_OPTS=%s\n", opts)), 0644); err != nil {
			return err
		}
		plan.Say("Wrote: %s", envPath)

		unit := "quake3-server@" + s.Key
		if err := plan.Systemctl("enable", unit); err != nil {
			fmt.Fprintf(plan.Out, "  WARN: systemctl enable %s failed: %v\n", unit, err)
		} else if plan.UseSystemd {
			plan.Say("Enabled: %s", unit)
		}
	}
	return nil
}
