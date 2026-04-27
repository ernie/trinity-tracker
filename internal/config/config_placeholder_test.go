package config

import (
	"strings"
	"testing"
)

// TestLoadRejectsREPLACEMEPlaceholders covers each field that
// scripts/config.yml.example ships with a "REPLACE-ME" placeholder.
// Without this check, a half-edited config loads cleanly and the
// collector publishes against a bogus source or rcons with the
// literal word "REPLACE-ME".
func TestLoadRejectsREPLACEMEPlaceholders(t *testing.T) {
	cases := []struct {
		name      string
		body      string
		wantField string
	}{
		{
			name: "source_id",
			body: `
tracker:
  collector:
    source_id: "REPLACE-ME"
    public_url: "https://example.com"
    hub_host: "trinity.run"
  nats:
    credentials_file: "/etc/trinity/source.creds"
`,
			wantField: "tracker.collector.source_id",
		},
		{
			name: "public_url",
			body: `
tracker:
  collector:
    source_id: "mygamesite"
    public_url: "https://REPLACE-ME.example.com"
    hub_host: "trinity.run"
  nats:
    credentials_file: "/etc/trinity/source.creds"
`,
			wantField: "tracker.collector.public_url",
		},
		{
			name: "q3_servers_address",
			body: `
tracker:
  collector:
    source_id: "mygamesite"
    public_url: "https://example.com"
    hub_host: "trinity.run"
  nats:
    credentials_file: "/etc/trinity/source.creds"
q3_servers:
  - key: ffa
    address: "REPLACE-ME.example.com:27960"
    log_path: /var/log/quake3/ffa.log
    rcon_password: secret
`,
			wantField: "q3_servers[0].address",
		},
		{
			name: "q3_servers_rcon_password",
			body: `
tracker:
  collector:
    source_id: "mygamesite"
    public_url: "https://example.com"
    hub_host: "trinity.run"
  nats:
    credentials_file: "/etc/trinity/source.creds"
q3_servers:
  - key: ffa
    address: "host.example.com:27960"
    log_path: /var/log/quake3/ffa.log
    rcon_password: "REPLACE-ME"
`,
			wantField: "q3_servers[0].rcon_password",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := writeConfig(t, tc.body)
			_, err := Load(p)
			if err == nil {
				t.Fatalf("Load: expected error for %s, got nil", tc.wantField)
			}
			if !strings.Contains(err.Error(), tc.wantField) {
				t.Errorf("error %q does not mention %s", err.Error(), tc.wantField)
			}
		})
	}
}

func TestLoadAcceptsCleanConfig(t *testing.T) {
	p := writeConfig(t, `
tracker:
  collector:
    source_id: "mygamesite"
    public_url: "https://q3.example.com"
    hub_host: "trinity.run"
  nats:
    credentials_file: "/etc/trinity/source.creds"
q3_servers:
  - key: ffa
    address: "q3.example.com:27960"
    log_path: /var/log/quake3/ffa.log
    rcon_password: "long-random-string"
`)
	if _, err := Load(p); err != nil {
		t.Fatalf("Load: clean config rejected: %v", err)
	}
}
