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

// Apply runs every install-time side effect implied by the answers.
// Order matters: user → dirs → config → engine → systemd → logrotate
// → trinity.cfg/per-server cfgs → enable services. Each step prints
// what it did so the operator can follow along.
//
// In DryRun mode no host state is touched; every action is printed
// with a `[DRY]` prefix instead.
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
		plan.say("Downloading latest trinity-engine release for %s ...", runtimeArch())
		if err := InstallEngine(plan, a.Quake3Dir, "/var/log/quake3", a.StaticDir); err != nil {
			return err
		}
		if err := chownRecursive(plan, a.Quake3Dir, uid, gid); err != nil {
			return fmt.Errorf("chown %s: %w", a.Quake3Dir, err)
		}
		plan.say("Engine installed at %s", a.Quake3Dir)
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
		plan.say("Enabled: trinity.service")
		if a.RunsLocalServers() {
			if err := plan.Systemctl("enable", "quake3-servers.target"); err != nil {
				return err
			}
			plan.say("Enabled: quake3-servers.target")
		}
	}

	if a.RunsLocalServers() {
		if err := installLogrotate(plan); err != nil {
			return err
		}
	}

	if a.RunsLocalServers() && len(a.Servers) > 0 {
		if err := installPerServerFiles(plan, a, uid, gid); err != nil {
			return err
		}
	}

	if a.Mode == ModeCollector && a.PublicURL != "" && a.AdminEmail != "" {
		if err := InstallNginx(plan, a.PublicURL, a.AdminEmail, a.StaticDir, a.Quake3Dir); err != nil {
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

func ensureDirs(plan *Plan, a *Answers, uid, gid int) error {
	// /etc/trinity is root-owned, group-readable by service user.
	if err := plan.MkdirAll("/etc/trinity", 0750); err != nil {
		return err
	}
	if err := plan.Chown("/etc/trinity", 0, gid); err != nil {
		return err
	}
	plan.say("Directory: /etc/trinity")

	// /var/lib/trinity always (collector watermark + spillover, hub DB).
	if err := plan.MkdirChown("/var/lib/trinity", 0755, uid, gid); err != nil {
		return err
	}
	plan.say("Directory: /var/lib/trinity")

	if a.HasHubFields() && a.StaticDir != "" {
		if err := plan.MkdirChown(a.StaticDir, 0755, uid, gid); err != nil {
			return err
		}
		plan.say("Directory: %s", a.StaticDir)
		// install.sh stages the prebuilt web frontend at a /tmp dir
		// before exec'ing the wizard. If it's there, populate the live
		// dir from it so a fresh hub install renders something on first
		// start. Operators who built from source can still copy
		// `web/dist/.` in manually.
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
		plan.say("Directory: /var/log/quake3")
	}
	return nil
}

// stagedWebDir returns the path to the staged web frontend, or ""
// if install.sh didn't hand one off. install.sh writes to a tempdir
// under /tmp/ and exposes it via the TRINITY_INIT_STAGE env var; the
// `web/` subdir there holds the prebuilt frontend.
//
// Defensive: only honors paths under /tmp/, so a stray env var can't
// point us at arbitrary parts of the filesystem.
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

// CleanupStage removes the install.sh-staged tempdir. Called by
// `trinity init` once Apply has consumed the staged web assets,
// success or failure. Same /tmp/ guard as stagedWebDir.
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
// configured StaticDir. No-op (with a hint) if nothing was staged —
// from-source installs that didn't go through install.sh need to
// copy `web/dist/.` themselves.
func stageWebAssets(plan *Plan, dest string, uid, gid int) error {
	src := stagedWebDir()
	if src == "" {
		fmt.Fprintf(plan.Out, "  NOTE: no staged web assets; copy your `web/dist/.` to %s manually.\n", dest)
		return nil
	}
	if plan.DryRun {
		plan.say("would copy tree %s → %s (chown %d:%d)", src, dest, uid, gid)
		return nil
	}
	if err := copyTree(src, dest, uid, gid); err != nil {
		return fmt.Errorf("staging web assets from %s: %w", src, err)
	}
	fmt.Fprintf(plan.Out, "Web assets: copied %s → %s\n", src, dest)
	return nil
}

// copyTree mirrors src into dst, chowning every entry to (uid, gid).
// Existing files in dst are overwritten; nothing in dst is removed.
// Designed for the web frontend overlay — the destination may already
// hold pk3-extracted runtime assets (levelshots, demopk3s) that we
// must not blow away.
func copyTree(src, dst string, uid, gid int) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			if err := os.MkdirAll(target, info.Mode().Perm()); err != nil {
				return err
			}
			return os.Chown(target, uid, gid)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			_ = os.Remove(target)
			if err := os.Symlink(link, target); err != nil {
				return err
			}
			return os.Lchown(target, uid, gid)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
		if err != nil {
			return err
		}
		defer out.Close()
		if _, err := io.Copy(out, in); err != nil {
			return err
		}
		return os.Chown(target, uid, gid)
	})
}

func chownRecursive(plan *Plan, root string, uid, gid int) error {
	if plan.DryRun {
		plan.say("would chown -R %d:%d %s", uid, gid, root)
		return nil
	}
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Walk follows symlinks for stat, but Lchown applies to the
		// link itself when path is a link — which is what we want
		// (the engine install creates baseq3/logs and missionpack/logs
		// symlinks; chowning them keeps ls -l predictable).
		return os.Lchown(path, uid, gid)
	})
}

func writeConfig(plan *Plan, cfg *config.Config, path string, gid int) error {
	// Marshal the config ourselves so dry-run can show what it would
	// write without touching the filesystem. config.Save's serialization
	// is just yaml.Marshal — see internal/config/config.go.
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
	plan.say("Wrote: %s", path)
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
	plan.say("Installed creds: %s", dest)
	return nil
}

func installSystemdUnits(plan *Plan, a *Answers) error {
	units := []string{"trinity.service"}
	if a.RunsLocalServers() {
		units = append(units, "quake3-server@.service", "quake3-servers.target")
	}
	for _, name := range units {
		data, err := SystemdUnit(name, a.ServiceUser)
		if err != nil {
			return err
		}
		dest := filepath.Join("/etc/systemd/system", name)
		if err := plan.WriteFile(dest, data, 0644); err != nil {
			return err
		}
		plan.say("Installed unit: %s", dest)
	}
	return nil
}

// logrotateTemplate is the same content as scripts/logrotate.quake3.
// Kept inline (rather than embedded) because it's nine lines and the
// `su` line bakes in the service user — easier to template than to
// post-process an embedded blob.
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
	// "quake quake" su line: a non-root logrotate can't truncate files
	// the systemd unit (running as quake) holds open.
	content := fmt.Sprintf(logrotateTemplate, "quake", "quake")
	if err := plan.WriteFile(dest, []byte(content), 0644); err != nil {
		return err
	}
	plan.say("Installed: %s", dest)
	return nil
}

// installPerServerFiles writes the trinity.cfg (shared cvars, derived
// from the first server's RCON password — which is the per-host one),
// one shared <stem>.cfg per gametype/mod tuple, each server's .env
// file with the bind port + +exec, and enables the systemd template
// instance.
//
// Note: q3 only supports a single rcon_password per server process,
// so RCON is effectively per-host. The wizard prompts per-server but
// they should match; we use the first server's value as the canonical
// trinity.cfg one.
func installPerServerFiles(plan *Plan, a *Answers, uid, gid int) error {
	publicURL := a.PublicURL

	// trinity.cfg is per-mod-folder. If any server uses missionpack
	// we install it there too.
	mods := map[string]bool{"baseq3": true}
	for _, s := range a.Servers {
		if s.RunsMissionpack() {
			mods["missionpack"] = true
		}
	}
	if len(a.Servers) > 0 {
		trinityCfg, err := RenderTrinityCfg(a.Servers[0].RconPassword, publicURL)
		if err != nil {
			return err
		}
		for mod := range mods {
			path := filepath.Join(a.Quake3Dir, mod, "trinity.cfg")
			if err := plan.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return err
			}
			if err := plan.WriteFile(path, []byte(trinityCfg), 0644); err != nil {
				return err
			}
			if err := plan.Chown(path, uid, gid); err != nil {
				return err
			}
			plan.say("Installed: %s", path)
		}

		// trinity-bots.txt: curated bot list referenced by
		// quake3-server@.service's `+set g_botsfile`. bot_minplayers
		// reads this; without it, bot fill silently fails. Always
		// installed to both baseq3 and missionpack — cheap, and saves
		// surprise if an operator later adds a TA server without
		// re-running the wizard.
		botsFile, err := TrinityBotsFile()
		if err != nil {
			return err
		}
		for _, m := range []string{"baseq3", "missionpack"} {
			botsPath := filepath.Join(a.Quake3Dir, m, "scripts", "trinity-bots.txt")
			if err := plan.MkdirAll(filepath.Dir(botsPath), 0755); err != nil {
				return err
			}
			if err := plan.WriteFile(botsPath, botsFile, 0644); err != nil {
				return err
			}
			if err := plan.Chown(botsPath, uid, gid); err != nil {
				return err
			}
			plan.say("Installed: %s", botsPath)
		}

		// autoexec.cfg only needs to live in baseq3 — q3's vfs looks
		// up cfg files in the active mod first and falls back to
		// baseq3, so a baseq3/autoexec.cfg that does `exec trinity.cfg`
		// covers missionpack too. Writing one in missionpack would
		// shadow operator customizations they put in baseq3.
		autoexec := filepath.Join(a.Quake3Dir, "baseq3", "autoexec.cfg")
		if _, err := os.Stat(autoexec); os.IsNotExist(err) {
			body := "// Generated by trinity init.\nexec trinity.cfg\n"
			if err := plan.WriteFile(autoexec, []byte(body), 0644); err != nil {
				return err
			}
			if err := plan.Chown(autoexec, uid, gid); err != nil {
				return err
			}
			plan.say("Installed: %s", autoexec)
		} else {
			fmt.Fprintf(plan.Out, "  NOTE: %s already exists. Add `exec trinity.cfg` to it manually.\n", autoexec)
		}
	}

	// Shared <stem>.cfg + rotation.<stem> per (gametype, useMissionpack)
	// tuple — multiple servers of the same mode share one cfg.
	// Operators wanting per-server overrides edit the .env file to
	// point +exec at a custom cfg.
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
		body, err := RenderServerCfg(s.Gametype, s.UseMissionpack)
		if err != nil {
			return err
		}
		if err := plan.WriteFile(cfgPath, []byte(body), 0644); err != nil {
			return err
		}
		if err := plan.Chown(cfgPath, uid, gid); err != nil {
			return err
		}
		plan.say("Wrote: %s", cfgPath)

		rotPath := filepath.Join(filepath.Dir(cfgPath), "rotation."+stem)
		rotBody, err := RenderRotation(s.Gametype, s.UseMissionpack)
		if err != nil {
			return err
		}
		if err := plan.WriteFile(rotPath, rotBody, 0644); err != nil {
			return err
		}
		if err := plan.Chown(rotPath, uid, gid); err != nil {
			return err
		}
		plan.say("Wrote: %s", rotPath)
	}

	// Per-instance .env file (port, fs_game, +exec the shared cfg) and
	// systemd enable.
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
		plan.say("Wrote: %s", envPath)

		unit := "quake3-server@" + s.Key
		if err := plan.Systemctl("enable", unit); err != nil {
			fmt.Fprintf(plan.Out, "  WARN: systemctl enable %s failed: %v\n", unit, err)
		} else if plan.UseSystemd {
			plan.say("Enabled: %s", unit)
		}
	}
	return nil
}
