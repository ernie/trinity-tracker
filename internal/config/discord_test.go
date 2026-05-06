package config

import (
	"strings"
	"testing"
)

// A missing or empty discord block is fine — Discord is opt-in.
func TestLoadAcceptsMissingDiscord(t *testing.T) {
	p := writeConfig(t, `
tracker:
  collector:
    source_id: "mygamesite"
    public_url: "https://q3.example.com"
    hub_host: "trinity.run"
  nats:
    credentials_file: "/etc/trinity/source.creds"
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Discord != nil {
		t.Errorf("Discord should be nil when block omitted, got %+v", cfg.Discord)
	}
}

func TestLoadAcceptsValidDiscord(t *testing.T) {
	p := writeConfig(t, `
discord:
  webhook_url: "https://discord.com/api/webhooks/1234567890/abcDEF-_xyz"
  digest_categories:
    - frags
    - kd_ratio
    - victories
tracker:
  collector:
    source_id: "mygamesite"
    public_url: "https://q3.example.com"
    hub_host: "trinity.run"
  nats:
    credentials_file: "/etc/trinity/source.creds"
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Discord == nil {
		t.Fatal("Discord should be populated")
	}
	if got := cfg.Discord.WebhookURL; got == "" {
		t.Error("webhook_url empty after Load")
	}
	if got := len(cfg.Discord.DigestCategories); got != 3 {
		t.Errorf("digest_categories: got %d, want 3", got)
	}
}

func TestLoadRejectsBadDiscord(t *testing.T) {
	cases := []struct {
		name      string
		body      string
		wantField string
	}{
		{
			name: "non_discord_url",
			body: `
discord:
  webhook_url: "https://example.com/hook"
`,
			wantField: "discord.webhook_url",
		},
		{
			name: "placeholder",
			body: `
discord:
  webhook_url: "https://discord.com/api/webhooks/REPLACE-ME/REPLACE-ME"
`,
			wantField: "discord.webhook_url",
		},
		{
			name: "unknown_category",
			body: `
discord:
  webhook_url: "https://discord.com/api/webhooks/1/x"
  digest_categories:
    - frags
    - rocket_jumps
`,
			wantField: "discord.digest_categories[1]",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := writeConfig(t, tc.body)
			_, err := Load(p)
			if err == nil {
				t.Fatalf("Load: expected error mentioning %s, got nil", tc.wantField)
			}
			if !strings.Contains(err.Error(), tc.wantField) {
				t.Errorf("error %q does not mention %s", err.Error(), tc.wantField)
			}
		})
	}
}
