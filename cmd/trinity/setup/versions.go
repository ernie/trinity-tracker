package setup

import (
	"archive/zip"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// VersionState classifies a current/latest comparison.
type VersionState int

const (
	StateUnknown  VersionState = iota // current couldn't be detected
	StateCurrent                      // exact match (or semver-core equal with no suffixes)
	StateBehind                       // installed semver-core is older than latest's
	StateDiverged                     // cores tie but installed carries a git-describe / dirty suffix,
	//                                   or installed core is somehow ahead of latest. Either way, the
	//                                   operator built or pulled this version deliberately and should
	//                                   confirm before we overwrite it.
)

// semverCore extracts the leading vX.Y.Z (or X.Y.Z) from a version
// string. Anything after the third numeric component is ignored and
// considered "suffix" by the caller.
var semverCore = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)`)

// CompareVersions classifies current relative to latest using a
// semver-core comparison. The model:
//
//   - Parse the leading vX.Y.Z (the "core") from each side.
//   - If installed core < latest core → StateBehind (clean upgrade).
//   - If installed core > latest core → StateDiverged (operator is
//     somehow ahead of the latest published tag).
//   - If cores tie:
//     • neither has a suffix → StateCurrent.
//     • either has a suffix (the git-describe "-N-g<hash>[-dirty]"
//     extended form, or a "-rc/-beta" pre-release marker) →
//     StateDiverged. The operator built or installed deliberately;
//     don't silently overwrite.
//
// Strings that don't parse (e.g. raw commit hashes) fall back to
// exact-equality → StateCurrent or otherwise → StateDiverged. Empty
// installed strings are StateUnknown.
//
// `trinity update` defaults the per-component prompt to Y for
// StateBehind and to N for StateDiverged / StateUnknown, so a
// press-enter-through never silently clobbers a custom build.
func CompareVersions(current, latest string) VersionState {
	c := strings.TrimSpace(current)
	l := strings.TrimSpace(latest)
	if c == "" {
		return StateUnknown
	}
	if c == l {
		return StateCurrent
	}
	cMaj, cMin, cPat, cOk := parseSemverCore(c)
	lMaj, lMin, lPat, lOk := parseSemverCore(l)
	if !cOk || !lOk {
		return StateDiverged
	}
	switch coreCmp(cMaj, cMin, cPat, lMaj, lMin, lPat) {
	case -1:
		return StateBehind
	case 1:
		return StateDiverged
	}
	// Cores tie — distinguish by suffix presence. Latest is almost
	// always a clean tag (no suffix); if installed has any suffix
	// it's a custom build past the bare core.
	if hasSuffix(c) || hasSuffix(l) {
		return StateDiverged
	}
	return StateCurrent
}

func parseSemverCore(s string) (major, minor, patch int, ok bool) {
	m := semverCore.FindStringSubmatch(s)
	if m == nil {
		return 0, 0, 0, false
	}
	major, _ = strconv.Atoi(m[1])
	minor, _ = strconv.Atoi(m[2])
	patch, _ = strconv.Atoi(m[3])
	return major, minor, patch, true
}

func coreCmp(aMaj, aMin, aPat, bMaj, bMin, bPat int) int {
	switch {
	case aMaj != bMaj:
		if aMaj < bMaj {
			return -1
		}
		return 1
	case aMin != bMin:
		if aMin < bMin {
			return -1
		}
		return 1
	case aPat != bPat:
		if aPat < bPat {
			return -1
		}
		return 1
	}
	return 0
}

// hasSuffix reports whether the version string carries anything past
// its semver core — git describe's "-N-g<hash>[-dirty]" form, a
// pre-release identifier ("-rc1", "-beta.2"), or a build metadata
// segment ("+build.7"). The leading "v" doesn't count.
func hasSuffix(s string) bool {
	t := strings.TrimPrefix(s, "v")
	return strings.ContainsAny(t, "-+")
}

// DetectEngineVersion runs `<quake3Dir>/trinity.ded --version` and
// extracts the "Trinity Engine: <ver>" line. Returns "" + nil error
// when the binary exists but predates the version flag (parsed output
// is missing the marker line); the orchestrator treats that as
// Unknown → upgradable.
func DetectEngineVersion(quake3Dir string) (string, error) {
	bin := filepath.Join(quake3Dir, "trinity.ded")
	cmd := exec.Command(bin, "--version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Older builds may not understand --version and exit non-zero.
		// That's a "predates the flag" signal, not a hard failure.
		return "", nil
	}
	for _, line := range strings.Split(string(out), "\n") {
		const prefix = "Trinity Engine:"
		if i := strings.Index(line, prefix); i >= 0 {
			return strings.TrimSpace(line[i+len(prefix):]), nil
		}
	}
	return "", nil
}

// ReadModVersionFromPk3 opens a .pk3 (a zip), looks for a top-level
// `trinity.ver`, and returns its trimmed contents. Returns "" + nil
// when the file isn't present (a non-trinity pk3 like a stock pak0).
func ReadModVersionFromPk3(pk3Path string) (string, error) {
	r, err := zip.OpenReader(pk3Path)
	if err != nil {
		return "", fmt.Errorf("opening %s: %w", pk3Path, err)
	}
	defer r.Close()
	for _, f := range r.File {
		if f.Name != "trinity.ver" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", fmt.Errorf("opening trinity.ver in %s: %w", pk3Path, err)
		}
		body, err := io.ReadAll(io.LimitReader(rc, 256))
		rc.Close()
		if err != nil {
			return "", fmt.Errorf("reading trinity.ver in %s: %w", pk3Path, err)
		}
		return strings.TrimSpace(string(body)), nil
	}
	return "", nil
}

// DetectModVersion scans <quake3Dir>/<modSubdir>/*.pk3 in alpha-reverse
// order (matching q3 vfs precedence) and returns the trinity.ver of
// the first pk3 that contains one — i.e. the version that would
// actually load at runtime. Returns "" + nil if no pk3 carries a
// trinity.ver marker (mod not installed).
func DetectModVersion(quake3Dir, modSubdir string) (string, error) {
	dir := filepath.Join(quake3Dir, modSubdir)
	entries, err := filepath.Glob(filepath.Join(dir, "*.pk3"))
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "", nil
	}
	// Alpha-reverse: pak9.pk3 wins over pak3.pk3, matching q3's load order.
	sort.Sort(sort.Reverse(sort.StringSlice(entries)))
	for _, p := range entries {
		ver, err := ReadModVersionFromPk3(p)
		if err != nil {
			// Skip unreadable / malformed pk3s — could be a stock pak.
			continue
		}
		if ver != "" {
			return ver, nil
		}
	}
	return "", nil
}
