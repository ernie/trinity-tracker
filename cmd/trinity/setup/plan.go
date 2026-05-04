package setup

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"syscall"
)

// Plan is Apply's outbound interface to the host. In real mode it
// performs the operation; in dry-run mode it prints what it would
// do (prefixed with "[DRY]") and returns nil.
//
// Every mutating action (mkdir, write, chown, useradd, systemctl,
// engine fetch, etc.) goes through Plan so --dry-run can be exercised
// end-to-end without root.
type Plan struct {
	DryRun     bool
	UseSystemd bool
	Out        io.Writer
}

// Say prints a status line through Plan, prefixed with "[DRY] " in
// dry-run mode. Use this rather than fmt.Fprintln so callers from
// outside the package get the same dry-run treatment.
func (p *Plan) Say(format string, args ...any) {
	prefix := ""
	if p.DryRun {
		prefix = "[DRY] "
	}
	fmt.Fprintf(p.Out, prefix+format+"\n", args...)
}

// EnsureUser creates a system user if it doesn't exist, returning the
// resolved (uid, gid). In dry-run mode the user is *not* created; if
// it already exists we report its real ids, otherwise we return (0, 0)
// and a placeholder so the rest of the dry run can proceed.
func (p *Plan) EnsureUser(name string) (uid, gid int, err error) {
	if u, lookupErr := user.Lookup(name); lookupErr == nil {
		p.Say("Service user '%s' already exists.", name)
		uid, _ = strconv.Atoi(u.Uid)
		gid, _ = strconv.Atoi(u.Gid)
		return uid, gid, nil
	}
	if p.DryRun {
		p.Say("would useradd -r -m -d /home/%s -s /bin/bash %s", name, name)
		return 0, 0, nil
	}
	fmt.Fprintf(p.Out, "Creating service user '%s' ...\n", name)
	cmd := exec.Command("useradd", "-r", "-m", "-d", "/home/"+name, "-s", "/bin/bash", name)
	cmd.Stdout = p.Out
	cmd.Stderr = p.Out
	if err := cmd.Run(); err != nil {
		return 0, 0, fmt.Errorf("useradd: %w", err)
	}
	u, err := user.Lookup(name)
	if err != nil {
		return 0, 0, fmt.Errorf("looking up user %s: %w", name, err)
	}
	uid, _ = strconv.Atoi(u.Uid)
	gid, _ = strconv.Atoi(u.Gid)
	return uid, gid, nil
}

func (p *Plan) MkdirAll(path string, mode os.FileMode) error {
	if p.DryRun {
		p.Say("would mkdir -p %s (mode %#o)", path, mode)
		return nil
	}
	if err := os.MkdirAll(path, mode); err != nil {
		return fmt.Errorf("mkdir %s: %w", path, err)
	}
	return nil
}

func (p *Plan) Chown(path string, uid, gid int) error {
	if p.DryRun {
		p.Say("would chown %d:%d %s", uid, gid, path)
		return nil
	}
	if err := os.Chown(path, uid, gid); err != nil {
		return fmt.Errorf("chown %s: %w", path, err)
	}
	return nil
}

func (p *Plan) Lchown(path string, uid, gid int) error {
	if p.DryRun {
		p.Say("would chown -h %d:%d %s", uid, gid, path)
		return nil
	}
	if err := os.Lchown(path, uid, gid); err != nil {
		return fmt.Errorf("lchown %s: %w", path, err)
	}
	return nil
}

func (p *Plan) WriteFile(path string, data []byte, mode os.FileMode) error {
	if p.DryRun {
		p.Say("would write %s (%d bytes, mode %#o)", path, len(data), mode)
		return nil
	}
	if err := os.WriteFile(path, data, mode); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	// os.WriteFile only applies perm on creation. Re-runs over an
	// existing file (e.g. trinity init after a manual chmod) would
	// otherwise leave stale modes on rconpassword-bearing cfgs.
	if err := os.Chmod(path, mode); err != nil {
		return fmt.Errorf("chmod %s: %w", path, err)
	}
	return nil
}

// MkdirChown is the common idiom: ensure a dir exists, chown it.
func (p *Plan) MkdirChown(path string, mode os.FileMode, uid, gid int) error {
	if err := p.MkdirAll(path, mode); err != nil {
		return err
	}
	return p.Chown(path, uid, gid)
}

func (p *Plan) Symlink(target, link string) error {
	if p.DryRun {
		p.Say("would symlink %s → %s", link, target)
		return nil
	}
	_ = os.Remove(link)
	if err := os.Symlink(target, link); err != nil {
		return fmt.Errorf("symlink %s → %s: %w", link, target, err)
	}
	return nil
}

func (p *Plan) Remove(path string) error {
	if p.DryRun {
		p.Say("would rm %s", path)
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("rm %s: %w", path, err)
	}
	return nil
}

func (p *Plan) RemoveAll(path string) error {
	if p.DryRun {
		p.Say("would rm -rf %s", path)
		return nil
	}
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("rm -rf %s: %w", path, err)
	}
	return nil
}

// Systemctl runs systemctl with the given args. No-op when UseSystemd
// is false (non-systemd hosts). In dry-run mode prints what it would
// run.
func (p *Plan) Systemctl(args ...string) error {
	if !p.UseSystemd {
		return nil
	}
	if p.DryRun {
		p.Say("would systemctl %s", strings.Join(args, " "))
		return nil
	}
	cmd := exec.Command("systemctl", args...)
	cmd.Stdout = p.Out
	cmd.Stderr = p.Out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("systemctl %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

// asUserCmd builds an exec.Cmd that will run with uid/gid credentials
// when started. Stdout/stderr default to plan.Out — caller may
// override (e.g. WriteFileAs sends helper stdout to /dev/null).
func (p *Plan) asUserCmd(uid, gid int, name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)},
	}
	cmd.Stdout = p.Out
	cmd.Stderr = p.Out
	return cmd
}

// helperPath is the path used to re-exec trinity for `_helper`
// subcommands. /proc/self/exe always resolves to the currently-
// running binary, even mid-update when /usr/local/bin/trinity has
// been atomically renamed onto a new inode.
const helperPath = "/proc/self/exe"

// runHelper re-execs trinity as uid/gid with `_helper <verb> args...`.
// This is how privileged-dropped operations (write file, copy tree,
// download URL, unzip archive) actually run their work in pure Go
// inside a child that doesn't have root's authority.
func (p *Plan) runHelper(uid, gid int, stdin []byte, verb string, args ...string) error {
	full := append([]string{"_helper", verb}, args...)
	if p.DryRun {
		p.Say("would run as uid=%d gid=%d: trinity _helper %s %s", uid, gid, verb, strings.Join(args, " "))
		return nil
	}
	cmd := p.asUserCmd(uid, gid, helperPath, full...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("_helper %s: %w", verb, err)
	}
	return nil
}

// WriteFileAs writes data to path with the file owned by uid/gid and
// the given mode. The actual write happens in `_helper write` running
// as the service user, so the file lands service-owned from creation.
func (p *Plan) WriteFileAs(uid, gid int, path string, data []byte, mode os.FileMode) error {
	if p.DryRun {
		p.Say("would write %s as %d:%d (%d bytes, mode %#o)", path, uid, gid, len(data), mode)
		return nil
	}
	return p.runHelper(uid, gid, data, "write", path, fmt.Sprintf("%#o", mode))
}

// CopyTreeAs mirrors src into dst as the service user. Existing files
// in dst are overwritten; nothing is removed (overlay semantics).
func (p *Plan) CopyTreeAs(uid, gid int, src, dst string) error {
	if p.DryRun {
		p.Say("would copy tree %s → %s as %d:%d", src, dst, uid, gid)
		return nil
	}
	return p.runHelper(uid, gid, nil, "copy", src, dst)
}

// DownloadAs HTTPS-GETs url and writes the body to dest as the
// service user. dest's parent directory must already be writable by
// uid/gid (caller's responsibility — typically via MkdirChown).
func (p *Plan) DownloadAs(uid, gid int, url, dest string) error {
	if p.DryRun {
		p.Say("would download %s → %s as %d:%d", url, dest, uid, gid)
		return nil
	}
	return p.runHelper(uid, gid, nil, "download", url, dest)
}

// UnzipAs extracts src into dest as the service user, dropping
// stripComponents leading path segments. Replaces the previous
// _unzip-helper subcommand with a unified verb.
func (p *Plan) UnzipAs(uid, gid int, src, dest string, stripComponents int) error {
	if p.DryRun {
		p.Say("would unzip %s → %s (strip=%d) as %d:%d", src, dest, stripComponents, uid, gid)
		return nil
	}
	return p.runHelper(uid, gid, nil, "unzip", src, dest, fmt.Sprintf("%d", stripComponents))
}

// Setfacl runs `setfacl` with the given args. In dry-run mode prints
// what it would run. Used by installNATSTLSSymlinks to grant the
// service user read access to /etc/letsencrypt without changing
// certbot's default ownership / mode.
func (p *Plan) Setfacl(args ...string) error {
	if p.DryRun {
		p.Say("would setfacl %s", strings.Join(args, " "))
		return nil
	}
	cmd := exec.Command("setfacl", args...)
	cmd.Stdout = p.Out
	cmd.Stderr = p.Out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("setfacl %s: %w", strings.Join(args, " "), err)
	}
	return nil
}
