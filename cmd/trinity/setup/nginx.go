package setup

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"

	"github.com/ernie/trinity-tracker/scripts"
)

//go:embed nginxtemplates/*.tmpl
var nginxTemplates embed.FS

// NginxFields are the placeholders shared by hub.conf.tmpl and
// collector.conf.tmpl. Both templates draw from the same struct: hub
// uses StaticDir as the SPA root, collector uses it as the static
// asset root the hub fetches from. Quake3Dir is the dl.<host> fastdl
// root in both cases.
//
// Kept narrow on purpose: any operator-specific tuning (cache TTLs,
// rate limiting, geoip rules) belongs in a hand-edit on the deployed
// box, not in the templated default.
type NginxFields struct {
	PublicHost string
	StaticDir  string
	Quake3Dir  string
}

// RenderHubNginxConfig renders the embedded hub.conf.tmpl to bytes.
// Used by InstallNginx (and by tests that assert the output shape).
func RenderHubNginxConfig(f NginxFields) ([]byte, error) {
	return renderNginxTemplate("hub.conf.tmpl", f)
}

// RenderCollectorNginxConfig renders the embedded collector.conf.tmpl.
// Mirrors RenderHubNginxConfig — same input struct, different vhost
// shape (static asset hosting + fastdl, no SPA / api / ws).
func RenderCollectorNginxConfig(f NginxFields) ([]byte, error) {
	return renderNginxTemplate("collector.conf.tmpl", f)
}

func renderNginxTemplate(name string, f NginxFields) ([]byte, error) {
	raw, err := nginxTemplates.ReadFile("nginxtemplates/" + name)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	t, err := template.New(name).Parse(string(raw))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", name, err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, f); err != nil {
		return nil, fmt.Errorf("execute %s: %w", name, err)
	}
	return buf.Bytes(), nil
}

// NginxMode picks which flavor of vhost bootstrap-nginx.sh installs.
type NginxMode string

const (
	NginxModeHub       NginxMode = "hub"
	NginxModeCollector NginxMode = "collector"
)

// InstallNginx writes the embedded bootstrap-nginx.sh to a tempfile
// and runs it under bash with the wizard-collected fields. The script
// handles package installation (nginx + certbot via apt/dnf/pacman),
// site config, firewall ports (ufw/firewalld), and (unless skipCert)
// the certbot --nginx run that gets the TLS cert.
//
// Both hub and collector modes render their respective templates Go-side
// and pass the resulting file to the script via --site-file. The script
// itself is mode-agnostic in its handling of the rendered config (it
// just installs the file at the right path for the host distro).
//
// In dry-run mode nothing is executed; we just announce the plan.
// On failure the script's stdout/stderr have already streamed to the
// operator's terminal — we wrap the exit error with the script path
// so they can re-run it manually for diagnosis.
func InstallNginx(plan *Plan, mode NginxMode, publicURL, adminEmail, staticDir, quake3Dir string, skipCert, skipFirewall bool) error {
	publicHost := HostFromURL(publicURL)
	if plan.DryRun {
		plan.say("would render %s nginx config for %s", mode, publicHost)
		if skipCert {
			plan.say("would install nginx + reload (skip-cert: caller has staged /etc/letsencrypt/)")
		} else {
			plan.say("would install nginx + certbot and obtain a Let's Encrypt cert for %s", publicHost)
		}
		if skipFirewall {
			plan.say("would skip ufw/firewalld port-open (--skip-firewall)")
		} else {
			ports := "80/tcp, 443/tcp, 27960-28000/udp"
			if mode == NginxModeHub {
				ports = "80/tcp, 443/tcp, 4222/tcp, 27960-28000/udp"
			}
			plan.say("would open firewall ports %s (ufw/firewalld)", ports)
		}
		return nil
	}

	tmp, err := os.CreateTemp("", "trinity-bootstrap-nginx-*.sh")
	if err != nil {
		return fmt.Errorf("creating bootstrap-nginx tempfile: %w", err)
	}
	scriptPath := tmp.Name()
	defer os.Remove(scriptPath)

	if _, err := tmp.Write(scripts.BootstrapNginx); err != nil {
		tmp.Close()
		return fmt.Errorf("writing bootstrap-nginx: %w", err)
	}
	if err := tmp.Chmod(0o755); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod bootstrap-nginx: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close bootstrap-nginx: %w", err)
	}

	siteFile, err := stageSiteConf(mode, publicHost, staticDir, quake3Dir)
	if err != nil {
		return err
	}
	defer os.Remove(siteFile)

	args := []string{
		scriptPath,
		"--mode=" + string(mode),
		"--hostname=" + publicHost,
		"--quake3-dir=" + quake3Dir,
		"--static-dir=" + staticDir,
		"--site-file=" + siteFile,
	}
	if !skipCert {
		args = append(args, "--email="+adminEmail)
	} else {
		args = append(args, "--skip-cert")
	}
	if skipFirewall {
		args = append(args, "--skip-firewall")
	}

	plan.say("Bootstrapping nginx for %s (mode=%s skip-cert=%v skip-firewall=%v)", publicHost, mode, skipCert, skipFirewall)
	cmd := exec.Command("/bin/bash", args...)
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		base := filepath.Base(scriptPath)
		return fmt.Errorf("bootstrap-nginx failed: %w (script was at %s before cleanup)", err, base)
	}
	return nil
}

// stageSiteConf renders the mode-appropriate template into a temp file
// readable by root and returns the path. Caller is responsible for
// removing it.
func stageSiteConf(mode NginxMode, publicHost, staticDir, quake3Dir string) (string, error) {
	fields := NginxFields{
		PublicHost: publicHost,
		StaticDir:  staticDir,
		Quake3Dir:  quake3Dir,
	}
	var body []byte
	var err error
	switch mode {
	case NginxModeHub:
		body, err = RenderHubNginxConfig(fields)
	case NginxModeCollector:
		body, err = RenderCollectorNginxConfig(fields)
	default:
		return "", fmt.Errorf("stageSiteConf: unknown mode %q", mode)
	}
	if err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp("", "trinity-"+string(mode)+"-nginx-*.conf")
	if err != nil {
		return "", fmt.Errorf("creating %s site tempfile: %w", mode, err)
	}
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", fmt.Errorf("writing %s site: %w", mode, err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return "", fmt.Errorf("close %s site: %w", mode, err)
	}
	return tmp.Name(), nil
}
