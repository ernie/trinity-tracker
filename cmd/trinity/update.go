package main

import (
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"

	"github.com/ernie/trinity-tracker/cmd/trinity/setup"
	"github.com/ernie/trinity-tracker/internal/config"
	flag "github.com/spf13/pflag"
)

const updateLockPath = "/var/lib/trinity/.update.lock"

// cmdHelper is the child that trinity init and trinity update re-exec as
// the service user for writes, through Plan's WriteFileAs, CopyTreeAs,
// DownloadAs and UnzipAs. It's hidden from --help.
//
//	_helper write    <path> <mode>             (reads stdin)
//	_helper copy     <src> <dst>
//	_helper download <url> <dest>
//	_helper unzip    <zip> <dest> <strip-components>
//
// It re-execs /proc/self/exe, so swapping the binary mid-update doesn't
// affect a running helper.
func cmdHelper(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: trinity _helper <verb> [args]")
		os.Exit(2)
	}
	switch args[0] {
	case "write":
		helperWrite(args[1:])
	case "copy":
		helperCopy(args[1:])
	case "download":
		helperDownload(args[1:])
	case "unzip":
		helperUnzip(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown _helper verb %q\n", args[0])
		os.Exit(2)
	}
}

func helperWrite(args []string) {
	if len(args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: trinity _helper write <path> <mode>")
		os.Exit(2)
	}
	mode, err := strconv.ParseUint(args[1], 0, 32)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bad mode %q: %v\n", args[1], err)
		os.Exit(2)
	}
	out, err := os.OpenFile(args[0], os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.FileMode(mode))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer out.Close()
	if _, err := io.Copy(out, os.Stdin); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// OpenFile applies the umask, so set the mode explicitly.
	if err := os.Chmod(args[0], os.FileMode(mode)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func helperCopy(args []string) {
	if len(args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: trinity _helper copy <src> <dst>")
		os.Exit(2)
	}
	if err := setup.CopyTree(args[0], args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func helperDownload(args []string) {
	if len(args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: trinity _helper download <url> <dest>")
		os.Exit(2)
	}
	if err := setup.DownloadURL(args[0], args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func helperUnzip(args []string) {
	if len(args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: trinity _helper unzip <zip> <dest> <strip-components>")
		os.Exit(2)
	}
	strip, err := strconv.Atoi(args[2])
	if err != nil {
		fmt.Fprintf(os.Stderr, "bad strip-components %q: %v\n", args[2], err)
		os.Exit(2)
	}
	if err := setup.UnzipInto(args[0], args[1], strip); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// cmdUpdate brings the tracker binary, web bundle, engine and mod up to
// their latest GitHub releases, skipping any that are current; with
// nothing behind, it restarts nothing. Under a flock it downloads and
// verifies everything, asks, stops services, swaps the binary, overlays
// the web bundle, extracts the engine, overlays the mod if it's still
// behind, and starts services.
func cmdUpdate(args []string) {
	fs := flag.NewFlagSet("update", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to config file")
	check := fs.Bool("check", false, "print versions and exit (0 if all current, 1 if any behind)")
	dryRun := fs.Bool("dry-run", false, "stage and verify downloads and print the plan, without installing or restarting services")
	yes := fs.Bool("yes", false, "skip the apply confirmation prompt")
	noRestart := fs.Bool("no-restart", false, "install the files but leave systemd alone")
	force := fs.Bool("force", false, "re-install even when current matches latest (no silent downgrade)")
	trackerTag := fs.String("tracker-tag", "", "pin a specific trinity-tracker tag (default: latest)")
	engineTag := fs.String("engine-tag", "", "pin a specific trinity-engine tag (default: latest)")
	modTag := fs.String("mod-tag", "", "pin a specific trinity (mod) tag (default: latest)")
	fs.Parse(args)

	// Writing /usr/local/bin/trinity, the static dir and Quake3Dir needs root.
	if !*dryRun && !*check && os.Getuid() != 0 {
		fmt.Fprintln(os.Stderr, "Error: trinity update must be run as root (or use --dry-run / --check).")
		os.Exit(1)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Concurrent updates corrupt the install.
	if !*check {
		release, err := acquireUpdateLock()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: another trinity update appears to be running.\n")
			fmt.Fprintf(os.Stderr, "       Lock: %s (rm if you're sure no other update is in flight).\n", updateLockPath)
			fmt.Fprintf(os.Stderr, "       (%v)\n", err)
			os.Exit(1)
		}
		defer release()
	}

	hasHub := cfg.Tracker != nil && cfg.Tracker.Hub != nil
	hasLocalServers := len(cfg.Q3Servers) > 0
	staticDir := cfg.Server.StaticDir
	quake3Dir := cfg.Server.Quake3Dir
	useSd := useSystemd(cfg)
	svcUser := serviceUser(cfg)

	var uid, gid int
	if hasLocalServers || (hasHub && staticDir != "") {
		u, err := userLookup(svcUser)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error looking up service user %q: %v\n", svcUser, err)
			os.Exit(1)
		}
		uid, gid = u.uid, u.gid
	}

	targetTracker := *trackerTag
	if targetTracker == "" {
		t, err := setup.ResolveLatestTag("ernie/trinity-tracker")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error resolving latest tracker tag: %v\n", err)
			os.Exit(1)
		}
		targetTracker = t
	}
	var targetEngine, targetMod string
	if hasLocalServers {
		if *engineTag != "" {
			targetEngine = *engineTag
		} else {
			t, err := setup.ResolveLatestTag("ernie/trinity-engine")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error resolving latest engine tag: %v\n", err)
				os.Exit(1)
			}
			targetEngine = t
		}
		if *modTag != "" {
			targetMod = *modTag
		} else {
			t, err := setup.ResolveLatestTag("ernie/trinity")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error resolving latest mod tag: %v\n", err)
				os.Exit(1)
			}
			targetMod = t
		}
	}

	currentTracker := version
	var currentEngine, currentBaseq3, currentMissionpack string
	var announcerExpected string
	var modRows []row
	if hasLocalServers {
		currentEngine, _ = setup.DetectEngineVersion(quake3Dir)
		currentBaseq3, _ = setup.DetectModVersion(quake3Dir, "baseq3")
		currentMissionpack, _ = setup.DetectModVersion(quake3Dir, "missionpack")

		expected, err := setup.FetchExpectedChecksum("ernie/trinity", targetMod, "zzz-trinity-announcer.pk3")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not check announcer state: %v\n", err)
		} else {
			announcerExpected = expected
		}
	}

	// The mod gets a row per subdir, plus the announcer.
	var rows []row
	rows = append(rows, makeRow("tracker", currentTracker, targetTracker, *force))
	if hasLocalServers {
		rows = append(rows, makeRow("engine", currentEngine, targetEngine, *force))
		modRows = []row{
			makeRow("mod (baseq3)", currentBaseq3, targetMod, *force),
			makeRow("mod (missionpack)", currentMissionpack, targetMod, *force),
		}
		if announcerExpected != "" {
			announcerPath := filepath.Join(quake3Dir, "baseq3", "zzz-trinity-announcer.pk3")
			announcerState, currentHash := setup.CheckAnnouncerState(announcerPath, announcerExpected)
			modRows = append(modRows, makeAnnouncerRow(announcerState, shortHash(currentHash), shortHash(announcerExpected), *force))
		}
		rows = append(rows, modRows...)
	}
	printDecisionTable(rows)

	decisions := []decision{rowToDecision(rows[0])}
	if hasLocalServers {
		decisions = append(decisions, rowToDecision(rows[1])) // engine
		decisions = append(decisions, mergeModDecision(modRows, targetMod, *force))
	}

	anyActionable := false
	for _, d := range decisions {
		if d.action != "current" {
			anyActionable = true
			break
		}
	}

	if *check {
		if anyActionable {
			os.Exit(1)
		}
		os.Exit(0)
	}
	if !anyActionable {
		fmt.Fprintln(os.Stderr, "Everything is up to date. Nothing to do.")
		return
	}

	// --dry-run approves everything so staging exercises every download.
	// A custom build's prompt defaults to no, so pressing Enter never
	// overwrites it.
	if !*yes && !*dryRun {
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Each component below can be applied or skipped independently.")
		fmt.Fprintln(os.Stderr, "Approving tracker restarts trinity.service.")
		fmt.Fprintln(os.Stderr, "Approving engine or mod restarts quake3-servers.target")
		fmt.Fprintln(os.Stderr, "(active players on those servers will be disconnected).")
		fmt.Fprintln(os.Stderr)
	}
	approve := map[string]bool{}
	prompter := setup.NewStdPrompter()
	for i, d := range decisions {
		if d.action == "current" {
			continue
		}
		if *yes || *dryRun {
			approve[d.name] = true
			continue
		}
		ok, err := prompter.YesNo(d.prompt, d.defaultYes)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Couldn't read your answer: %v\n", err)
			os.Exit(1)
		}
		approve[d.name] = ok
		_ = i
	}

	approvedAny := false
	for _, v := range approve {
		if v {
			approvedAny = true
			break
		}
	}
	if !approvedAny {
		fmt.Fprintln(os.Stderr, "Nothing approved, so nothing changed.")
		return
	}

	plan := &setup.Plan{
		DryRun:     *dryRun,
		UseSystemd: useSd,
		Out:        os.Stderr,
	}

	// Stage and verify everything before touching services, so a failed
	// download leaves the install untouched. The service user copies the
	// web overlay out of here, so the 0700 temp dir is opened to 0755.
	stageDir, err := os.MkdirTemp("", "trinity-update-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating staging dir: %v\n", err)
		os.Exit(1)
	}
	if err := os.Chmod(stageDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error opening stage dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(stageDir)

	var trackerStage *setup.TrackerStage
	if approve["tracker"] {
		fmt.Fprintln(os.Stderr, "Staging the tracker release…")
		trackerStage, err = setup.DownloadTrackerRelease(targetTracker, stageDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error staging tracker: %v\n", err)
			os.Exit(1)
		}
	}

	if *dryRun {
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Dry run complete: downloads staged and verified, nothing installed.")
		return
	}

	// trinity.service restarts when the binary or web bundle changes,
	// quake3-servers.target when the engine or mod does. A failure from
	// here leaves the affected services stopped until the next run.
	restartTrinity := !*noRestart && approve["tracker"]
	restartQ3 := !*noRestart && hasLocalServers && (approve["engine"] || approve["mod"])
	if restartQ3 {
		plan.Say("Stopping quake3-servers.target…")
		if err := plan.Systemctl("stop", "quake3-servers.target"); err != nil {
			fmt.Fprintf(os.Stderr, "Error stopping the Quake III servers: %v\n", err)
			os.Exit(1)
		}
	}
	if restartTrinity {
		plan.Say("Stopping trinity.service…")
		if err := plan.Systemctl("stop", "trinity.service"); err != nil {
			fmt.Fprintf(os.Stderr, "Error stopping trinity.service: %v\n", err)
			os.Exit(1)
		}
	}

	if trackerStage != nil {
		if err := installTrackerBinary(trackerStage.BinPath); err != nil {
			fmt.Fprintf(os.Stderr, "Error installing tracker binary: %v\n", err)
			os.Exit(1)
		}
		plan.Say("Installed /usr/local/bin/trinity (%s)", trackerStage.ResolvedTag)

		if hasHub && staticDir != "" && trackerStage.WebDir != "" {
			plan.Say("Overlaying the web bundle into %s as %s…", staticDir, svcUser)
			if err := plan.CopyTreeAs(uid, gid, trackerStage.WebDir, staticDir); err != nil {
				fmt.Fprintf(os.Stderr, "Error overlaying web bundle: %v\n", err)
				os.Exit(1)
			}
		}
	}

	if hasLocalServers && approve["engine"] {
		plan.Say("Installing trinity-engine %s…", targetEngine)
		if _, err := setup.InstallEngine(plan, targetEngine, quake3Dir, "/var/log/quake3", staticDir, uid, gid); err != nil {
			fmt.Fprintf(os.Stderr, "Error installing engine: %v\n", err)
			os.Exit(1)
		}
	}

	// The engine bundle may already carry a current mod.
	if hasLocalServers && approve["mod"] {
		nowBaseq3, _ := setup.DetectModVersion(quake3Dir, "baseq3")
		nowMissionpack, _ := setup.DetectModVersion(quake3Dir, "missionpack")
		baseq3State := setup.CompareVersions(nowBaseq3, targetMod)
		mpState := setup.CompareVersions(nowMissionpack, targetMod)
		announcerPath := filepath.Join(quake3Dir, "baseq3", "zzz-trinity-announcer.pk3")
		nowAnnouncerState, _ := setup.CheckAnnouncerState(announcerPath, announcerExpected)
		if (baseq3State != setup.StateCurrent || mpState != setup.StateCurrent || nowAnnouncerState != setup.StateCurrent) || *force {
			plan.Say("Installing the trinity mod %s…", targetMod)
			if _, err := setup.InstallMod(plan, targetMod, quake3Dir, uid, gid); err != nil {
				fmt.Fprintf(os.Stderr, "Error installing mod: %v\n", err)
				os.Exit(1)
			}
		} else {
			plan.Say("The engine bundle already includes mod %s, so the mod overlay is skipped.", targetMod)
		}
	}

	if restartTrinity {
		plan.Say("Starting trinity.service…")
		if err := plan.Systemctl("start", "trinity.service"); err != nil {
			fmt.Fprintf(os.Stderr, "Error starting trinity.service: %v\n", err)
			os.Exit(1)
		}
	}
	if restartQ3 {
		plan.Say("Starting quake3-servers.target…")
		if err := plan.Systemctl("start", "quake3-servers.target"); err != nil {
			fmt.Fprintf(os.Stderr, "Error starting the Quake III servers: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Update complete.")
}

// installTrackerBinary renames the new binary into place: writing over
// the running executable fails with ETXTBSY, while a rename leaves this
// process on the old inode and the next start on the new one.
func installTrackerBinary(srcBin string) error {
	const dest = "/usr/local/bin/trinity"
	stage := filepath.Join(filepath.Dir(dest), ".trinity.new")
	in, err := os.Open(srcBin)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(stage, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		_ = os.Remove(stage)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(stage)
		return err
	}
	if err := os.Chmod(stage, 0755); err != nil {
		_ = os.Remove(stage)
		return err
	}
	if err := os.Rename(stage, dest); err != nil {
		_ = os.Remove(stage)
		return err
	}
	return nil
}

// row is one line of the update table. Several rows can share one decision.
type row struct {
	name       string
	current    string
	latest     string
	action     string
	state      setup.VersionState
	defaultYes bool
}

// decision is one approval. The mod's rows share one, since InstallMod
// lays down every pak from a single release.
type decision struct {
	name       string // "tracker" / "engine" / "mod"
	prompt     string
	action     string // matches the row's action so we can short-circuit "current"
	defaultYes bool
}

func makeRow(name, current, latest string, force bool) row {
	state := setup.CompareVersions(current, latest)
	r := row{
		name:    name,
		current: orUnknown(current),
		latest:  latest,
		state:   state,
	}
	switch state {
	case setup.StateCurrent:
		if force {
			r.action = "reinstall"
			r.defaultYes = true
		} else {
			r.action = "current"
			r.defaultYes = false
		}
	case setup.StateBehind:
		r.action = "upgrade"
		r.defaultYes = true
	case setup.StateDiverged:
		r.action = "diverged"
		r.defaultYes = false
	case setup.StateUnknown:
		r.action = "missing"
		r.defaultYes = false
	}
	return r
}

func shortHash(h string) string {
	if len(h) > 8 {
		return h[:8]
	}
	return h
}

func makeAnnouncerRow(state setup.VersionState, current, latest string, force bool) row {
	r := row{
		name:   "mod (announcer)",
		latest: latest,
		state:  state,
	}
	switch state {
	case setup.StateCurrent:
		r.current = orUnknown(current)
		if force {
			r.action = "reinstall"
			r.defaultYes = true
		} else {
			r.action = "current"
		}
	case setup.StateBehind:
		r.current = orUnknown(current)
		r.action = "upgrade"
		r.defaultYes = true
	case setup.StateUnknown:
		r.current = "(missing)"
		r.action = "missing"
	}
	return r
}

func rowToDecision(r row) decision {
	return decision{
		name:       strings.SplitN(r.name, " ", 2)[0], // "tracker" / "engine"
		prompt:     fmt.Sprintf("%s: %s → %s; %s?", r.name, r.current, r.latest, r.action),
		action:     r.action,
		defaultYes: r.defaultYes,
	}
}

// mergeModDecision prompts with the mod component that most needs
// attention.
func mergeModDecision(modRows []row, latest string, force bool) decision {
	worst := modRows[0]
	for _, r := range modRows[1:] {
		if modPriority(r.state) > modPriority(worst.state) {
			worst = r
		}
	}
	d := decision{
		name:       "mod",
		action:     worst.action,
		defaultYes: worst.defaultYes,
	}
	if force && worst.state == setup.StateCurrent {
		d.action = "reinstall"
		d.defaultYes = true
	}
	var parts []string
	for _, r := range modRows {
		short := strings.TrimSuffix(strings.TrimPrefix(r.name, "mod ("), ")")
		parts = append(parts, fmt.Sprintf("%s %s", short, r.current))
	}
	d.prompt = fmt.Sprintf("mod: %s → %s; %s?",
		strings.Join(parts, ", "), latest, d.action)
	return d
}

// modPriority ranks a custom build (Diverged) above an upgrade, then a
// missing pak, then Current.
func modPriority(s setup.VersionState) int {
	switch s {
	case setup.StateDiverged:
		return 3
	case setup.StateBehind:
		return 2
	case setup.StateUnknown:
		return 1
	default:
		return 0
	}
}

func orUnknown(s string) string {
	if s == "" {
		return "(unknown)"
	}
	return s
}

func printDecisionTable(rows []row) {
	tw := tabwriter.NewWriter(os.Stderr, 2, 0, 3, ' ', 0)
	fmt.Fprintln(tw, "Component\tCurrent\tLatest\tState")
	for _, r := range rows {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.name, r.current, r.latest, r.action)
	}
	tw.Flush()
}

// acquireUpdateLock fails with EWOULDBLOCK while another update holds
// the lock or a crashed one left it.
func acquireUpdateLock() (release func(), err error) {
	if err := os.MkdirAll(filepath.Dir(updateLockPath), 0755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(updateLockPath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}

type sysUser struct{ uid, gid int }

func userLookup(name string) (sysUser, error) {
	u, err := user.Lookup(name)
	if err != nil {
		return sysUser{}, err
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return sysUser{}, fmt.Errorf("parsing uid %q: %w", u.Uid, err)
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return sysUser{}, fmt.Errorf("parsing gid %q: %w", u.Gid, err)
	}
	return sysUser{uid: uid, gid: gid}, nil
}
