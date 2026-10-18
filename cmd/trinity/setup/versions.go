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
	StateDiverged                     // a custom build or one ahead of latest; confirm before overwriting
)

// semverCore extracts the leading vX.Y.Z (or X.Y.Z) from a version
// string. Anything after the third numeric component is ignored and
// considered "suffix" by the caller.
var semverCore = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)`)

// CompareVersions classifies current relative to latest by their
// semver cores (the leading vX.Y.Z):
//
//   - an older core is StateBehind
//   - a newer core is StateDiverged
//   - equal cores are StateCurrent, or StateDiverged when either side
//     carries a suffix (git describe's "-N-g<hash>[-dirty]" or a
//     pre-release such as "-rc1")
//
// Strings without a core, such as raw commit hashes, are StateCurrent
// when equal and StateDiverged otherwise. An empty current is
// StateUnknown. `trinity update` defaults its prompt to Y only for
// StateBehind, so pressing Enter through it keeps a custom build.
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
// its semver core: git describe's "-N-g<hash>[-dirty]" form, a
// pre-release identifier ("-rc1", "-beta.2"), or a build metadata
// segment ("+build.7"). The leading "v" doesn't count.
func hasSuffix(s string) bool {
	t := strings.TrimPrefix(s, "v")
	return strings.ContainsAny(t, "-+")
}

// DetectEngineVersion runs `<quake3Dir>/trinity.ded --version` and
// extracts the "Trinity Engine: <ver>" line. It returns "" and a nil
// error for a binary that predates --version.
func DetectEngineVersion(quake3Dir string) (string, error) {
	bin := filepath.Join(quake3Dir, "trinity.ded")
	cmd := exec.Command(bin, "--version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		// A build that predates --version exits non-zero.
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
// `trinity.ver`, and returns its trimmed contents, or "" and a nil
// error for a pk3 without one (such as a stock pak0).
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

// DetectModVersion scans <quake3Dir>/<modSubdir>/*.pk3 in reverse
// alphabetical order, the game's load precedence, and returns the
// first trinity.ver it finds: the version that loads at runtime. It
// returns "" and a nil error when no pk3 has one.
func DetectModVersion(quake3Dir, modSubdir string) (string, error) {
	dir := filepath.Join(quake3Dir, modSubdir)
	entries, err := filepath.Glob(filepath.Join(dir, "*.pk3"))
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "", nil
	}
	sort.Sort(sort.Reverse(sort.StringSlice(entries)))
	for _, p := range entries {
		ver, err := ReadModVersionFromPk3(p)
		if err != nil {
			continue
		}
		if ver != "" {
			return ver, nil
		}
	}
	return "", nil
}
