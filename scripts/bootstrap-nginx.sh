#!/usr/bin/env bash
set -euo pipefail

# Provisions nginx + certbot for either flavor of Trinity install. Both
# modes get their site config rendered Go-side (from the embedded
# templates at cmd/trinity/setup/nginxtemplates/{hub,collector}.conf.tmpl)
# and passed in via --site-file=<path>. This script is mode-agnostic in
# its handling of the rendered config — it just installs nginx, opens
# firewall ports, copies the file into place, runs certbot, and reloads.
#
# Modes:
#   --mode=hub       Public front for a Trinity hub (SPA + api/ws +
#                    asset-fallback proxy + :27970 fastdl).
#   --mode=collector Static asset host the hub 302s to (demos,
#                    levelshots, demopk3s + :27970 fastdl).
#
# Cross-distro:
#   - Debian/Ubuntu: sites-available/sites-enabled, certbot --nginx via
#     python3-certbot-nginx
#   - Arch: conf.d/, certbot-nginx
#   - Fedora/RHEL: conf.d/, python3-certbot-nginx; demotes the stock
#     :80 default_server so server_name matching wins for our vhost
#
# See docs/collector-setup.md §6 for collector-mode background.

usage() {
    cat <<'EOF'
Usage:
  sudo bootstrap-nginx.sh \
      --mode=hub|collector \
      --hostname=<host> \
      --site-file=<path> \
      [--email=<addr>] \
      [--quake3-dir=<path>] \
      [--static-dir=<path>] \
      [--skip-cert]

Required:
  --mode=hub|collector   Which flavor of vhost to install.
  --hostname=<host>      Bare public hostname (e.g. trinity.run).
  --site-file=<path>     Pre-rendered nginx site file (the trinity
                         binary stages this from its embedded template
                         before invoking the script).
  --email=<addr>         Email certbot uses for renewal notices.
                         Required unless --skip-cert.

Optional:
  --quake3-dir=<path>    Informational only (the path is baked into
                         --site-file already; passed for log output).
                         Default /usr/lib/quake3.
  --static-dir=<path>    Used in collector mode to pre-create the
                         demos/, assets/levelshots/, demopk3s/ subtrees
                         with quake ownership so trinity demobake +
                         levelshots can write into them. Default
                         /var/lib/trinity/web.
  --skip-cert            Do not run certbot. Caller must have placed
                         /etc/letsencrypt/live/<host>/{fullchain,privkey}.pem
                         already (used during hub migrations where the
                         operator rsynces /etc/letsencrypt from another
                         host before DNS flips).
  --skip-firewall        Do not poke ufw/firewalld. Operator manages
                         host firewall via cloud dashboard, nftables,
                         or config-management.
EOF
}

MODE=""
PUBLIC_HOST=""
ADMIN_EMAIL=""
SITE_FILE=""
QUAKE3_DIR="/usr/lib/quake3"
STATIC_DIR="/var/lib/trinity/web"
SKIP_CERT=0
SKIP_FIREWALL=0

while [[ $# -gt 0 ]]; do
    case "$1" in
        --mode=*)        MODE="${1#*=}" ;;
        --hostname=*)    PUBLIC_HOST="${1#*=}" ;;
        --email=*)       ADMIN_EMAIL="${1#*=}" ;;
        --site-file=*)   SITE_FILE="${1#*=}" ;;
        --quake3-dir=*)  QUAKE3_DIR="${1#*=}" ;;
        --static-dir=*)  STATIC_DIR="${1#*=}" ;;
        --skip-cert)     SKIP_CERT=1 ;;
        --skip-firewall) SKIP_FIREWALL=1 ;;
        -h|--help)       usage; exit 0 ;;
        *) echo "ERROR: unknown argument: $1" >&2; usage >&2; exit 1 ;;
    esac
    shift
done

case "$MODE" in
    hub|collector) ;;
    "") echo "ERROR: --mode is required (hub or collector)" >&2; usage >&2; exit 1 ;;
    *) echo "ERROR: --mode must be 'hub' or 'collector' (got '$MODE')" >&2; exit 1 ;;
esac

missing=()
[[ -z "$PUBLIC_HOST" ]] && missing+=("--hostname")
[[ -z "$SITE_FILE" ]]   && missing+=("--site-file")
if (( SKIP_CERT == 0 )); then
    [[ -z "$ADMIN_EMAIL" ]] && missing+=("--email (or --skip-cert)")
fi
if (( ${#missing[@]} )); then
    echo "ERROR: missing required args:" >&2
    for m in "${missing[@]}"; do echo "  - $m" >&2; done
    echo >&2
    usage >&2
    exit 1
fi

if [[ ! -f "$SITE_FILE" ]]; then
    echo "ERROR: --site-file '$SITE_FILE' not found or not a regular file" >&2
    exit 1
fi

if [[ $EUID -ne 0 ]]; then
    cat <<EOF
This script needs root to install nginx + certbot, write to /etc/nginx,
and run certbot. It will re-exec itself under sudo, preserving args.

EOF
    read -r -p "Proceed with sudo? [y/N] " reply
    case "$reply" in
        y|Y|yes|YES) ;;
        *) echo "Aborted." >&2; exit 1 ;;
    esac
    exec sudo -E "$0" \
        --mode="$MODE" \
        --hostname="$PUBLIC_HOST" \
        --site-file="$SITE_FILE" \
        --quake3-dir="$QUAKE3_DIR" \
        --static-dir="$STATIC_DIR" \
        ${ADMIN_EMAIL:+--email="$ADMIN_EMAIL"} \
        $( (( SKIP_CERT     )) && echo --skip-cert ) \
        $( (( SKIP_FIREWALL )) && echo --skip-firewall )
fi

echo "==> mode=$MODE  host=$PUBLIC_HOST  skip-cert=$SKIP_CERT  skip-firewall=$SKIP_FIREWALL"

echo "==> installing nginx + certbot"
# Mirror install.sh's package-manager dispatch. Package names diverge:
# Debian/Ubuntu ship the certbot --nginx integration as
# python3-certbot-nginx; Arch calls it certbot-nginx.
if command -v apt-get >/dev/null 2>&1; then
    apt-get update
    DEBIAN_FRONTEND=noninteractive apt-get install -y nginx certbot python3-certbot-nginx
elif command -v pacman >/dev/null 2>&1; then
    pacman -Sy --noconfirm nginx certbot certbot-nginx
elif command -v dnf >/dev/null 2>&1; then
    dnf install -y nginx certbot python3-certbot-nginx
else
    echo "ERROR: no supported package manager (apt/pacman/dnf) found." >&2
    exit 1
fi

# Collector-only: stage the asset dirs the hub will hit via 302. Hub
# mode owns its own dir setup via apply.go's ensureDirs and doesn't
# need this here.
if [[ "$MODE" == "collector" ]]; then
    echo "==> ensuring asset dirs exist under $STATIC_DIR"
    install -d -o quake -g quake \
        "$STATIC_DIR" \
        "$STATIC_DIR/demos" \
        "$STATIC_DIR/assets" \
        "$STATIC_DIR/assets/levelshots" \
        "$STATIC_DIR/demopk3s" \
        "$STATIC_DIR/demopk3s/maps"
fi

# Open the firewall before certbot runs — Let's Encrypt validates over
# HTTP-01 to port 80, so a UFW/firewalld default-deny will time the
# cert fetch out. Hub mode also opens 4222/tcp for remote collectors.
open_firewall_ports() {
    local ports_tcp=(80 443 27970)
    local ports_udp_range="27960:28000"
    if [[ "$MODE" == "hub" ]]; then
        ports_tcp+=(4222)
    fi
    if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -q '^Status: active'; then
        echo "==> opening UFW ports (${ports_tcp[*]}/tcp + 27960-28000/udp)"
        for p in "${ports_tcp[@]}"; do ufw allow "${p}/tcp" >/dev/null; done
        ufw allow "${ports_udp_range}/udp" >/dev/null
        return
    fi
    if command -v firewall-cmd >/dev/null 2>&1 && firewall-cmd --state 2>/dev/null | grep -q running; then
        echo "==> opening firewalld ports (${ports_tcp[*]}/tcp + 27960-28000/udp)"
        firewall-cmd --permanent --add-service=http  >/dev/null
        firewall-cmd --permanent --add-service=https >/dev/null
        for p in "${ports_tcp[@]}"; do
            case "$p" in 80|443) ;; *) firewall-cmd --permanent --add-port="${p}/tcp" >/dev/null ;; esac
        done
        firewall-cmd --permanent --add-port=27960-28000/udp >/dev/null
        firewall-cmd --reload >/dev/null
        return
    fi
    echo "==> no active host firewall (ufw/firewalld) detected; skipping local port-open"
    echo "    (open ${ports_tcp[*]}/tcp + 27960-28000/udp on any cloud firewall yourself)"
}
if (( SKIP_FIREWALL )); then
    echo "==> --skip-firewall: leaving ufw/firewalld alone (operator manages host firewall)"
else
    open_firewall_ports
fi

# Distro layout detection. Different distros lay out nginx config
# differently; emit configs in the matching shape so we don't have to
# maintain two scripts.
if [[ -d /etc/nginx/sites-available && -d /etc/nginx/sites-enabled ]]; then
    LAYOUT=debian
    SITE_DIR=/etc/nginx/sites-available
    SITE_ENABLED_DIR=/etc/nginx/sites-enabled
else
    LAYOUT=confd
    SITE_DIR=/etc/nginx/conf.d
    SITE_ENABLED_DIR=
    install -d -m 0755 /etc/nginx/conf.d
    if ! grep -qE 'include[[:space:]]+[^;]*conf\.d/' /etc/nginx/nginx.conf 2>/dev/null; then
        echo "==> patching /etc/nginx/nginx.conf to include conf.d/*.conf"
        cp -p /etc/nginx/nginx.conf "/etc/nginx/nginx.conf.bak.$(date +%s)"
        last_brace=$(grep -n '^}' /etc/nginx/nginx.conf | tail -1 | cut -d: -f1)
        if [[ -z "$last_brace" ]]; then
            echo "ERROR: couldn't find closing http{} brace in /etc/nginx/nginx.conf to patch" >&2
            exit 1
        fi
        sed -i "${last_brace}i\\    include /etc/nginx/conf.d/*.conf;" /etc/nginx/nginx.conf
    fi
fi

# Both modes write a single combined site file (HTTP→HTTPS + HTTPS
# vhost + :27970 fastdl all in one). Filename varies by distro layout.
if [[ "$LAYOUT" == "debian" ]]; then
    PRIMARY_SITE="$SITE_DIR/trinity"
else
    PRIMARY_SITE="$SITE_DIR/trinity.conf"
fi

# Stage-1 site is mode-agnostic: HTTP-only with the right server_name,
# enough for certbot --nginx to add its ACME challenge location. We
# overwrite it with --site-file once the cert is issued.
write_stage1_site() {
    cat > "$PRIMARY_SITE" <<NGINX
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
}

# Decide between stage-1 and final based on cert presence + skip-cert.
CERT_PATH="/etc/letsencrypt/live/$PUBLIC_HOST/fullchain.pem"
NEEDS_CERTBOT=0
if (( SKIP_CERT == 0 )) && [[ ! -e "$CERT_PATH" ]]; then
    NEEDS_CERTBOT=1
fi

if (( NEEDS_CERTBOT )); then
    echo "==> writing stage-1 site (HTTP-only, for ACME challenge)"
    write_stage1_site
else
    echo "==> installing $MODE site from $SITE_FILE"
    cp -f "$SITE_FILE" "$PRIMARY_SITE"
fi

# Activate the site file (Debian-style sites-enabled symlink).
if [[ "$LAYOUT" == "debian" ]]; then
    ln -sf "$PRIMARY_SITE" "$SITE_ENABLED_DIR/$(basename "$PRIMARY_SITE")"
    if [[ -L "$SITE_ENABLED_DIR/default" || -e "$SITE_ENABLED_DIR/default" ]]; then
        echo "==> removing $SITE_ENABLED_DIR/default to free :80"
        rm -f "$SITE_ENABLED_DIR/default"
    fi
fi

# Fedora/RHEL nginx.conf has its own :80 default_server. Demote it.
if [[ -f /etc/nginx/nginx.conf ]] && grep -qE '^\s*listen\s+(\[::\]:)?80\s+default_server' /etc/nginx/nginx.conf; then
    echo "==> demoting /etc/nginx/nginx.conf's :80 default_server"
    sed -i -E 's/^(\s*listen\s+(\[::\]:)?80)\s+default_server/\1/' /etc/nginx/nginx.conf
fi

# Make sure cert renewals reload nginx so the served fullchain rotates.
echo "==> installing certbot deploy hook to reload nginx on renewal"
install -d /etc/letsencrypt/renewal-hooks/deploy
cat > /etc/letsencrypt/renewal-hooks/deploy/reload-nginx.sh <<'EOF'
#!/bin/sh
systemctl reload nginx
EOF
chmod 0755 /etc/letsencrypt/renewal-hooks/deploy/reload-nginx.sh

echo "==> nginx -t"
nginx -t

# Debian's nginx package starts + enables the service at install;
# Fedora/RHEL/Arch install it disabled. enable --now is idempotent.
echo "==> ensuring nginx is running"
systemctl enable --now nginx

echo "==> reloading nginx"
systemctl reload nginx

if (( NEEDS_CERTBOT )); then
    echo "==> issuing TLS cert via certbot (--nginx)"
    # --nginx (full installer mode), not --webroot: certbot creates
    # /etc/letsencrypt/options-ssl-nginx.conf as a side effect of its
    # config-modification step, which collector configs `include`.
    # certbot's edits to our stage-1 are transient — the final cp
    # below overwrites them.
    certbot --nginx \
        --non-interactive --agree-tos \
        --email "$ADMIN_EMAIL" \
        -d "$PUBLIC_HOST"

    echo "==> installing final $MODE site from $SITE_FILE"
    cp -f "$SITE_FILE" "$PRIMARY_SITE"

    echo "==> nginx -t (final)"
    nginx -t
    echo "==> reloading nginx"
    systemctl reload nginx
fi

# Enable the auto-renewal timer. Debian's certbot package starts its own
# (certbot.timer) at install; Arch installs it disabled. Without this,
# certs expire silently in 90 days. Skip when --skip-cert (the operator
# is managing renewals from elsewhere — typically a hub migration where
# the old host is still doing renewals until the cutover).
if (( SKIP_CERT == 0 )); then
    echo "==> enabling certbot auto-renewal timer"
    for timer in certbot.timer certbot-renew.timer; do
        if systemctl list-unit-files --no-legend "$timer" 2>/dev/null | grep -q "$timer"; then
            if systemctl enable --now "$timer" >/dev/null 2>&1; then
                echo "    enabled $timer"
            else
                echo "    WARN: failed to enable $timer (cert auto-renewal will not run)" >&2
            fi
            break
        fi
    done
else
    echo "==> --skip-cert: leaving certbot.timer alone (operator manages cert renewals)"
fi

echo
case "$MODE" in
    hub)
        echo "Hub nginx ready. Public URL: https://$PUBLIC_HOST/"
        echo "  Fast-dl:     http://$PUBLIC_HOST:27970/"
        ;;
    collector)
        echo "Collector nginx ready. URLs the hub will fetch from this host:"
        echo "  Demos:       https://$PUBLIC_HOST/demos/<uuid>.tvd"
        echo "  Levelshots:  https://$PUBLIC_HOST/assets/levelshots/<map>.jpg"
        echo "  Demo pk3s:   https://$PUBLIC_HOST/demopk3s/maps/<map>.pk3"
        echo "  Fast-dl:     http://$PUBLIC_HOST:27970/"
        ;;
esac
