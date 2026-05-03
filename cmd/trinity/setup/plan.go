package setup

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
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

func (p *Plan) say(format string, args ...any) {
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
		p.say("Service user '%s' already exists.", name)
		uid, _ = strconv.Atoi(u.Uid)
		gid, _ = strconv.Atoi(u.Gid)
		return uid, gid, nil
	}
	if p.DryRun {
		p.say("would useradd -r -m -d /home/%s -s /bin/bash %s", name, name)
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
		p.say("would mkdir -p %s (mode %#o)", path, mode)
		return nil
	}
	if err := os.MkdirAll(path, mode); err != nil {
		return fmt.Errorf("mkdir %s: %w", path, err)
	}
	return nil
}

func (p *Plan) Chown(path string, uid, gid int) error {
	if p.DryRun {
		p.say("would chown %d:%d %s", uid, gid, path)
		return nil
	}
	if err := os.Chown(path, uid, gid); err != nil {
		return fmt.Errorf("chown %s: %w", path, err)
	}
	return nil
}

func (p *Plan) Lchown(path string, uid, gid int) error {
	if p.DryRun {
		p.say("would chown -h %d:%d %s", uid, gid, path)
		return nil
	}
	if err := os.Lchown(path, uid, gid); err != nil {
		return fmt.Errorf("lchown %s: %w", path, err)
	}
	return nil
}

func (p *Plan) WriteFile(path string, data []byte, mode os.FileMode) error {
	if p.DryRun {
		p.say("would write %s (%d bytes, mode %#o)", path, len(data), mode)
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
		p.say("would symlink %s → %s", link, target)
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
		p.say("would rm %s", path)
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("rm %s: %w", path, err)
	}
	return nil
}

func (p *Plan) RemoveAll(path string) error {
	if p.DryRun {
		p.say("would rm -rf %s", path)
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
		p.say("would systemctl %s", strings.Join(args, " "))
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

// Setfacl runs `setfacl` with the given args. In dry-run mode prints
// what it would run. Used by installNATSTLSSymlinks to grant the
// service user read access to /etc/letsencrypt without changing
// certbot's default ownership / mode.
func (p *Plan) Setfacl(args ...string) error {
	if p.DryRun {
		p.say("would setfacl %s", strings.Join(args, " "))
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
