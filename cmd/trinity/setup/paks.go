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

// requiredBaseq3 is the pak set the engine expects in baseq3/. pak0 is
// the retail game's and can't be redistributed; pak1–pak8 are id
// Software's 1.32 point release patch data, redistributed under its
// EULA.
var requiredBaseq3 = []string{
	"pak0.pk3",
	"pak1.pk3", "pak2.pk3", "pak3.pk3", "pak4.pk3",
	"pak5.pk3", "pak6.pk3", "pak7.pk3", "pak8.pk3",
}

// requiredMissionpack is the pak set for missionpack/, checked when at
// least one Team Arena server is configured.
var requiredMissionpack = []string{
	"pak0.pk3",
	"pak1.pk3", "pak2.pk3", "pak3.pk3",
}

const (
	// patchZipName is the 1.32 point release patch data, which the hub
	// serves from <static_dir>/downloads/ for every install to fetch.
	patchZipName = "quake3-1.32-pk3s.zip"

	// canonBaseq3 and canonMP are the names the install banner asks the
	// operator to give their retail pak0s. Found in the working
	// directory, they're placed without a prompt.
	canonBaseq3 = "q3-pak0.pk3"
	canonMP     = "mp-pak0.pk3"
)

// PakStepOptions controls the post-apply pak placement step.
type PakStepOptions struct {
	Quake3Dir   string
	ServiceUser string
	UseSystemd  bool   // controls whether the auto-start prompt fires
	TrinityBin  string // path to the installed trinity binary; defaults to /usr/local/bin/trinity

	// HubHost is the hub the patch zip is fetched from over HTTPS when
	// there's no local copy.
	HubHost string

	// StaticDir is the hub's web-asset root (e.g. /var/lib/trinity/web).
	// On a hub, zips found in Cwd are mirrored into StaticDir/downloads/
	// for collectors to fetch. Empty on collector-only installs.
	StaticDir string

	// Cwd is the directory `trinity init` was launched from, searched
	// first for an operator-supplied patch zip. That's how the first hub
	// gets the zip.
	Cwd string

	Out      io.Writer
	Prompter Prompter
}

// PakStepResult tells the caller which "Next steps" lines still apply.
type PakStepResult struct {
	AllReady bool
	Baked    bool // levelshots + demobake were run
	Started  bool
}

// RunPakStep places paks after Apply and offers to start the services.
// Every failure falls back to telling the operator what to do by hand,
// and the wizard still succeeds.
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

	// Only the operator can supply the pak0s. Team Arena's is offered
	// even without a Team Arena server, so adding one later needs no
	// rerun.
	if contains(missingBaseq3, "pak0.pk3") {
		stagePak0(opts, out, cwd, "baseq3", canonBaseq3, "retail Quake III Arena")
	}
	if contains(missingMP, "pak0.pk3") {
		stagePak0(opts, out, cwd, "missionpack", canonMP, "Team Arena")
	}

	missingBaseq3 = missingPaks(opts.Quake3Dir, "baseq3", requiredBaseq3)
	missingMP = missingPaks(opts.Quake3Dir, "missionpack", requiredMissionpack)
	patchesMissing := anyPatchMissing(missingBaseq3) || anyPatchMissing(missingMP)
	if patchesMissing {
		offerPatchDownload(opts, out)
		missingBaseq3 = missingPaks(opts.Quake3Dir, "baseq3", requiredBaseq3)
		missingMP = missingPaks(opts.Quake3Dir, "missionpack", requiredMissionpack)
	}

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
	fmt.Fprintln(out, "  Copy these in yourself before starting the game servers.")
	return result
}

// missingPaks returns the subset of names that aren't present in
// quake3Dir/mod; a missing mod directory means all of them.
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

// stagePak0 places a pak0.pk3 into quake3Dir/mod: canonName from the
// working directory without a prompt if it's there, otherwise any file
// the operator names. A blank answer skips it.
func stagePak0(opts PakStepOptions, out io.Writer, cwd, mod, canonName, label string) {
	dest := filepath.Join(opts.Quake3Dir, mod, "pak0.pk3")

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

	for {
		prompt := fmt.Sprintf("  Path to %s pak0.pk3 (blank to skip)", label)
		path, err := opts.Prompter.Optional(prompt, "")
		if err != nil {
			fmt.Fprintf(out, "  (skipping %s pak0)\n", mod)
			return
		}
		path = strings.TrimSpace(path)
		if path == "" {
			fmt.Fprintf(out, "  (skipped: put pak0.pk3 at %s before starting the server)\n", dest)
			return
		}
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

// isPlausiblePak checks for the zip magic number (PK\x03\x04), so a
// wrong path fails at the prompt instead of when the game server
// boots.
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

// installPakFile copies src to dest atomically (temp file and rename),
// mode 0644 and owned by the service user, creating the mod directory
// if needed.
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

// chownByName changes ownership to user:user with the system's `chown`,
// which resolves the name.
func chownByName(path, user string) {
	if user == "" {
		return
	}
	cmd := exec.Command("chown", user+":"+user, path)
	_ = cmd.Run()
}

// offerPatchDownload shows the EULA, then fetches and extracts the id
// Software 1.32 point release pk3s. It reports failures and returns;
// with the patches still missing, auto-start is skipped.
func offerPatchDownload(opts PakStepOptions, out io.Writer) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, "  The servers need the Quake III Arena 1.32 point-release patches. They")
	fmt.Fprintln(out, "  are distributed under the id Software EULA, shown on the next screen.")
	fmt.Fprintln(out)
	_ = opts.Prompter.Pause("  Press Enter to view the EULA: ")

	if err := pageEULA(); err != nil {
		fmt.Fprintf(out, "  (couldn't show the EULA with `more`: %v)\n", err)
		if url := hubURL(opts.HubHost, "/quake3-eula"); url != "" {
			fmt.Fprintf(out, "  The EULA is also at %s\n", url)
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

// printManualInstall tells the operator how to recover when the fetch
// couldn't run: put the zip in the working directory and rerun, or
// extract it by hand.
func printManualInstall(out io.Writer, opts PakStepOptions, name string) {
	fmt.Fprintf(out, "  To install them yourself, put %s in the directory you ran trinity init\n", name)
	fmt.Fprintf(out, "  from and run it again, or extract it into %s\n", opts.Quake3Dir)
	if url := hubURL(opts.HubHost, "/downloads/"+name); url != "" && opts.StaticDir == "" {
		fmt.Fprintf(out, "  (the hub serves it at %s).\n", url)
	}
}

// hubURL builds an https://<host><path> URL when host is non-empty,
// or "" when no hub is configured.
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

// pageEULA pipes the EULA through `more` so the operator scrolls
// through it. Without `more` it prints the text and waits for Enter.
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
// allowed mods into <Quake3Dir>/<mod>/<rest>. Existing files are kept.
//
// The zip has `baseq3/...` and `missionpack/...` at the top level, or
// inside one wrapper directory (e.g. `quake3-latest-pk3s/`), which is
// stripped. Entries outside the allowed mods, or escaping their mod
// directory with "..", are skipped.
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
		// A ".." in the entry could escape the mod dir.
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
// shares one top-level dir that isn't an allowed mod (the usual patch
// zip layout), and "" otherwise, including for mixed layouts.
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
//  1. opts.Cwd/<name>: a copy the operator put in the working
//     directory, mirrored into StaticDir/downloads/ on a hub.
//  2. opts.StaticDir/downloads/<name>: the hub's mirrored copy.
//  3. https://<HubHost>/downloads/<name>: downloaded from a remote hub
//     into a temp file.
//
// The cleanup func removes the temp file after a download and does
// nothing for a local path.
func resolveZip(opts PakStepOptions, out io.Writer, name string) (string, func(), error) {
	if opts.Cwd != "" {
		staged := filepath.Join(opts.Cwd, name)
		if _, err := os.Stat(staged); err == nil {
			fmt.Fprintf(out, "  Using %s from the current directory…\n", name)
			mirrorToDownloads(staged, opts, out, name)
			return staged, func() {}, nil
		}
	}
	if opts.StaticDir != "" {
		local := filepath.Join(opts.StaticDir, "downloads", name)
		if _, err := os.Stat(local); err == nil {
			fmt.Fprintf(out, "  Using local %s…\n", local)
			return local, func() {}, nil
		}
	}
	url := hubURL(opts.HubHost, "/downloads/"+name)
	if url == "" {
		return "", func() {}, fmt.Errorf("no copy of %s found; put it in the current directory and run again", name)
	}
	fmt.Fprintf(out, "  Downloading %s…\n", url)
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

// mirrorToDownloads copies a zip from the working directory into
// <StaticDir>/downloads/ for collectors to fetch, unless there's no
// StaticDir or a copy is already there.
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

// runBake produces the levelshot images and demo-playback pk3s the hub
// serves to web viewers, once the paks are in place. Both subcommands
// run as the service user through runuser so the trinity service can
// read what they write. It returns true if at least one step finished;
// failures are reported and don't stop the wizard.
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
		fmt.Fprintf(out, "  Running trinity %s…\n", sub)
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

// maybeAutoStart offers to start trinity and quake3-servers.target when
// systemd is in use. It returns true when the start command ran, however
// each service fared.
func maybeAutoStart(opts PakStepOptions, out io.Writer) bool {
	if !opts.UseSystemd {
		return false
	}
	confirm, err := opts.Prompter.YesNo("\nStart trinity and the game servers now?", true)
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
