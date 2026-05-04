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

// cmdHelper is the privilege-dropped child re-exec'd by trinity init
// and trinity update so write-side work happens with the service
// user's credentials. Hidden from --help; only invoked via Plan's
// WriteFileAs / CopyTreeAs / DownloadAs / UnzipAs.
//
// Verbs:
//
//	_helper write    <path> <mode>             (reads stdin)
//	_helper copy     <src> <dst>
//	_helper download <url> <dest>
//	_helper unzip    <zip> <dest> <strip-components>
//
// Pure Go inside the child — no shelling to coreutils — so we share
// one error-handling style and one set of tests across both
// privileged and dropped paths. Re-exec target is /proc/self/exe, so
// the binary swap mid-update is invisible to in-flight helpers.
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
	// Force mode explicitly — OpenFile honors umask, which the
	// caller can't predict.
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

// cmdUpdate refreshes the tracker binary, hub web bundle, engine, and
// mod against the latest GitHub releases. Each component is checked
// against its currently-installed version and skipped when already
// current; the whole thing is a no-op (no service restart) when
// nothing is behind.
//
// Steps run in this order, all gated on a flock so a second invocation
// bails cleanly: pre-flight + checksum-verify everything, confirm,
// stop services, atomic-rename binary, web overlay, engine extract,
// mod overlay (if still behind after engine), start services.
func cmdUpdate(args []string) {
	fs := flag.NewFlagSet("update", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to config file")
	check := fs.Bool("check", false, "print versions and exit (0 if all current, 1 if any behind)")
	dryRun := fs.Bool("dry-run", false, "stage and verify downloads, print plan, no install / no service restart")
	yes := fs.Bool("yes", false, "skip the apply confirmation prompt")
	noRestart := fs.Bool("no-restart", false, "install bits but don't touch systemd")
	force := fs.Bool("force", false, "re-install even when current matches latest (no silent downgrade)")
	trackerTag := fs.String("tracker-tag", "", "pin a specific trinity-tracker tag (default: latest)")
	engineTag := fs.String("engine-tag", "", "pin a specific trinity-engine tag (default: latest)")
	modTag := fs.String("mod-tag", "", "pin a specific trinity (mod) tag (default: latest)")
	fs.Parse(args)

	// Dry-run is a preview; everything else needs root for writing
	// /usr/local/bin/trinity, the static dir, and Quake3Dir.
	if !*dryRun && !*check && os.Getuid() != 0 {
		fmt.Fprintln(os.Stderr, "Error: trinity update must be run as root (or use --dry-run / --check).")
		os.Exit(1)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Lock first — concurrent updates corrupt state. Print the path
	// so the operator can clear a stale lock if a previous invocation
	// crashed without releasing.
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

	// Detect roles + relevant paths from the config.
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

	// Resolve target tags.
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

	// Detect current versions.
	currentTracker := version
	var currentEngine, currentBaseq3, currentMissionpack string
	if hasLocalServers {
		currentEngine, _ = setup.DetectEngineVersion(quake3Dir)
		currentBaseq3, _ = setup.DetectModVersion(quake3Dir, "baseq3")
		currentMissionpack, _ = setup.DetectModVersion(quake3Dir, "missionpack")
	}

	// Build the displayed table — 4 rows for visibility (mod is shown
	// per subdir so an operator sees baseq3/missionpack discrepancies).
	var rows []row
	rows = append(rows, makeRow("tracker", currentTracker, targetTracker, *force))
	if hasLocalServers {
		rows = append(rows, makeRow("engine", currentEngine, targetEngine, *force))
		rows = append(rows, makeRow("mod (baseq3)", currentBaseq3, targetMod, *force))
		rows = append(rows, makeRow("mod (missionpack)", currentMissionpack, targetMod, *force))
	}
	printDecisionTable(rows)

	// Decisions are coarser than rows: tracker, engine, mod. Mod is
	// one release that lays down both paks, so baseq3/missionpack
	// share a single approval. We pick the most-divergent state of
	// the two for the prompt's default (StateDiverged > StateBehind >
	// StateUnknown > StateCurrent in "needs operator attention" terms).
	decisions := []decision{rowToDecision(rows[0])}
	if hasLocalServers {
		decisions = append(decisions, rowToDecision(rows[1])) // engine
		decisions = append(decisions, mergeModDecision(rows[2], rows[3], targetMod, *force))
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
		fmt.Fprintln(os.Stderr, "Everything up to date — nothing to do.")
		return
	}

	// Per-decision approval. --yes accepts all; --dry-run also
	// implicitly approves all so the staging step exercises every
	// download path. Without either, we prompt per decision with the
	// state-derived default (StateBehind → Y; StateDiverged/StateUnknown
	// → N, so press-enter-through doesn't silently overwrite a custom
	// build).
	if !*yes && !*dryRun {
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Each component below can be applied or skipped independently.")
		fmt.Fprintln(os.Stderr, "Approving any component will stop and restart the affected service(s)")
		fmt.Fprintln(os.Stderr, "(trinity.service, plus quake3-servers.target if engine/mod changes).")
		fmt.Fprintln(os.Stderr, "Active players on a restarted q3 server will be disconnected.")
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
			fmt.Fprintf(os.Stderr, "Read confirm: %v\n", err)
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
		fmt.Fprintln(os.Stderr, "Nothing approved — no changes made.")
		return
	}

	plan := &setup.Plan{
		DryRun:     *dryRun,
		UseSystemd: useSd,
		Out:        os.Stderr,
	}

	// Stage everything before touching services. If any download or
	// checksum fails we abort with the install untouched. mktemp -d
	// defaults to mode 0700 — open it up to 0755 so the service-user
	// AsUser cp -rT for the web overlay can traverse and read.
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
		fmt.Fprintln(os.Stderr, "Staging tracker release ...")
		trackerStage, err = setup.DownloadTrackerRelease(targetTracker, stageDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error staging tracker: %v\n", err)
			os.Exit(1)
		}
	}

	if *dryRun {
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Dry run complete — staged + verified downloads, no install performed.")
		return
	}

	// Apply phase. From here on, partial failure leaves services
	// stopped; operator re-runs `trinity update` after fixing the
	// underlying issue (each step is idempotent, so re-runs converge).
	if !*noRestart {
		if hasLocalServers {
			plan.Say("Stopping quake3-servers.target ...")
			if err := plan.Systemctl("stop", "quake3-servers.target"); err != nil {
				fmt.Fprintf(os.Stderr, "Error stopping q3 servers: %v\n", err)
				os.Exit(1)
			}
		}
		plan.Say("Stopping trinity.service ...")
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
			plan.Say("Overlaying web bundle into %s as %s ...", staticDir, svcUser)
			if err := plan.CopyTreeAs(uid, gid, trackerStage.WebDir, staticDir); err != nil {
				fmt.Fprintf(os.Stderr, "Error overlaying web bundle: %v\n", err)
				os.Exit(1)
			}
		}
	}

	if hasLocalServers && approve["engine"] {
		plan.Say("Installing trinity-engine %s ...", targetEngine)
		if _, err := setup.InstallEngine(plan, targetEngine, quake3Dir, "/var/log/quake3", staticDir, uid, gid); err != nil {
			fmt.Fprintf(os.Stderr, "Error installing engine: %v\n", err)
			os.Exit(1)
		}
	}

	// After engine install the bundled mod may already satisfy the
	// mod check. Re-detect to avoid re-fetching when the bundle
	// covers it. Only runs when mod was explicitly approved — an
	// engine-only upgrade leaves the mod alone.
	if hasLocalServers && approve["mod"] {
		nowBaseq3, _ := setup.DetectModVersion(quake3Dir, "baseq3")
		nowMissionpack, _ := setup.DetectModVersion(quake3Dir, "missionpack")
		baseq3State := setup.CompareVersions(nowBaseq3, targetMod)
		mpState := setup.CompareVersions(nowMissionpack, targetMod)
		if (baseq3State != setup.StateCurrent || mpState != setup.StateCurrent) || *force {
			plan.Say("Installing trinity mod %s ...", targetMod)
			if _, err := setup.InstallMod(plan, targetMod, quake3Dir, uid, gid); err != nil {
				fmt.Fprintf(os.Stderr, "Error installing mod: %v\n", err)
				os.Exit(1)
			}
		} else {
			plan.Say("Engine bundle already satisfies mod %s — skipping mod overlay.", targetMod)
		}
	}

	if !*noRestart {
		plan.Say("Starting trinity.service ...")
		if err := plan.Systemctl("start", "trinity.service"); err != nil {
			fmt.Fprintf(os.Stderr, "Error starting trinity.service: %v\n", err)
			os.Exit(1)
		}
		if hasLocalServers {
			plan.Say("Starting quake3-servers.target ...")
			if err := plan.Systemctl("start", "quake3-servers.target"); err != nil {
				fmt.Fprintf(os.Stderr, "Error starting q3 servers: %v\n", err)
				os.Exit(1)
			}
		}
	}

	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Update complete.")
}

// installTrackerBinary atomically swaps /usr/local/bin/trinity. cp /
// install would hit ETXTBSY because we (the running update process)
// hold the binary as our own executable; rename swaps the inode while
// our process keeps the old one until exit, and the next systemctl
// start picks up the new inode.
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

// row is a display entry in the decision table — one line per
// detectable component (mod is split into baseq3 / missionpack so the
// operator sees per-subdir state). state and defaultYes drive the
// per-decision prompt; the prompt itself is owned by the coarser
// `decision` type so two display rows can share one approval (mod).
type row struct {
	name       string
	current    string
	latest     string
	action     string
	state      setup.VersionState
	defaultYes bool
}

// decision is one approval the operator makes per component. tracker
// and engine map 1:1 with rows; mod merges the baseq3 + missionpack
// rows because InstallMod lays down both paks as a single release.
type decision struct {
	name       string // "tracker" / "engine" / "mod"
	prompt     string
	action     string // matches the row's action so we can short-circuit "current"
	defaultYes bool
}

// makeRow builds a display row for a single (current, latest) pair.
// State drives the action label and the default for the eventual
// prompt: Behind → "upgrade" / default Y; Diverged → "diverged" /
// default N; Unknown → "missing" / default N. --force flips Current
// to "reinstall" with default Y.
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

// rowToDecision lifts a single display row into a decision (used for
// tracker and engine; mod uses mergeModDecision instead).
func rowToDecision(r row) decision {
	return decision{
		name:       strings.SplitN(r.name, " ", 2)[0], // "tracker" / "engine"
		prompt:     fmt.Sprintf("%s: %s → %s — %s?", r.name, r.current, r.latest, r.action),
		action:     r.action,
		defaultYes: r.defaultYes,
	}
}

// mergeModDecision collapses the baseq3 + missionpack display rows
// into a single mod decision. We pick whichever side most needs
// attention (Diverged > Behind > Unknown > Current) so the operator
// sees the "worst case" in the prompt; the mod release lays down
// both paks regardless of which one was the trigger.
func mergeModDecision(b, mp row, latest string, force bool) decision {
	pick := func(a, b row) row {
		if modPriority(a.state) >= modPriority(b.state) {
			return a
		}
		return b
	}
	worst := pick(b, mp)
	d := decision{
		name:       "mod",
		action:     worst.action,
		defaultYes: worst.defaultYes,
	}
	if force && worst.state == setup.StateCurrent {
		d.action = "reinstall"
		d.defaultYes = true
	}
	d.prompt = fmt.Sprintf("mod: baseq3 %s, missionpack %s → %s — %s?",
		b.current, mp.current, latest, d.action)
	return d
}

// modPriority orders states by "needs operator attention". Diverged
// (custom build) is the loudest signal, then Behind (clean upgrade),
// then Unknown (missing pak), then Current.
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

// acquireUpdateLock takes an exclusive flock on updateLockPath,
// returning a release func. EWOULDBLOCK means another update is
// running (or the lock leaked); the caller surfaces the path so the
// operator can clear it manually if needed.
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
