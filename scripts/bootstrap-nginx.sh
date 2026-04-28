#!/usr/bin/env bash
set -euo pipefail

# Provisions nginx + certbot on a collector host so the hub can 302 to
# it for /demos/, /assets/levelshots/, /demopk3s/, plus a :27970 vhost
# for q3 fast-downloads. Required for collectors joining a hub network:
# without HTTPS the hub UI can't load demos from your host, and without
# the :27970 vhost q3 clients fall back to UDP downloads through the
# server itself — functional, but slow enough to be a poor user
# experience on any nontrivial pk3. See docs/collector-setup.md §6.

usage() {
    cat <<'EOF'
Usage: sudo PUBLIC_URL=... ADMIN_EMAIL=... ./scripts/bootstrap-nginx.sh

Required env:
  PUBLIC_URL   Publicly-reachable https URL for this host (e.g.
               https://nil.ernie.io). Hostname is used for server_name
               and certbot.
  ADMIN_EMAIL  Email certbot uses for renewal notices.

Optional env:
  QUAKE3_DIR   Root for the :27970 fast-download vhost (default
               /usr/lib/quake3).
  STATIC_DIR   Doc root mirroring the hub layout — /demos/,
               /assets/levelshots/, /demopk3s/ are served straight from
               here (default /var/lib/trinity/web).
EOF
}

PUBLIC_URL="${PUBLIC_URL:-}"
ADMIN_EMAIL="${ADMIN_EMAIL:-}"
QUAKE3_DIR="${QUAKE3_DIR:-/usr/lib/quake3}"
STATIC_DIR="${STATIC_DIR:-/var/lib/trinity/web}"

missing=()
[[ -z "$PUBLIC_URL"  ]] && missing+=("PUBLIC_URL")
[[ -z "$ADMIN_EMAIL" ]] && missing+=("ADMIN_EMAIL")
if (( ${#missing[@]} )); then
    echo "ERROR: missing required env: ${missing[*]}" >&2
    echo >&2
    usage >&2
    exit 1
fi

if [[ $EUID -ne 0 ]]; then
    cat <<EOF
This script needs root to install nginx + certbot (apt/dnf/pacman),
write to /etc/nginx, and run certbot. It will re-exec itself under
sudo, preserving the env vars above.

EOF
    read -r -p "Proceed with sudo? [y/N] " reply
    case "$reply" in
        y|Y|yes|YES) ;;
        *) echo "Aborted." >&2; exit 1 ;;
    esac
    exec sudo -E PUBLIC_URL="$PUBLIC_URL" ADMIN_EMAIL="$ADMIN_EMAIL" \
        QUAKE3_DIR="$QUAKE3_DIR" STATIC_DIR="$STATIC_DIR" \
        "$0" "$@"
fi

# Strip scheme, trailing path, and any port — certbot's -d takes a
# bare hostname only, and nginx's server_name doesn't take ports either.
PUBLIC_HOST="$(printf '%s' "$PUBLIC_URL" | sed -E 's|^https?://||; s|/.*$||; s|:.*$||')"
if [[ -z "$PUBLIC_HOST" ]]; then
    echo "ERROR: could not parse hostname from PUBLIC_URL=$PUBLIC_URL" >&2
    exit 1
fi

echo "==> installing nginx + certbot"
# Mirror install.sh's package-manager dispatch so this works on the
# same set of distros. Package names diverge: Debian/Ubuntu ships the
# nginx integration as python3-certbot-nginx; Fedora and Arch call it
# certbot-nginx. We use --webroot for ACME (not --nginx), so the nginx
# plugin isn't strictly required, but installing it keeps the script
# friendly to operators who later switch to --nginx renewal.
if command -v apt-get >/dev/null 2>&1; then
    apt-get update
    DEBIAN_FRONTEND=noninteractive apt-get install -y nginx certbot python3-certbot-nginx
elif command -v dnf >/dev/null 2>&1; then
    dnf install -y nginx certbot certbot-nginx
elif command -v pacman >/dev/null 2>&1; then
    pacman -Sy --noconfirm nginx certbot certbot-nginx
else
    echo "ERROR: no supported package manager (apt/dnf/pacman) found." >&2
    echo "       Install nginx + certbot manually and re-run." >&2
    exit 1
fi

echo "==> ensuring asset dirs exist under $STATIC_DIR"
install -d -o quake -g quake \
    "$STATIC_DIR" \
    "$STATIC_DIR/demos" \
    "$STATIC_DIR/assets/levelshots" \
    "$STATIC_DIR/demopk3s/maps"

# Open the firewall before certbot runs — Let's Encrypt validates over
# HTTP-01 to port 80, so a UFW/firewalld default-deny will time the cert
# fetch out (real failure mode hit on Debian + UFW). We open exactly the
# four ports a collector needs:
#   80/tcp       Let's Encrypt validation + HTTP→HTTPS redirect
#   443/tcp      collector content (demos, levelshots, demopk3s)
#   27970/tcp    nginx fast-download vhost for in-game pk3 fetches
#   27960-28000/udp  Quake 3 server traffic
# Skipped silently if the firewall is inactive — operators on cloud-side
# firewalls (Vultr/Hetzner/etc.) need to open the same ports there too.
open_firewall_ports() {
    if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -q '^Status: active'; then
        echo "==> opening UFW ports (80, 443, 27970/tcp + 27960-28000/udp)"
        ufw allow 80/tcp     >/dev/null
        ufw allow 443/tcp    >/dev/null
        ufw allow 27970/tcp  >/dev/null
        ufw allow 27960:28000/udp >/dev/null
        return
    fi
    if command -v firewall-cmd >/dev/null 2>&1 && firewall-cmd --state 2>/dev/null | grep -q running; then
        echo "==> opening firewalld ports (80, 443, 27970/tcp + 27960-28000/udp)"
        firewall-cmd --permanent --add-service=http  >/dev/null
        firewall-cmd --permanent --add-service=https >/dev/null
        firewall-cmd --permanent --add-port=27970/tcp >/dev/null
        firewall-cmd --permanent --add-port=27960-28000/udp >/dev/null
        firewall-cmd --reload >/dev/null
        return
    fi
    echo "==> no active host firewall (ufw/firewalld) detected; skipping local port-open"
    echo "    (open 80/tcp 443/tcp 27970/tcp 27960-28000/udp on any cloud firewall yourself)"
}
open_firewall_ports

SITE=/etc/nginx/sites-available/trinity-collector
write_collector_site() {
    # If $1 is "stage1": HTTP-only block, just enough for certbot's
    # webroot challenge. If "final": the full HTTP→HTTPS + HTTPS shape.
    case "$1" in
    stage1)
        cat > "$SITE" <<NGINX
# Stage 1 — HTTP-only, serves /.well-known/acme-challenge/ for certbot.
server {
    listen 80;
    listen [::]:80;
    server_name $PUBLIC_HOST;

    root $STATIC_DIR;

    location /.well-known/acme-challenge/ {
        autoindex off;
    }

    location / {
        return 404;
    }
}
NGINX
        ;;
    final)
        cat > "$SITE" <<NGINX
# HTTP → HTTPS redirect.
server {
    listen 80;
    listen [::]:80;
    server_name $PUBLIC_HOST;
    return 301 https://\$host\$request_uri;
}

# HTTPS content server.
server {
    listen [::]:443 ssl ipv6only=on;
    listen 443 ssl;
    server_name $PUBLIC_HOST;

    ssl_certificate     /etc/letsencrypt/live/$PUBLIC_HOST/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/$PUBLIC_HOST/privkey.pem;
    include             /etc/letsencrypt/options-ssl-nginx.conf;

    root $STATIC_DIR;

    # Recorded demos — fetched cross-origin by the hub's WASM engine
    # loader, so wildcard CORS is required.
    location /demos/ {
        autoindex off;
        add_header Access-Control-Allow-Origin "*" always;
    }

    # Map levelshots — \`<img>\` tags don't need CORS for display, but
    # adding the header is harmless and future-proofs JS use.
    location /assets/levelshots/ {
        autoindex off;
        expires 30d;
        add_header Cache-Control "public";
        add_header Access-Control-Allow-Origin "*" always;
    }

    # Demobaked map pk3s — fetched cross-origin by the engine loader.
    location /demopk3s/ {
        autoindex off;
        add_header Access-Control-Allow-Origin "*" always;
    }

    # Everything else: this isn't an app, return 404.
    location / {
        return 404;
    }
}
NGINX
        ;;
    *) echo "BUG: write_collector_site needs stage1|final" >&2; exit 1 ;;
    esac
}

CERT_PATH="/etc/letsencrypt/live/$PUBLIC_HOST/fullchain.pem"
if [[ -e "$CERT_PATH" ]]; then
    echo "==> existing cert at $CERT_PATH — writing final collector site directly"
    write_collector_site final
else
    echo "==> writing stage-1 collector site (HTTP-only, for ACME challenge)"
    write_collector_site stage1
fi

# Separate :27970 fast-download vhost mirrors what trinity.run
# does. Quake clients connect directly here (not via 80/443), so it's
# always plain HTTP. server_name is "_" (catch-all) so certbot --nginx
# doesn't see this block as a candidate for the cert; trinity-collector
# is the only block matching $PUBLIC_HOST on 80/443.
SITE_FD=/etc/nginx/sites-available/trinity-fastdl
cat > "$SITE_FD" <<NGINX
server {
    listen 27970 default_server;
    listen [::]:27970 default_server;
    server_name _;

    root $QUAKE3_DIR;

    # Retail pak0.pk3 isn't redistributable — block before the allowlist.
    location ~ ^/(baseq3|missionpack)/pak0\\.pk3\$ {
        return 403;
    }

    # Positive allowlist: only .pk3 (game assets) and .tvd (recorded
    # demos) are fetchable. q3 clients only ever ask for these.
    location ~ \\.(pk3|tvd)\$ {
        try_files \$uri =404;
    }

    # Default: refuse. Avoids leaking .cfg, .qvm, scripts/, etc.
    location / {
        return 403;
    }
}
NGINX

ln -sf "$SITE"    /etc/nginx/sites-enabled/trinity-collector
ln -sf "$SITE_FD" /etc/nginx/sites-enabled/trinity-fastdl

if [[ -L /etc/nginx/sites-enabled/default || -e /etc/nginx/sites-enabled/default ]]; then
    echo "==> removing /etc/nginx/sites-enabled/default to free :80 (was nginx's stock placeholder)"
    rm -f /etc/nginx/sites-enabled/default
fi

# Make sure cert renewals reload nginx so the served fullchain rotates.
# Idempotent — runs on every bootstrap, even when we skipped stage-1.
echo "==> installing certbot deploy hook to reload nginx on renewal"
install -d /etc/letsencrypt/renewal-hooks/deploy
cat > /etc/letsencrypt/renewal-hooks/deploy/reload-nginx.sh <<'EOF'
#!/bin/sh
systemctl reload nginx
EOF
chmod 0755 /etc/letsencrypt/renewal-hooks/deploy/reload-nginx.sh

echo "==> nginx -t"
nginx -t

echo "==> reloading nginx"
systemctl reload nginx

if [[ ! -e "$CERT_PATH" ]]; then
    echo "==> issuing TLS cert via certbot (--nginx)"
    # --nginx (full installer mode), not --webroot: certbot creates
    # /etc/letsencrypt/options-ssl-nginx.conf as a side effect of its
    # config-modification step. The final config below `include`s that
    # snippet, and webroot mode wouldn't have produced it on Debian
    # (Mozilla cipher recipe lives there, not bundled with certbot core).
    # certbot's edits to our stage-1 config are transient — the final
    # write_collector_site overwrites them with our preferred shape.
    certbot --nginx \
        --non-interactive --agree-tos \
        --email "$ADMIN_EMAIL" \
        -d "$PUBLIC_HOST"

    echo "==> writing final collector site (HTTP redirect + HTTPS content)"
    write_collector_site final

    echo "==> nginx -t (final)"
    nginx -t

    echo "==> reloading nginx"
    systemctl reload nginx
fi

echo
echo "Done."
echo "  Demos:       ${PUBLIC_URL%/}/demos/<uuid>.tvd"
echo "  Levelshots:  ${PUBLIC_URL%/}/assets/levelshots/<map>.jpg"
echo "  Demo pk3s:   ${PUBLIC_URL%/}/demopk3s/maps/<map>.pk3"
echo "  Fast-dl:     http://$PUBLIC_HOST:27970/"
echo
echo "Open TCP/80, TCP/443, TCP/27970 in your firewall if you have one."
echo "Generate the asset content with:"
echo "  sudo -u quake trinity levelshots /usr/lib/quake3"
echo "  sudo -u quake trinity demobake   /usr/lib/quake3"
