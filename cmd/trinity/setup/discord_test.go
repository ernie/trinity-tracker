package setup

import (
	"strings"
	"testing"
)

// Default-on hub answers omit Discord — ToConfig must not emit a
// discord block, and Validate must accept the absence.
func TestToConfig_HubMode_NoDiscordWhenDisabled(t *testing.T) {
	a := validHubOnlyAnswers()
	if a.DiscordEnabled {
		t.Fatal("test fixture should default to disabled")
	}
	cfg := a.ToConfig()
	if cfg.Discord != nil {
		t.Errorf("expected cfg.Discord nil when DiscordEnabled=false, got %+v", cfg.Discord)
	}
	if err := a.Validate(); err != nil {
		t.Errorf("disabled-discord answers should validate, got %v", err)
	}
}

func TestToConfig_HubMode_EmitsDiscordWhenEnabled(t *testing.T) {
	a := validHubOnlyAnswers()
	a.DiscordEnabled = true
	a.DiscordWebhookURL = "https://discord.com/api/webhooks/12345/aBcDeF"
	a.DiscordSchedule = "Mon 00:00"

	if err := a.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	cfg := a.ToConfig()
	if cfg.Discord == nil {
		t.Fatal("cfg.Discord should be populated when DiscordEnabled=true")
	}
	if cfg.Discord.WebhookURL != a.DiscordWebhookURL {
		t.Errorf("webhook url: got %q, want %q", cfg.Discord.WebhookURL, a.DiscordWebhookURL)
	}
	if len(cfg.Discord.DigestCategories) != 0 {
		t.Errorf("DigestCategories should be empty so the subcommand uses its defaults, got %v", cfg.Discord.DigestCategories)
	}
}

// Collector-only mode should never emit a discord block, even if the
// fields are set somehow (e.g. a test fixture mistake).
func TestToConfig_CollectorOnly_DropsDiscord(t *testing.T) {
	a := &Answers{
		Mode: ModeCollector, ServiceUser: "quake", ListenAddr: "127.0.0.1", HTTPPort: 8080,
		Quake3Dir: "/usr/lib/quake3", HubHost: "trinity.run",
		PublicURL: "https://q3.example.com", AdminEmail: "ops@example.com", SourceID: "mygame", CredsFile: "/tmp/mygame.creds",
		DiscordEnabled:    true, // shouldn't matter
		DiscordWebhookURL: "https://discord.com/api/webhooks/12345/aBcDeF",
		DiscordSchedule:   "Mon 00:00",
		Servers: []ServerAnswers{
			{Key: "ffa", Gametype: GametypeFFA, Address: "q3.example.com:27960", Port: 27960, RconPassword: "secret", LogPath: "/var/log/quake3/ffa.log"},
		},
	}
	cfg := a.ToConfig()
	if cfg.Discord != nil {
		t.Errorf("collector-only should not emit cfg.Discord, got %+v", cfg.Discord)
	}
}

func TestAnswersValidate_DiscordRequiresWebhookAndSchedule(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*Answers)
		want string
	}{
		{"empty webhook", func(a *Answers) { a.DiscordEnabled = true; a.DiscordSchedule = "Mon 00:00" }, "webhook URL"},
		{"empty schedule", func(a *Answers) {
			a.DiscordEnabled = true
			a.DiscordWebhookURL = "https://discord.com/api/webhooks/1/x"
		}, "schedule"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := validHubOnlyAnswers()
			tc.mut(a)
			err := a.Validate()
			if err == nil {
				t.Fatalf("expected error mentioning %q, got nil", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q should mention %q", err, tc.want)
			}
		})
	}
}

// validateDiscordWebhookURL is the per-prompt validator used in the
// wizard. Mirrors internal/config's regex so the operator gets the
// same accept/reject judgment at prompt time as at config-load time.
func TestValidateDiscordWebhookURL(t *testing.T) {
	good := []string{
		"https://discord.com/api/webhooks/123/abcDEF-_xyz",
		"https://discord.com/api/webhooks/9999999999/aB1_-2cD",
	}
	for _, s := range good {
		if err := validateDiscordWebhookURL(s); err != nil {
			t.Errorf("expected accept, got %v: %s", err, s)
		}
	}
	bad := []string{
		"http://discord.com/api/webhooks/1/x",      // wrong scheme
		"https://discordapp.com/api/webhooks/1/x",  // wrong host
		"https://discord.com/api/webhooks/abc/xyz", // non-numeric id
		"https://discord.com/api/webhooks/1/",      // empty token
		"",
	}
	for _, s := range bad {
		if err := validateDiscordWebhookURL(s); err == nil {
			t.Errorf("expected reject: %s", s)
		}
	}
}
