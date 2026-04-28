package setup

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ernie/trinity-tracker/scripts"
)

// InstallNginx writes the embedded bootstrap-nginx.sh to a tempfile
// and runs it under bash with the wizard-collected env (PUBLIC_URL,
// ADMIN_EMAIL, STATIC_DIR, QUAKE3_DIR). The script handles package
// installation (nginx + certbot via apt/dnf/pacman), site config,
// firewall ports (ufw/firewalld), and the certbot --nginx run that
// gets the TLS cert.
//
// In dry-run mode nothing is executed; we just announce the plan.
// On failure the script's stdout/stderr have already streamed to the
// operator's terminal — we wrap the exit error with the script path
// so they can re-run it manually for diagnosis.
func InstallNginx(plan *Plan, publicURL, adminEmail, staticDir, quake3Dir string) error {
	if plan.DryRun {
		plan.say("would install nginx + certbot and obtain a Let's Encrypt cert for %s", publicURL)
		plan.say("would open firewall ports 80/tcp, 443/tcp, 27970/tcp, 27960-28000/udp (ufw/firewalld)")
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

	plan.say("Bootstrapping nginx + certbot for %s", publicURL)
	cmd := exec.Command("/bin/bash", scriptPath)
	cmd.Env = append(os.Environ(),
		"PUBLIC_URL="+publicURL,
		"ADMIN_EMAIL="+adminEmail,
		"STATIC_DIR="+staticDir,
		"QUAKE3_DIR="+quake3Dir,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		// Hint the operator at how to re-run if they fix whatever blocked
		// the script (DNS not propagated, firewall, etc.).
		base := filepath.Base(scriptPath)
		return fmt.Errorf("bootstrap-nginx failed: %w\n  retry: curl -fsSL https://raw.githubusercontent.com/ernie/trinity-tracker/main/scripts/bootstrap-nginx.sh \\\n    | sudo PUBLIC_URL=%s ADMIN_EMAIL=%s bash\n  (script was at %s before cleanup)", err, publicURL, adminEmail, base)
	}
	return nil
}
