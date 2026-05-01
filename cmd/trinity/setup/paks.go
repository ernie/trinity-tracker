package setup

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// requiredBaseq3 is the pak set the trinity-engine fork expects in
// baseq3/. pak0 is the retail asset (not redistributable); pak1–pak8
// are the id Software 1.32 point release patch data, redistributed
// under the EULA reproduced below.
var requiredBaseq3 = []string{
	"pak0.pk3",
	"pak1.pk3", "pak2.pk3", "pak3.pk3", "pak4.pk3",
	"pak5.pk3", "pak6.pk3", "pak7.pk3", "pak8.pk3",
}

// requiredMissionpack is the same idea for missionpack/, only checked
// when an operator has at least one Team Arena server configured.
var requiredMissionpack = []string{
	"pak0.pk3",
	"pak1.pk3", "pak2.pk3", "pak3.pk3",
}

const (
	// patchZipName is the file the wizard fetches for the 1.32 point
	// release patch data. Hub operators drop the (re-zipped) ioquake3.org
	// bundle into <static_dir>/downloads/ once at hub setup time; every
	// install fetches it from there. Zip layout: `baseq3/pakN.pk3` and
	// `missionpack/pakN.pk3` at the top level.
	patchZipName = "quake3-1.32-pk3s.zip"

	// hqqBaseq3ZipName / hqqMPZipName are the optional High Quality
	// Quake assets the wizard offers after patches. Same layout: top-
	// level `baseq3/...` or `missionpack/...` directories.
	hqqBaseq3ZipName = "hqq-baseq3.zip"
	hqqMPZipName     = "hqq-missionpack.zip"

	// canonBaseq3 / canonMP are the filenames the prereq/install banner
	// suggests the operator copy their retail pak0s as. When present in
	// the wizard's CWD they're auto-staged without a prompt.
	canonBaseq3 = "q3-pak0.pk3"
	canonMP     = "mp-pak0.pk3"
)

// PakStepOptions controls the post-apply pak placement step.
type PakStepOptions struct {
	Quake3Dir   string
	ServiceUser string
	UseSystemd  bool   // controls whether the auto-start prompt fires
	TrinityBin  string // path to the installed trinity binary; defaults to /usr/local/bin/trinity

	// HubHost is the hostname the wizard uses to fetch hub-hosted
	// downloads (patch zip, HQQ assets) over HTTPS when no local copy
	// is found. Required for collector-only installs that have to pull
	// from a remote hub.
	HubHost string

	// StaticDir is the hub's web-asset root (e.g. /var/lib/trinity/web).
	// On a hub install, zips found via Cwd are mirrored into
	// StaticDir/downloads/ so future collectors can fetch them from
	// this hub. Empty on collector-only installs.
	StaticDir string

	// Cwd is the directory `trinity init` was launched from. The wizard
	// looks here first for operator-supplied zips (patch bundle, HQQ),
	// matching the pak0 staging pattern. This is how a first hub install
	// bootstraps before any other hub exists to fetch from.
	Cwd string

	Out      io.Writer
	Prompter Prompter
}

// PakStepResult tells the caller what happened so it can adjust the
// trailing "Next steps:" output (e.g. skip the start-command line if
// services were already started here).
type PakStepResult struct {
	AllReady bool
	Baked    bool // levelshots + demobake were run
	Started  bool
}

// RunPakStep performs the post-apply pak placement and optional
// auto-start. It's best-effort UX polish — every failure path falls
// back to telling the operator what to do manually, and the wizard
// still considers itself successful. Returns information the caller
// uses to decide what (if anything) to print as remaining steps.
func RunPakStep(opts PakStepOptions) PakStepResult {
	result := PakStepResult{}
	out := opts.Out
	if out == nil {
		out = os.Stderr
	}
	cwd, _ := os.Getwd()

	fmt.Fprintln(out)
	fmt.Fprintln(out, "Pak files:")
	missingBaseq3 := missingPaks(opts.Quake3Dir, "baseq3", requiredBaseq3)
	missingMP := missingPaks(opts.Quake3Dir, "missionpack", requiredMissionpack)

	if len(missingBaseq3) == 0 && len(missingMP) == 0 {
		fmt.Fprintln(out, "  All required pak files are already in place.")
		result.AllReady = true
		offerHQQ(opts, out)
		result.Baked = runBake(opts, out)
		result.Started = maybeAutoStart(opts, out)
		return result
	}

	// pak0s first — we can't fetch these from anywhere, so the operator
	// has to provide them. Both gametypes are offered unconditionally:
	// adding a TA server later shouldn't require re-running the wizard
	// just to drop a pak0 in.
	if contains(missingBaseq3, "pak0.pk3") {
		stagePak0(opts, out, cwd, "baseq3", canonBaseq3, "retail Quake 3")
	}
	if contains(missingMP, "pak0.pk3") {
		stagePak0(opts, out, cwd, "missionpack", canonMP, "Team Arena")
	}

	// Re-check what's still missing after pak0 staging — the patch zip
	// only matters if any of pak1+ are absent.
	missingBaseq3 = missingPaks(opts.Quake3Dir, "baseq3", requiredBaseq3)
	missingMP = missingPaks(opts.Quake3Dir, "missionpack", requiredMissionpack)
	patchesMissing := anyPatchMissing(missingBaseq3) || anyPatchMissing(missingMP)
	if patchesMissing {
		offerPatchDownload(opts, out)
		// Re-stat one more time so the auto-start gate sees fresh state.
		missingBaseq3 = missingPaks(opts.Quake3Dir, "baseq3", requiredBaseq3)
		missingMP = missingPaks(opts.Quake3Dir, "missionpack", requiredMissionpack)
	}

	// Final status.
	if len(missingBaseq3) == 0 && len(missingMP) == 0 {
		fmt.Fprintln(out, "  All required pak files are now in place.")
		result.AllReady = true
		offerHQQ(opts, out)
		result.Baked = runBake(opts, out)
		result.Started = maybeAutoStart(opts, out)
		return result
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "  Still missing:")
	for _, f := range missingBaseq3 {
		fmt.Fprintf(out, "    %s\n", filepath.Join(opts.Quake3Dir, "baseq3", f))
	}
	for _, f := range missingMP {
		fmt.Fprintf(out, "    %s\n", filepath.Join(opts.Quake3Dir, "missionpack", f))
	}
	fmt.Fprintln(out, "  Place these manually before starting the q3 servers.")
	return result
}

// missingPaks returns the subset of names that aren't present in
// quake3Dir/mod. A non-existent mod directory simply means everything
// is missing — the engine install step creates it, but a manual
// install or a re-run after rm could miss it.
func missingPaks(quake3Dir, mod string, names []string) []string {
	var out []string
	for _, n := range names {
		p := filepath.Join(quake3Dir, mod, n)
		if _, err := os.Stat(p); err != nil {
			out = append(out, n)
		}
	}
	return out
}

func anyPatchMissing(missing []string) bool {
	for _, m := range missing {
		if m != "pak0.pk3" {
			return true
		}
	}
	return false
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// stagePak0 places a pak0.pk3 into quake3Dir/mod. First tries the
// canonical filename the install banner asked the operator to drop in
// CWD (silent if found); falls back to a path prompt that accepts any
// filename anywhere on the filesystem. Blank answer = skip and let
// the operator copy it manually later.
func stagePak0(opts PakStepOptions, out io.Writer, cwd, mod, canonName, label string) {
	dest := filepath.Join(opts.Quake3Dir, mod, "pak0.pk3")

	// Canonical-named candidate in CWD: zero-prompt path.
	if cwd != "" {
		candidate := filepath.Join(cwd, canonName)
		if isPlausiblePak(candidate) {
			if err := installPakFile(candidate, dest, opts.ServiceUser); err != nil {
				fmt.Fprintf(out, "  WARN: failed to copy %s → %s: %v\n", candidate, dest, err)
				return
			}
			fmt.Fprintf(out, "  Copied %s → %s\n", canonName, dest)
			return
		}
	}

	// Operator-supplied path.
	for {
		prompt := fmt.Sprintf("  Path to %s pak0.pk3 (blank to skip)", label)
		path, err := opts.Prompter.Optional(prompt, "")
		if err != nil {
			fmt.Fprintf(out, "  (skipping %s pak0)\n", mod)
			return
		}
		path = strings.TrimSpace(path)
		if path == "" {
			fmt.Fprintf(out, "  (skipped — drop pak0.pk3 at %s before starting the server)\n", dest)
			return
		}
		// Expand ~ for convenience; not bothering with $VAR — operators
		// don't expect shell expansion in interactive prompts.
		if strings.HasPrefix(path, "~/") {
			if home, herr := os.UserHomeDir(); herr == nil {
				path = filepath.Join(home, path[2:])
			}
		}
		info, err := os.Stat(path)
		if err != nil {
			fmt.Fprintf(out, "    cannot read %s: %v\n", path, err)
			continue
		}
		if info.IsDir() {
			fmt.Fprintf(out, "    %s is a directory, not a pk3 file\n", path)
			continue
		}
		if !isPlausiblePak(path) {
			fmt.Fprintf(out, "    %s does not look like a pk3 (zip) file\n", path)
			continue
		}
		if filepath.Base(path) != "pak0.pk3" {
			fmt.Fprintf(out, "    Will copy as pak0.pk3 (your file is named %q).\n", filepath.Base(path))
		}
		if err := installPakFile(path, dest, opts.ServiceUser); err != nil {
			fmt.Fprintf(out, "    failed to copy %s → %s: %v\n", path, dest, err)
			continue
		}
		fmt.Fprintf(out, "  Copied %s → %s\n", path, dest)
		return
	}
}

// isPlausiblePak does a cheap magic-number check (PK\x03\x04, the zip
// local file header). Stops the operator from typing the path to a
// random text file and finding out only when the q3 server fails to
// boot. Doesn't validate the contents — id's pak0 is just a zip.
func isPlausiblePak(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	var buf [4]byte
	if _, err := io.ReadFull(f, buf[:]); err != nil {
		return false
	}
	return buf[0] == 'P' && buf[1] == 'K' && buf[2] == 0x03 && buf[3] == 0x04
}

// installPakFile copies src into dest (atomically, via temp + rename),
// 0644, owned by the service user. mod parent dir is created if absent.
func installPakFile(src, dest, serviceUser string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".pak0-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }
	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Chmod(0644); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpPath, dest); err != nil {
		cleanup()
		return err
	}
	chownByName(dest, serviceUser)
	return nil
}

// chownByName changes ownership to user:user using the system's
// `chown` command — saves us a uid/gid lookup and matches what the
// rest of the apply phase does.
func chownByName(path, user string) {
	if user == "" {
		return
	}
	cmd := exec.Command("chown", user+":"+user, path)
	_ = cmd.Run()
}

// offerPatchDownload runs the EULA flow + zip fetch + extract for the
// id Software 1.32 point-release pk3s. Always returns; failures are
// reported to the operator and the wizard moves on to the auto-start
// gate (which then sees the patches still missing and skips itself).
func offerPatchDownload(opts PakStepOptions, out io.Writer) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, "  The Quake 3 1.32 point-release patches are required for the server to")
	fmt.Fprintln(out, "  run correctly. They are distributed under the id Software EULA,")
	fmt.Fprintln(out, "  reproduced on the next screen.")
	fmt.Fprintln(out)
	// Gate the pager — `more` scrolls/clears the screen and the lead-in
	// above is the only context the operator gets that this isn't a
	// random terminal-spam event.
	_ = opts.Prompter.Pause("  Press Enter to view the EULA: ")

	if err := pageEULA(); err != nil {
		fmt.Fprintf(out, "  (failed to show EULA via `more`: %v)\n", err)
		if url := hubURL(opts.HubHost, "/quake3-eula"); url != "" {
			fmt.Fprintf(out, "  EULA is also available at %s\n", url)
		}
	}

	agree, err := opts.Prompter.YesNo("  I have read and agree to the license above. Download patches?", false)
	if err != nil || !agree {
		fmt.Fprintln(out, "  Skipping patch download.")
		printManualInstall(out, opts, patchZipName)
		return
	}

	if err := fetchAndExtractMods(opts, out, patchZipName, []string{"baseq3", "missionpack"}); err != nil {
		fmt.Fprintf(out, "  WARN: patch download failed: %v\n", err)
		printManualInstall(out, opts, patchZipName)
		return
	}
	fmt.Fprintln(out, "  Patches installed.")
}

// offerHQQ optionally installs High Quality Quake — a community asset
// pack that ships sharper levelshots and player portraits than the
// stock retail pk3s. Runs after patches and before bake so the
// `levelshots` / `demobake` steps see the higher-quality textures.
//
// HQQ baseq3 is offered always; HQQ TA is offered only when the
// missionpack/pak0.pk3 retail asset is in place (no point installing
// TA assets for an operator who hasn't supplied a TA pak0).
//
// Optional, non-fatal: any error or skip just logs and moves on.
func offerHQQ(opts PakStepOptions, out io.Writer) {
	mpInstalled := fileExists(filepath.Join(opts.Quake3Dir, "missionpack", "pak0.pk3"))

	fmt.Fprintln(out)
	fmt.Fprintln(out, "  High Quality Quake (HQQ) is an optional community asset pack with")
	fmt.Fprintln(out, "  sharper levelshots and player portraits than the stock pk3s. Trinity")
	fmt.Fprintln(out, "  uses these for the hub UI's map and player thumbnails.")
	want, err := opts.Prompter.YesNo("  Install High Quality Quake assets?", true)
	if err != nil || !want {
		fmt.Fprintln(out, "  Skipping HQQ.")
		return
	}

	if err := fetchAndExtractMods(opts, out, hqqBaseq3ZipName, []string{"baseq3"}); err != nil {
		fmt.Fprintf(out, "  WARN: HQQ baseq3 download failed: %v\n", err)
		printManualInstall(out, opts, hqqBaseq3ZipName)
	}
	if mpInstalled {
		if err := fetchAndExtractMods(opts, out, hqqMPZipName, []string{"missionpack"}); err != nil {
			fmt.Fprintf(out, "  WARN: HQQ missionpack download failed: %v\n", err)
			printManualInstall(out, opts, hqqMPZipName)
		}
	}
}

// printManualInstall tells the operator how to recover when the
// automated fetch couldn't run — either by dropping the zip in the
// install dir on a future re-run, or by extracting it manually.
func printManualInstall(out io.Writer, opts PakStepOptions, name string) {
	fmt.Fprintf(out, "  Manual install: drop %s into the install directory and re-run trinity init,\n", name)
	fmt.Fprintf(out, "  or extract it manually into %s\n", opts.Quake3Dir)
	if url := hubURL(opts.HubHost, "/downloads/"+name); url != "" && opts.StaticDir == "" {
		fmt.Fprintf(out, "  (the hub serves it at %s).\n", url)
	}
}

// hubURL builds an https://<host><path> URL when host is non-empty,
// returning "" when there's no configured hub (which would mean the
// wizard is running outside any expected mode — caller should handle
// the empty case).
func hubURL(host, path string) string {
	if host == "" {
		return ""
	}
	return "https://" + host + path
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// pageEULA pipes the embedded EULA text through `more` so the operator
// has to scroll through it. `more` ships with util-linux on every
// supported distro; if it's somehow missing, fall back to a plain
// print + "press enter when done" so the wizard still works.
func pageEULA() error {
	if _, err := exec.LookPath("more"); err != nil {
		fmt.Print(quake3EULA)
		fmt.Print("\nPress Enter when done reading: ")
		var line string
		_, _ = fmt.Scanln(&line)
		return nil
	}
	cmd := exec.Command("more")
	cmd.Stdin = strings.NewReader(quake3EULA)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// fetchAndExtractMods locates the named zip (preferring a local copy
// under StaticDir/downloads/, otherwise fetching from the hub over
// HTTPS) and extracts every entry whose top-level dir is one of the
// allowed mods into <Quake3Dir>/<mod>/<rest>. Existing files are left
// alone — we only fill gaps, never overwrite operator copies.
//
// Hub-hosted zips have `baseq3/...` and/or `missionpack/...` at the
// top level. Some upstream patch/HQQ zips wrap everything in a single
// release-name dir (e.g. `quake3-latest-pk3s/baseq3/...`); we strip
// that wrapper transparently. Entries outside the allowed mods (or
// that resolve outside their mod dir via "..") are skipped.
func fetchAndExtractMods(opts PakStepOptions, out io.Writer, name string, mods []string) error {
	zipPath, cleanup, err := resolveZip(opts, out, name)
	if err != nil {
		return err
	}
	defer cleanup()

	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer zr.Close()

	allowed := make(map[string]bool, len(mods))
	for _, m := range mods {
		allowed[m] = true
	}

	stripPrefix := detectWrapperDir(zr.File, allowed)

	for _, entry := range zr.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		entryName := strings.TrimPrefix(entry.Name, stripPrefix)
		mod, rest, ok := strings.Cut(entryName, "/")
		if !ok || !allowed[mod] || rest == "" {
			continue
		}
		modRoot := filepath.Join(opts.Quake3Dir, mod)
		dest := filepath.Join(modRoot, rest)
		// Zip-slip guard: filepath.Join + .. could escape the mod dir.
		if !strings.HasPrefix(dest, modRoot+string(os.PathSeparator)) {
			continue
		}
		if _, err := os.Stat(dest); err == nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return err
		}
		if err := extractZipEntry(entry, dest); err != nil {
			return fmt.Errorf("extract %s: %w", entry.Name, err)
		}
		chownByName(dest, opts.ServiceUser)
	}
	return nil
}

// detectWrapperDir returns "wrapper/" when every entry in the zip
// shares one non-allowed top-level dir (the typical patch/HQQ layout),
// or "" when entries already start with allowed mod names. Mixed
// layouts return "" — we'd rather extract nothing than partially.
func detectWrapperDir(files []*zip.File, allowed map[string]bool) string {
	tops := make(map[string]bool)
	for _, entry := range files {
		top, _, _ := strings.Cut(entry.Name, "/")
		if top == "" {
			continue
		}
		tops[top] = true
	}
	for top := range tops {
		if allowed[top] {
			return ""
		}
	}
	if len(tops) != 1 {
		return ""
	}
	for top := range tops {
		return top + "/"
	}
	return ""
}

// resolveZip returns a filesystem path to the named zip. Sources are
// tried in order:
//
//  1. opts.Cwd/<name> — operator-staged file in the install dir, the
//     same pattern as pak0 staging. Mirrored into StaticDir/downloads/
//     on a hub install so the hub serves it to future collectors.
//  2. opts.StaticDir/downloads/<name> — already-mirrored copy, common
//     on a hub re-run.
//  3. https://<HubHost>/downloads/<name> — collector pulling from a
//     remote hub. Downloaded into a tempfile.
//
// The cleanup func removes the tempfile when the source was downloaded;
// it's a no-op when the path was a local file we shouldn't touch.
func resolveZip(opts PakStepOptions, out io.Writer, name string) (string, func(), error) {
	if opts.Cwd != "" {
		staged := filepath.Join(opts.Cwd, name)
		if _, err := os.Stat(staged); err == nil {
			fmt.Fprintf(out, "  Using %s from current directory ...\n", name)
			mirrorToDownloads(staged, opts, out, name)
			return staged, func() {}, nil
		}
	}
	if opts.StaticDir != "" {
		local := filepath.Join(opts.StaticDir, "downloads", name)
		if _, err := os.Stat(local); err == nil {
			fmt.Fprintf(out, "  Using local %s ...\n", local)
			return local, func() {}, nil
		}
	}
	url := hubURL(opts.HubHost, "/downloads/"+name)
	if url == "" {
		return "", func() {}, fmt.Errorf("no source for %s — drop it into the current directory and re-run", name)
	}
	fmt.Fprintf(out, "  Downloading %s ...\n", url)
	tmp, err := os.CreateTemp("", "trinity-dl-*.zip")
	if err != nil {
		return "", func() {}, err
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }
	resp, err := http.Get(url)
	if err != nil {
		tmp.Close()
		cleanup()
		return "", func() {}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		tmp.Close()
		cleanup()
		return "", func() {}, fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		cleanup()
		return "", func() {}, err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return tmpPath, cleanup, nil
}

// mirrorToDownloads copies a CWD-staged zip into <StaticDir>/downloads/
// so the hub serves it to future collector installs. No-op when no
// StaticDir (collector install) or when a copy already exists.
func mirrorToDownloads(src string, opts PakStepOptions, out io.Writer, name string) {
	if opts.StaticDir == "" {
		return
	}
	dlDir := filepath.Join(opts.StaticDir, "downloads")
	dest := filepath.Join(dlDir, name)
	if _, err := os.Stat(dest); err == nil {
		return
	}
	if err := os.MkdirAll(dlDir, 0755); err != nil {
		fmt.Fprintf(out, "  WARN: could not create %s: %v\n", dlDir, err)
		return
	}
	chownByName(dlDir, opts.ServiceUser)
	if err := installPakFile(src, dest, opts.ServiceUser); err != nil {
		fmt.Fprintf(out, "  WARN: mirror %s → %s: %v\n", src, dest, err)
		return
	}
	fmt.Fprintf(out, "  Mirrored %s to %s\n", name, dlDir)
}

func extractZipEntry(entry *zip.File, dest string) error {
	in, err := entry.Open()
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".pak-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Chmod(0644); err != nil {
		tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, dest)
}

// runBake produces the levelshot images and demo-playback pk3s the
// hub serves to web viewers. Runs unconditionally when paks are in
// place — there's no reason to make the operator do this manually
// once we know the data files needed are present. Both subcommands
// run as the service user via runuser (util-linux, ships everywhere)
// so the assets they write are readable by the running trinity
// service. Returns true if at least one bake step finished cleanly;
// failures are reported and treated as non-fatal.
func runBake(opts PakStepOptions, out io.Writer) bool {
	bin := opts.TrinityBin
	if bin == "" {
		bin = "/usr/local/bin/trinity"
	}
	if _, err := os.Stat(bin); err != nil {
		return false
	}
	user := opts.ServiceUser
	if user == "" {
		user = "quake"
	}
	any := false
	for _, sub := range []string{"levelshots", "demobake"} {
		fmt.Fprintf(out, "  Running trinity %s ...\n", sub)
		cmd := exec.Command("runuser", "-u", user, "--", bin, sub)
		cmd.Stdout = out
		cmd.Stderr = out
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(out, "  WARN: trinity %s failed: %v\n", sub, err)
			fmt.Fprintf(out, "  Re-run later: sudo -u %s %s %s\n", user, bin, sub)
			continue
		}
		any = true
	}
	return any
}

// maybeAutoStart prompts to start trinity + quake3-servers.target. No-op
// when systemd isn't in play. Returns true when the start command ran
// (regardless of the operator's per-service success).
func maybeAutoStart(opts PakStepOptions, out io.Writer) bool {
	if !opts.UseSystemd {
		return false
	}
	confirm, err := opts.Prompter.YesNo("\nStart trinity and the q3 servers now?", true)
	if err != nil || !confirm {
		return false
	}
	args := []string{"start", "trinity.service", "quake3-servers.target"}
	cmd := exec.Command("systemctl", args...)
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(out, "  WARN: systemctl start failed: %v\n", err)
		fmt.Fprintln(out, "  Check `journalctl -u trinity -u 'quake3-server@*'` for details.")
		return true
	}
	fmt.Fprintln(out, "  Started: trinity.service, quake3-servers.target")
	return true
}
