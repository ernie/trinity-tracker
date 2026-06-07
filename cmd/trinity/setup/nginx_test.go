package setup

import (
	"strings"
	"testing"
)

func TestRenderHubNginxConfig_ContainsExpectedDirectives(t *testing.T) {
	out, err := RenderHubNginxConfig(NginxFields{
		PublicHost: "trinity.run",
		StaticDir:  "/var/lib/trinity/web",
		Quake3Dir:  "/usr/lib/quake3",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	rendered := string(out)

	mustContain := []string{
		"server_name trinity.run;",
		"ssl_certificate     /etc/letsencrypt/live/trinity.run/fullchain.pem;",
		"ssl_certificate_key /etc/letsencrypt/live/trinity.run/privkey.pem;",
		"root /var/lib/trinity/web;",
		"return 301 https://$host$request_uri;",
		"location /api/ {",
		"location /ws {",
		"proxy_set_header Upgrade $http_upgrade;",
		"location /demos/ {",
		"location /assets/levelshots/ {",
		"add_header Cache-Control \"public, max-age=31536000, immutable\" always;",
		"location /demopk3s/maps/        { try_files $uri @trinity_fallback; }",
		"location @trinity_fallback {",
		// TrinityVision live: two-segment router is always present (beats the SPA).
		"location ~ ^/tv/[^/]+/[^/]+/stream$ {",
		"location @spa {",
		"rewrite ^ /index.html last;",
		// /q3/ pak/tvd alias on the main vhost (used by the in-browser engine).
		"location ~ ^/q3/(baseq3|missionpack)/pak0\\.pk3$ {",
		"location ~ ^/q3/(.+\\.(pk3|tvd))$ {",
		"alias /usr/lib/quake3/$1;",
		// fastdl block (dl.<host> on :80 + :443)
		"server_name dl.trinity.run;",
		"root /usr/lib/quake3;",
		"location ~ ^/(baseq3|missionpack)/pak0\\.pk3$ {",
		"location ~ \\.(pk3|tvd)$ {",
	}
	for _, want := range mustContain {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered hub.conf missing %q\n--- output ---\n%s", want, rendered)
		}
	}

	// The template must NOT carry the legacy redirect (operator retired
	// trinity.ernie.io ahead of the migration).
	if strings.Contains(rendered, "trinity.ernie.io") {
		t.Errorf("rendered hub.conf still references the legacy hostname:\n%s", rendered)
	}

	// Pure hub (HasLocalRelay default false): no local relay, so no
	// single-segment /tv proxy to :8081 — it only routes the two-segment form.
	if strings.Contains(rendered, "127.0.0.1:8081") {
		t.Errorf("pure hub.conf must not proxy to the relay :8081:\n%s", rendered)
	}
}

func TestRenderHubNginxConfig_CombinedIncludesRelay(t *testing.T) {
	out, err := RenderHubNginxConfig(NginxFields{
		PublicHost:    "trinity.run",
		StaticDir:     "/var/lib/trinity/web",
		Quake3Dir:     "/usr/lib/quake3",
		HasLocalRelay: true,
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	rendered := string(out)
	// A combined box serves the single-segment relay stream itself. The relay
	// can go >60s without bytes (intermission/engine stall); without long
	// timeouts nginx 504s the viewer. proxy_send_timeout is the discriminator —
	// the /ws block carries only proxy_read_timeout.
	for _, want := range []string{
		"location ~ ^/tv/[^/]+$ {",
		"proxy_pass http://127.0.0.1:8081;",
		"proxy_send_timeout 86400;",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("combined hub.conf missing %q\n--- output ---\n%s", want, rendered)
		}
	}
}

func TestRenderHubNginxConfig_HostInterpolation(t *testing.T) {
	out, err := RenderHubNginxConfig(NginxFields{
		PublicHost: "hub.example.com",
		StaticDir:  "/srv/trinity",
		Quake3Dir:  "/srv/q3",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	rendered := string(out)
	if !strings.Contains(rendered, "server_name hub.example.com;") {
		t.Errorf("custom hostname not interpolated:\n%s", rendered)
	}
	if !strings.Contains(rendered, "/etc/letsencrypt/live/hub.example.com/fullchain.pem") {
		t.Errorf("cert path not interpolated for custom host:\n%s", rendered)
	}
	if !strings.Contains(rendered, "root /srv/trinity;") {
		t.Errorf("static_dir not interpolated:\n%s", rendered)
	}
	if !strings.Contains(rendered, "root /srv/q3;") {
		t.Errorf("quake3_dir not interpolated for fastdl block:\n%s", rendered)
	}
}

func TestRenderCollectorNginxConfig_ContainsExpectedDirectives(t *testing.T) {
	out, err := RenderCollectorNginxConfig(NginxFields{
		PublicHost: "q3.example.com",
		StaticDir:  "/var/lib/trinity/web",
		Quake3Dir:  "/usr/lib/quake3",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	rendered := string(out)

	mustContain := []string{
		"server_name q3.example.com;",
		"return 301 https://$host$request_uri;",
		"ssl_certificate     /etc/letsencrypt/live/q3.example.com/fullchain.pem;",
		"include             /etc/letsencrypt/options-ssl-nginx.conf;",
		"root /var/lib/trinity/web;",
		"location /demos/ {",
		`add_header Cache-Control "public, max-age=31536000, immutable" always;`,
		`add_header Access-Control-Allow-Origin "*" always;`,
		"location /assets/levelshots/ {",
		"expires 30d;",
		"location /demopk3s/ {",
		// TrinityVision live: collectors always serve the single-segment relay.
		"location ~ ^/tv/[^/]+$ {",
		"proxy_pass http://127.0.0.1:8081;",
		// Long timeouts + HTTP/1.1 so a quiet spell (intermission) doesn't 504.
		"proxy_http_version 1.1;",
		"proxy_read_timeout 86400;",
		"proxy_send_timeout 86400;",
		"return 404;",
		// fastdl block (dl.<host> on :80 + :443)
		"server_name dl.q3.example.com;",
		"root /usr/lib/quake3;",
		"location ~ ^/(baseq3|missionpack)/pak0\\.pk3$ {",
		"return 403;",
		"location ~ \\.(pk3|tvd)$ {",
	}
	for _, want := range mustContain {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered collector.conf missing %q\n--- output ---\n%s", want, rendered)
		}
	}

	// Collector vhost is not an app — no SPA, no api/ws proxy.
	for _, unwanted := range []string{
		"location /api/ {",
		"location /ws {",
		"@spa",
		"@trinity_fallback",
	} {
		if strings.Contains(rendered, unwanted) {
			t.Errorf("collector.conf unexpectedly contains %q\n%s", unwanted, rendered)
		}
	}
}

func TestRenderCollectorNginxConfig_HostInterpolation(t *testing.T) {
	out, err := RenderCollectorNginxConfig(NginxFields{
		PublicHost: "node-1.example.com",
		StaticDir:  "/srv/trinity",
		Quake3Dir:  "/srv/q3",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	rendered := string(out)
	for _, want := range []string{
		"server_name node-1.example.com;",
		"/etc/letsencrypt/live/node-1.example.com/fullchain.pem",
		"root /srv/trinity;",
		"root /srv/q3;",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("collector.conf missing interpolated %q\n%s", want, rendered)
		}
	}
}
