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
  digest:
    webhook_url: "https://discord.com/api/webhooks/1234567890/abcDEF-_xyz"
    categories:
      - frags
      - kd_ratio
      - victories
  activity:
    webhook_url: "https://discord.com/api/webhooks/9876543210/zyxWVU-_abc"
    inactive_debounce_seconds: 90
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
	if cfg.Discord.Digest == nil || cfg.Discord.Digest.WebhookURL == "" {
		t.Error("digest.webhook_url empty after Load")
	}
	if got := len(cfg.Discord.Digest.Categories); got != 3 {
		t.Errorf("digest.categories: got %d, want 3", got)
	}
	if cfg.Discord.Activity == nil || cfg.Discord.Activity.WebhookURL == "" {
		t.Error("activity.webhook_url empty after Load")
	}
	if cfg.Discord.Activity.InactiveDebounceSeconds != 90 {
		t.Errorf("activity.inactive_debounce_seconds: got %d, want 90", cfg.Discord.Activity.InactiveDebounceSeconds)
	}
}

// Old-shape configs (discord.webhook_url / discord.digest_categories at
// the top level) must fail loudly so operators upgrading see exactly
// what to rename. Silent yaml passthrough would just stop posting.
func TestLoadRejectsLegacyDiscordShape(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{
			name: "legacy_webhook_url",
			body: `
discord:
  webhook_url: "https://discord.com/api/webhooks/1/x"
`,
		},
		{
			name: "legacy_digest_categories",
			body: `
discord:
  digest_categories:
    - frags
`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := writeConfig(t, tc.body)
			_, err := Load(p)
			if err == nil {
				t.Fatal("Load: expected migration error, got nil")
			}
			if !strings.Contains(err.Error(), "discord.digest") {
				t.Errorf("error %q should point at the new nested fields", err.Error())
			}
		})
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
  digest:
    webhook_url: "https://example.com/hook"
`,
			wantField: "discord.digest.webhook_url",
		},
		{
			name: "placeholder",
			body: `
discord:
  digest:
    webhook_url: "https://discord.com/api/webhooks/REPLACE-ME/REPLACE-ME"
`,
			wantField: "discord.digest.webhook_url",
		},
		{
			name: "unknown_category",
			body: `
discord:
  digest:
    webhook_url: "https://discord.com/api/webhooks/1/x"
    categories:
      - frags
      - rocket_jumps
`,
			wantField: "discord.digest.categories[1]",
		},
		{
			name: "bad_activity_url",
			body: `
discord:
  activity:
    webhook_url: "https://example.com/hook"
`,
			wantField: "discord.activity.webhook_url",
		},
		{
			name: "negative_debounce",
			body: `
discord:
  activity:
    webhook_url: "https://discord.com/api/webhooks/1/x"
    inactive_debounce_seconds: -1
`,
			wantField: "discord.activity.inactive_debounce_seconds",
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
