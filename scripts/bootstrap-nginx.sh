#!/usr/bin/env bash
set -euo pipefail

# Sets up nginx and certbot for a hub or a collector. The trinity binary
# renders the site config from cmd/trinity/setup/nginxtemplates/ and
# passes it in with --site-file; this script installs nginx, opens
# firewall ports, puts the file in place, runs certbot and reloads.
#
# Modes:
#   --mode=hub       Public front for a hub: the website, api and ws,
#                    the asset fallback proxy, and dl.<host> fast downloads.
#   --mode=collector Asset host the hub redirects to (demos, levelshots,
#                    demo pk3s) plus dl.<host> fast downloads.
#
# nginx layout by distro:
#   - Debian/Ubuntu: sites-available/sites-enabled, python3-certbot-nginx
#   - Arch: conf.d/, certbot-nginx
#
# Collector mode is covered in section 6 of docs/collector-setup.md.

usage() {
    cat <<'EOF'
Usage:
  sudo bootstrap-nginx.sh \
      --mode=hub|collector \
      --hostname=<host> \
      --site-file=<path> \
      [--email=<addr>] \
      [--static-dir=<path>] \
      [--skip-cert]

Required:
  --mode=hub|collector   Which flavor of vhost to install.
  --hostname=<host>      Bare public hostname, such as trinity.run.
  --site-file=<path>     Rendered nginx site file (the trinity binary
                         writes it before running this script).
  --email=<addr>         Email certbot uses for renewal notices.
                         Required unless --skip-cert.

Optional:
  --static-dir=<path>    In collector mode, the demos/, assets/levelshots/
                         and demopk3s/ directories are created here,
                         owned by quake, so trinity demobake and
                         levelshots can write to them. Default
                         /var/lib/trinity/web.
  --skip-cert            Skip certbot.
                         /etc/letsencrypt/live/<host>/{fullchain,privkey}.pem
                         must already be in place, as in a hub
                         migration that copies /etc/letsencrypt from
                         the old host before DNS changes.
  --skip-firewall        Leave ufw and firewalld alone, for hosts whose
                         firewall is managed another way.
EOF
}

MODE=""
PUBLIC_HOST=""
ADMIN_EMAIL=""
SITE_FILE=""
STATIC_DIR="/var/lib/trinity/web"
SKIP_CERT=0
SKIP_FIREWALL=0

while [[ $# -gt 0 ]]; do
    case "$1" in
        --mode=*)        MODE="${1#*=}" ;;
        --hostname=*)    PUBLIC_HOST="${1#*=}" ;;
        --email=*)       ADMIN_EMAIL="${1#*=}" ;;
        --site-file=*)   SITE_FILE="${1#*=}" ;;
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
This script needs root to install nginx and certbot, write to /etc/nginx
and run certbot. It will run itself again with sudo, keeping your
arguments.

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
        --static-dir="$STATIC_DIR" \
        ${ADMIN_EMAIL:+--email="$ADMIN_EMAIL"} \
        $( (( SKIP_CERT     )) && echo --skip-cert ) \
        $( (( SKIP_FIREWALL )) && echo --skip-firewall )
fi

echo "==> mode=$MODE  host=$PUBLIC_HOST  skip-cert=$SKIP_CERT  skip-firewall=$SKIP_FIREWALL"

echo "==> installing nginx and certbot"
if command -v apt-get >/dev/null 2>&1; then
    apt-get update
    DEBIAN_FRONTEND=noninteractive apt-get install -y nginx certbot python3-certbot-nginx acl
elif command -v pacman >/dev/null 2>&1; then
    pacman -Sy --noconfirm nginx certbot certbot-nginx acl
else
    echo "ERROR: no supported package manager (apt or pacman) found." >&2
    exit 1
fi

# The hub redirects asset requests to these directories. Hub installs
# create their own in apply.go's ensureDirs.
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

# Let's Encrypt checks over HTTP on port 80, so the ports open before
# certbot runs. Hubs also open 4222/tcp for remote collectors.
open_firewall_ports() {
    local ports_tcp=(80 443)
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
    echo "==> no active ufw or firewalld found, so no local ports were opened"
    echo "    (open ${ports_tcp[*]}/tcp and 27960-28000/udp in any cloud firewall yourself)"
}
if (( SKIP_FIREWALL )); then
    echo "==> --skip-firewall: leaving ufw and firewalld alone"
else
    open_firewall_ports
fi

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

# One site file holds the HTTP redirect, the HTTPS vhost and the
# dl.<host> vhost.
if [[ "$LAYOUT" == "debian" ]]; then
    PRIMARY_SITE="$SITE_DIR/trinity"
else
    PRIMARY_SITE="$SITE_DIR/trinity.conf"
fi

# Before the certificate exists, certbot needs an HTTP site answering
# for both names; --site-file replaces it once the certificate is issued.
write_stage1_site() {
    cat > "$PRIMARY_SITE" <<NGINX
server {
    listen 80;
    listen [::]:80;
    server_name $PUBLIC_HOST dl.$PUBLIC_HOST;

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

CERT_PATH="/etc/letsencrypt/live/$PUBLIC_HOST/fullchain.pem"
NEEDS_CERTBOT=0
if (( SKIP_CERT == 0 )) && [[ ! -e "$CERT_PATH" ]]; then
    NEEDS_CERTBOT=1
fi

if (( NEEDS_CERTBOT )); then
    echo "==> writing stage-1 site (HTTP only, for the ACME challenge)"
    write_stage1_site
else
    echo "==> installing $MODE site from $SITE_FILE"
    cp -f "$SITE_FILE" "$PRIMARY_SITE"
fi

if [[ "$LAYOUT" == "debian" ]]; then
    ln -sf "$PRIMARY_SITE" "$SITE_ENABLED_DIR/$(basename "$PRIMARY_SITE")"
    if [[ -L "$SITE_ENABLED_DIR/default" || -e "$SITE_ENABLED_DIR/default" ]]; then
        echo "==> removing $SITE_ENABLED_DIR/default to free :80"
        rm -f "$SITE_ENABLED_DIR/default"
    fi
fi

# nginx serves a renewed certificate only after a reload.
echo "==> installing certbot deploy hook to reload nginx on renewal"
install -d /etc/letsencrypt/renewal-hooks/deploy
cat > /etc/letsencrypt/renewal-hooks/deploy/reload-nginx.sh <<'EOF'
#!/bin/sh
systemctl reload nginx
EOF
chmod 0755 /etc/letsencrypt/renewal-hooks/deploy/reload-nginx.sh

echo "==> nginx -t"
nginx -t

# Arch installs nginx disabled.
echo "==> ensuring nginx is running"
systemctl enable --now nginx

echo "==> reloading nginx"
systemctl reload nginx

if (( NEEDS_CERTBOT )); then
    echo "==> getting a TLS certificate with certbot (--nginx)"
    # --nginx writes /etc/letsencrypt/options-ssl-nginx.conf, which the
    # collector site file includes; the copy below replaces certbot's
    # edits to the stage-1 site. Both names must already resolve here.
    certbot --nginx \
        --non-interactive --agree-tos \
        --email "$ADMIN_EMAIL" \
        -d "$PUBLIC_HOST" \
        -d "dl.$PUBLIC_HOST"

    echo "==> installing final $MODE site from $SITE_FILE"
    cp -f "$SITE_FILE" "$PRIMARY_SITE"

    echo "==> nginx -t (final)"
    nginx -t
    echo "==> reloading nginx"
    systemctl reload nginx
fi

# Arch installs the certbot renewal timer disabled. With --skip-cert,
# renewals stay where the certificate came from, as in a hub migration
# where the old host renews until the cutover.
if (( SKIP_CERT == 0 )); then
    echo "==> enabling certbot auto-renewal timer"
    for timer in certbot.timer certbot-renew.timer; do
        if systemctl list-unit-files --no-legend "$timer" 2>/dev/null | grep -q "$timer"; then
            if systemctl enable --now "$timer" >/dev/null 2>&1; then
                echo "    enabled $timer"
            else
                echo "    WARN: couldn't enable $timer, so certificates won't renew automatically" >&2
            fi
            break
        fi
    done
else
    echo "==> --skip-cert: leaving the certbot renewal timer alone"
fi

echo
case "$MODE" in
    hub)
        echo "Hub nginx ready. Public URL: https://$PUBLIC_HOST/"
        echo "  Fast-dl:     https://dl.$PUBLIC_HOST/"
        ;;
    collector)
        echo "Collector nginx ready. URLs the hub will fetch from this host:"
        echo "  Demos:       https://$PUBLIC_HOST/demos/<uuid>.tvd"
        echo "  Levelshots:  https://$PUBLIC_HOST/assets/levelshots/<map>.jpg"
        echo "  Demo pk3s:   https://$PUBLIC_HOST/demopk3s/maps/<map>.pk3"
        echo "  Fast-dl:     https://dl.$PUBLIC_HOST/"
        ;;
esac
