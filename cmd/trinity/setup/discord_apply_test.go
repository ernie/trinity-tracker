package setup

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestApply_DryRun_Discord_DisabledOmitsDigestUnits(t *testing.T) {
	tmp := t.TempDir()
	a := &Answers{
		Mode:         ModeHubOnly,
		ServiceUser:  "quake",
		ListenAddr:   "127.0.0.1",
		HTTPPort:     8080,
		DatabasePath: filepath.Join(tmp, "var", "trinity.db"),
		StaticDir:    filepath.Join(tmp, "web"),
		PublicURL:    "https://hub.example.com",
		AdminEmail:   "ops@example.com",
	}
	if err := a.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	buf := &bytes.Buffer{}
	if err := Apply(a, ApplyOptions{
		ConfigPath: filepath.Join(tmp, "config.yml"),
		UseSystemd: true,
		DryRun:     true,
		Out:        buf,
	}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	out := buf.String()
	for _, unwanted := range []string{
		"trinity-digest.service",
		"trinity-digest.timer",
		"systemctl enable --now trinity-digest.timer",
	} {
		if strings.Contains(out, unwanted) {
			t.Errorf("disabled-discord dry-run referenced %q\n--- output ---\n%s", unwanted, out)
		}
	}
}

func TestApply_DryRun_Discord_EnabledInstallsAndEnablesTimer(t *testing.T) {
	tmp := t.TempDir()
	a := &Answers{
		Mode:              ModeHubOnly,
		ServiceUser:       "quake",
		ListenAddr:        "127.0.0.1",
		HTTPPort:          8080,
		DatabasePath:      filepath.Join(tmp, "var", "trinity.db"),
		StaticDir:         filepath.Join(tmp, "web"),
		PublicURL:         "https://hub.example.com",
		AdminEmail:        "ops@example.com",
		DiscordEnabled:    true,
		DiscordWebhookURL: "https://discord.com/api/webhooks/12345/abcDEF-_xyz",
		DiscordSchedule:   "Mon 00:00",
	}
	if err := a.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	buf := &bytes.Buffer{}
	if err := Apply(a, ApplyOptions{
		ConfigPath: filepath.Join(tmp, "config.yml"),
		UseSystemd: true,
		DryRun:     true,
		Out:        buf,
	}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"[DRY] would write /etc/systemd/system/trinity-digest.service",
		"[DRY] would write /etc/systemd/system/trinity-digest.timer",
		"[DRY] would systemctl enable --now trinity-digest.timer",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q\n--- output ---\n%s", want, out)
		}
	}
}

func TestSystemdUnitTemplated_TimerSchedule(t *testing.T) {
	data, err := SystemdUnitTemplated("trinity-digest.timer", "quake", map[string]string{
		"schedule": "Mon..Fri 09:30",
	})
	if err != nil {
		t.Fatalf("SystemdUnitTemplated: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, "OnCalendar=Mon..Fri 09:30") {
		t.Errorf("schedule not substituted; got: %s", body)
	}
	if strings.Contains(body, "{{schedule}}") {
		t.Errorf("template token left untouched")
	}
}
