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
	// patchZipURL is the canonical bundle ioquake3.org distributes
	// (~26 MB). Single zip with `quake3-latest-pk3s/{baseq3,missionpack}/pakN.pk3`
	// inside, so the strip-1 unpack lands files in the right places.
	patchZipURL = "https://files.ioquake3.org/quake3-latest-pk3s.zip"

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
	UseSystemd  bool // controls whether the auto-start prompt fires
	TrinityBin  string // path to the installed trinity binary; defaults to /usr/local/bin/trinity
	Out         io.Writer
	Prompter    Prompter
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
	fmt.Fprintln(out, "  run correctly. The ioquake3 project hosts them free of charge under")
	fmt.Fprintln(out, "  the id Software EULA, reproduced on the next screen.")
	fmt.Fprintln(out)
	// Gate the pager — `more` scrolls/clears the screen and the lead-in
	// above is the only context the operator gets that this isn't a
	// random terminal-spam event.
	_ = opts.Prompter.Pause("  Press Enter to view the EULA: ")

	if err := pageEULA(); err != nil {
		fmt.Fprintf(out, "  (failed to show EULA via `more`: %v)\n", err)
		fmt.Fprintln(out, "  EULA is published at https://ioquake3.org/extras/patch-data/")
	}

	agree, err := opts.Prompter.YesNo("  I have read and agree to the license above. Download patches?", false)
	if err != nil || !agree {
		fmt.Fprintln(out, "  Skipping patch download.")
		fmt.Fprintln(out, "  Manual install: https://ioquake3.org/extras/patch-data/")
		return
	}

	fmt.Fprintf(out, "  Downloading %s ...\n", patchZipURL)
	if err := downloadAndExtractPatches(opts.Quake3Dir, opts.ServiceUser); err != nil {
		fmt.Fprintf(out, "  WARN: patch download failed: %v\n", err)
		fmt.Fprintln(out, "  Manual install: https://ioquake3.org/extras/patch-data/")
		return
	}
	fmt.Fprintln(out, "  Patches installed.")
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

// downloadAndExtractPatches pulls the canonical patch zip from
// ioquake3.org, walks its entries, and writes the pakN.pk3 files into
// baseq3/ and missionpack/ — both unconditionally so an operator who
// later adds a TA server isn't left without patches. Existing files
// are left alone; we only fill gaps, never overwrite operator copies.
func downloadAndExtractPatches(quake3Dir, serviceUser string) error {
	tmp, err := os.CreateTemp("", "trinity-patches-*.zip")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	resp, err := http.Get(patchZipURL)
	if err != nil {
		tmp.Close()
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		tmp.Close()
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, patchZipURL)
	}
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	zr, err := zip.OpenReader(tmpPath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer zr.Close()

	for _, entry := range zr.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		// Strip leading directory ("quake3-latest-pk3s/"): we only care
		// about whatever's after the first slash.
		name := entry.Name
		if i := strings.IndexByte(name, '/'); i >= 0 {
			name = name[i+1:]
		}
		// Expect "baseq3/pakN.pk3" or "missionpack/pakN.pk3".
		parts := strings.SplitN(name, "/", 2)
		if len(parts) != 2 {
			continue
		}
		mod, file := parts[0], parts[1]
		if mod != "baseq3" && mod != "missionpack" {
			continue
		}
		if !strings.HasPrefix(file, "pak") || !strings.HasSuffix(file, ".pk3") {
			continue
		}
		dest := filepath.Join(quake3Dir, mod, file)
		if _, err := os.Stat(dest); err == nil {
			// Don't overwrite an existing file — operator may have a
			// hand-curated copy or a custom patch.
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return err
		}
		if err := extractZipEntry(entry, dest); err != nil {
			return fmt.Errorf("extract %s: %w", file, err)
		}
		chownByName(dest, serviceUser)
	}
	return nil
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
