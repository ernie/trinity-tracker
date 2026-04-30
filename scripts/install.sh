#!/usr/bin/env bash
set -euo pipefail

# Tiny bootstrap for `trinity init`. Installs the few system packages
# Trinity needs, drops the prebuilt `trinity` binary on /usr/local/bin,
# then hands off to the collector-only wizard.
#
# Designed for curl|bash:
#   curl -fsSL https://raw.githubusercontent.com/ernie/trinity-tracker/main/scripts/install.sh \
#       | sudo bash
#
# Or from a checkout:
#   sudo ./scripts/install.sh
#
# Use --from-source to build from a local checkout instead of fetching
# the prebuilt release (only meaningful when run from a working tree).
#
# Default mode: collector — joining a Quake 3 host to an existing
# Trinity hub. Pass --allow-hub to stand up a new hub on this host
# instead (the wizard then offers Combined and HubOnly modes). See
# docs/distributed-deployment.md for hub-mode notes.
#
# What this does NOT do: pick a mode, write any config, install the
# trinity-engine release, install systemd units, manage q3 server
# files. All of that is `trinity init`.

usage() {
    cat <<'EOF'
Usage: sudo ./scripts/install.sh [--from-source] [--release-tag VERSION] [--upgrade]
                                 [--allow-hub]
                                 [--skip-cert] [--skip-firewall]
                                 [--skip-nginx] [--skip-logrotate]

Options:
  --from-source            build trinity from this checkout instead of fetching a release
                           (only valid when running from a git checkout, not via curl|bash)
  --release-tag VERSION    pin a trinity-tracker release tag (default: latest)
  --upgrade                replace the binary on an existing install and restart
                           trinity.service. Skips the wizard. Requires /etc/trinity/config.yml.
  --allow-hub              install a Trinity hub on this host instead of a collector.
                           Enables Combined and HubOnly modes in the wizard. Default
                           (without this flag) is collector — joining an existing hub.

Expert-mode skips (forwarded to `trinity init`):
  --skip-cert              install nginx + render config + reload, but skip the certbot
                           run (caller has staged /etc/letsencrypt/live/<host>/ already;
                           used during hub migrations)
  --skip-firewall          do not poke ufw/firewalld; operator manages host firewall via
                           cloud dashboard, nftables, or config-management
  --skip-nginx             do not install or configure nginx; operator runs their own
                           reverse proxy (Caddy, Traefik, manual nginx, etc.).
                           Implies --skip-cert in the wizard.
  --skip-logrotate         do not write /etc/logrotate.d/quake3; operator manages log
                           rotation via fluent-bit, vector, journald-only, etc.

  -h, --help               show this help
EOF
}

FROM_SOURCE=0
UPGRADE=0
ALLOW_HUB=0
SKIP_CERT=0
SKIP_FIREWALL=0
SKIP_NGINX=0
SKIP_LOGROTATE=0
RELEASE_TAG="latest"
TRACKER_REPO="ernie/trinity-tracker"

while (( $# )); do
    case "$1" in
        --from-source)        FROM_SOURCE=1; shift ;;
        --upgrade)            UPGRADE=1; shift ;;
        --release-tag)        RELEASE_TAG="$2"; shift 2 ;;
        --allow-hub)          ALLOW_HUB=1; shift ;;
        --skip-cert)          SKIP_CERT=1; shift ;;
        --skip-firewall)      SKIP_FIREWALL=1; shift ;;
        --skip-nginx)         SKIP_NGINX=1; shift ;;
        --skip-logrotate)     SKIP_LOGROTATE=1; shift ;;
        -h|--help)            usage; exit 0 ;;
        *) echo "Unknown option: $1" >&2; usage >&2; exit 1 ;;
    esac
done

if (( UPGRADE )) && [[ ! -f /etc/trinity/config.yml ]]; then
    echo "ERROR: --upgrade needs an existing install — /etc/trinity/config.yml not found." >&2
    echo "       Drop --upgrade to do a fresh install." >&2
    exit 1
fi

if [[ $EUID -ne 0 ]]; then
    echo "This script needs root to install packages and write to /usr/local/bin." >&2
    echo "Re-running under sudo..." >&2
    exec sudo -E "$0" "$@"
fi

# When run via curl|bash, $0 is "bash" or "/dev/stdin" and there's no
# checkout to refer to. Only resolve a SRC_DIR when we plausibly have one.
if [[ -f "$0" ]]; then
    HERE="$(cd "$(dirname "$0")" && pwd)"
    SRC_DIR="$(cd "$HERE/.." 2>/dev/null && pwd)"
else
    SRC_DIR=""
fi

if (( FROM_SOURCE )) && [[ -z "$SRC_DIR" || ! -f "$SRC_DIR/go.mod" ]]; then
    echo "ERROR: --from-source needs a git checkout (no go.mod found via $0)." >&2
    echo "       Either clone the repo and run scripts/install.sh from it, or" >&2
    echo "       drop --from-source to fetch the prebuilt release." >&2
    exit 1
fi

# Banner — print once on a fresh install. --upgrade just swaps the
# binary, so skip it there; the operator already knows what trinity
# is by the time they're upgrading.
if (( ! UPGRADE )); then
    if (( ALLOW_HUB )); then
        cat <<'BANNER'

================================================================
 Trinity Hub Installer (--allow-hub)
================================================================
 This script stands up a new Trinity hub on this host. The hub
 serves the SPA, the API, and the WebSocket feed; if you tell the
 wizard to expect remote collectors, it also exposes an embedded
 NATS broker on :4222 with TLS for collectors to connect to.

 It will:

   - install OS packages (nginx, certbot, screen, logrotate, ...)
   - drop /usr/local/bin/trinity onto this host
   - download the trinity-engine release into /usr/lib/quake3/
   - create the 'quake' service user
   - write /etc/trinity/config.yml and systemd units
     (trinity.service, quake3-server@.service, quake3-servers.target)
   - obtain a Let's Encrypt SAN cert for both <hostname> and
     dl.<hostname> (skip with --skip-cert if you've pre-staged
     /etc/letsencrypt/ yourself, e.g. for a host migration)
   - open 80, 443, 4222/tcp and 27960-28000/udp on UFW or firewalld

 Designed for a fresh Debian/Ubuntu or Arch host. Coexisting with
 other services on the same box (especially other web servers or
 anything else holding :80/:443) may need manual cleanup. There is
 no built-in uninstall.

 You will need:
   - a public hostname pointing at this box (DNS already in place),
     plus a dl.<hostname> A/AAAA record pointing at the same host
   - an admin email for Let's Encrypt renewal alerts
     (not asked when --skip-cert or --skip-nginx is set)

 Optional (saves a path-prompt during pak install):
   - retail Quake 3 pak0.pk3 copied into the current directory as
     q3-pak0.pk3, plus mp-pak0.pk3 if you'll run Team Arena gametypes
   - quake3-1.32-pk3s.zip in the current directory if you have the
     1.32 patch bundle on hand. Hub installs mirror it into
     /var/lib/trinity/web/downloads/ so collectors fetch it from
     this hub instead of needing their own copy.
   - hqq-baseq3.zip (and hqq-missionpack.zip if running TA) in the
     current directory to install High Quality Quake assets for
     sharper levelshots and player portraits.

 If you'd rather manage some of these steps yourself — your own
 reverse proxy, host firewall, or log rotation — abort now and
 re-run with one or more of:
   --skip-cert        (skip certbot; pre-stage /etc/letsencrypt/
                       yourself, e.g. for a host migration)
   --skip-nginx       (skip nginx + Let's Encrypt; manage your own
                       reverse proxy and TLS)
   --skip-firewall    (skip ufw/firewalld; manage host firewall via
                       your cloud dashboard, nftables, etc.)
   --skip-logrotate   (skip /etc/logrotate.d/quake3; ship logs via
                       fluent-bit/vector/journald-only)
 See docs/distributed-deployment.md for hub-mode notes.
================================================================

BANNER
    else
        cat <<'BANNER'

================================================================
 Trinity Collector Installer
================================================================
 This script joins your Quake 3 host to a Trinity hub network.
 It will:

   - install OS packages (nginx, certbot, screen, logrotate, ...)
   - drop /usr/local/bin/trinity onto this host
   - download the trinity-engine release into /usr/lib/quake3/
   - create the 'quake' service user
   - write /etc/trinity/{config.yml,source.creds} and systemd units
     (trinity.service, quake3-server@.service, quake3-servers.target)
   - obtain a Let's Encrypt SAN cert for both <hostname> and
     dl.<hostname>
   - open 80, 443/tcp and 27960-28000/udp on UFW or firewalld

 Designed for a fresh Debian/Ubuntu or Arch host. Coexisting with
 other services on the same box (especially other web servers or
 anything else holding :80/:443) may need manual cleanup. There is
 no built-in uninstall.

 You will need:
   - a public hostname pointing at this box (DNS already in place),
     plus a dl.<hostname> A/AAAA record pointing at the same host
   - an admin email for Let's Encrypt renewal alerts
   - a hub source ID and .creds file from your hub admin

 Optional (saves a path-prompt during pak install):
   - retail Quake 3 pak0.pk3 copied into the current directory as
     q3-pak0.pk3, plus mp-pak0.pk3 if you'll run Team Arena gametypes

 If you'd rather manage some of these steps yourself — your own
 reverse proxy, host firewall, or log rotation — abort now and
 re-run with one or more of:
   --skip-nginx       (skip nginx + Let's Encrypt; manage your own
                       reverse proxy and TLS)
   --skip-firewall    (skip ufw/firewalld; manage host firewall via
                       your cloud dashboard, nftables, etc.)
   --skip-logrotate   (skip /etc/logrotate.d/quake3; ship logs via
                       fluent-bit/vector/journald-only)
 See docs/collector-setup.md for the manual equivalents of each.
================================================================

BANNER
    fi

    # Read from /dev/tty since stdin is the script itself under curl|bash.
    if [[ -r /dev/tty ]]; then
        read -r -p "Proceed with installation? [y/N] " reply </dev/tty || reply=""
    else
        echo "ERROR: no controlling tty to confirm — re-run interactively." >&2
        exit 1
    fi
    case "$reply" in
        [yY]|[yY][eE][sS]) ;;
        *) echo "Aborted."; exit 0 ;;
    esac
fi

echo "==> installing baseline OS packages"
# logrotate is missing from Arch's base install. Skip it if the operator
# has opted out — they're shipping logs via fluent-bit/vector/journald
# and don't need it. nginx + certbot are handled separately by
# bootstrap-nginx.sh, so --skip-nginx doesn't trim anything here.
pkgs=(curl ca-certificates unzip screen)
if (( ! SKIP_LOGROTATE )); then
    pkgs+=(logrotate)
fi
if (( FROM_SOURCE )) && ! command -v go >/dev/null 2>&1; then
    pkgs+=(golang)
fi
if command -v apt-get >/dev/null 2>&1; then
    apt-get update
    DEBIAN_FRONTEND=noninteractive apt-get install -y "${pkgs[@]}"
elif command -v pacman >/dev/null 2>&1; then
    pacman -Sy --noconfirm "${pkgs[@]}"
else
    echo "ERROR: no supported package manager (apt/pacman) found." >&2
    echo "       Trinity supports Debian/Ubuntu and Arch. Fedora/RHEL are" >&2
    echo "       unsupported — SELinux confines the q3 server unit's screen" >&2
    echo "       wrapper. If you're on one of those and know SELinux, install" >&2
    echo "       these manually and re-run with the wizard: ${pkgs[*]}" >&2
    exit 1
fi

# mise/asdf/manual go installs aren't on root's PATH under sudo.
if [[ -z "${GO_BIN:-}" ]] && command -v go >/dev/null 2>&1; then
    GO_BIN="$(command -v go)"
fi

# mktemp -p /tmp keeps staging off the host's real filesystem. We
# do NOT trap-cleanup: the script exec's trinity init at the end, so
# the EXIT trap would never fire — instead we hand the dir off via
# TRINITY_INIT_STAGE and let `trinity init` rm it once the actuator
# has consumed the web/ directory inside.
STAGE="$(mktemp -d -p /tmp trinity-install.XXXXXX)"
chmod 0755 "$STAGE"

# Bail-out cleanup: if anything before the final exec fails, this
# trap fires and removes the stage. The exec replaces the shell, so
# the trap is no-op once we've handed control to trinity.
cleanup_stage() { rm -rf "$STAGE"; }
trap cleanup_stage ERR

if (( FROM_SOURCE )); then
    if [[ -z "${GO_BIN:-}" ]]; then
        echo "ERROR: --from-source requires Go but none was found." >&2
        echo "       Install Go (https://go.dev/dl/) and re-run with GO_BIN=/path/to/go." >&2
        exit 1
    fi
    # Cross-check Go version against go.mod. Distros frequently ship
    # something a release or two behind.
    required_go="$(awk '/^go [0-9]/{print $2; exit}' "$SRC_DIR/go.mod")"
    if [[ -n "$required_go" ]]; then
        current_go="$("$GO_BIN" env GOVERSION 2>/dev/null | sed 's/^go//')"
        older="$(printf '%s\n%s\n' "$required_go" "$current_go" | sort -V | head -1)"
        if [[ "$older" != "$required_go" ]]; then
            echo "ERROR: go $current_go at $GO_BIN is older than go.mod's required $required_go." >&2
            exit 1
        fi
    fi
    echo "==> building trinity from source ($SRC_DIR)"
    ( cd "$SRC_DIR" && \
        GOPATH="$STAGE/go" GOCACHE="$STAGE/go-cache" \
        "$GO_BIN" build -ldflags "-X main.version=installer-$(date -u +%Y%m%d)" \
        -o "$STAGE/trinity" ./cmd/trinity )
    # Drop the Go build caches now — only trinity + the web/ subdir live on.
    rm -rf "$STAGE/go" "$STAGE/go-cache"
    if [[ -d "$SRC_DIR/web/dist" ]]; then
        cp -r "$SRC_DIR/web/dist" "$STAGE/web"
    fi
else
    case "$(uname -m)" in
        x86_64|amd64)  arch=amd64 ;;
        aarch64|arm64) arch=arm64 ;;
        armv7l|armv6l) arch=arm   ;;
        *) echo "ERROR: unsupported arch $(uname -m); use --from-source from a checkout" >&2; exit 1 ;;
    esac
    asset="trinity-linux-${arch}.tar.gz"
    if [[ "$RELEASE_TAG" == "latest" ]]; then
        base="https://github.com/${TRACKER_REPO}/releases/latest/download"
    else
        base="https://github.com/${TRACKER_REPO}/releases/download/${RELEASE_TAG}"
    fi
    echo "==> fetching $base/$asset"
    curl -fsSL --output "$STAGE/$asset" "$base/$asset"

    # Verify the tarball against sha256sums.txt before we extract it
    # and run anything from inside. The release pipeline publishes the
    # manifest alongside the tarballs starting with the v0.x release
    # that introduced this check; if the file is missing the release
    # predates checksum verification and the operator should pin a
    # newer --release-tag.
    echo "==> verifying checksum"
    if ! curl -fsSL --output "$STAGE/sha256sums.txt" "$base/sha256sums.txt"; then
        echo "ERROR: could not fetch $base/sha256sums.txt" >&2
        echo "       The selected release predates checksum verification." >&2
        echo "       Pin a newer release via --release-tag <vX.Y.Z>." >&2
        exit 1
    fi
    expected="$(awk -v a="$asset" '$2 == a || $2 == "*"a {print $1; exit}' "$STAGE/sha256sums.txt")"
    if [[ -z "$expected" ]]; then
        echo "ERROR: $asset not listed in sha256sums.txt" >&2
        exit 1
    fi
    actual="$(sha256sum "$STAGE/$asset" | awk '{print $1}')"
    if [[ "$expected" != "$actual" ]]; then
        echo "ERROR: checksum mismatch for $asset" >&2
        echo "       expected $expected" >&2
        echo "       got      $actual" >&2
        exit 1
    fi
    rm -f "$STAGE/sha256sums.txt"

    # --strip-components=1 flattens the trinity-linux-<arch>/ wrapper
    # the release pipeline writes; the tarball's top-level entry is
    # always exactly one directory.
    tar -C "$STAGE" -xzf "$STAGE/$asset" --strip-components=1
    rm -f "$STAGE/$asset"
fi

# Stop the running binary before swapping it (text file busy otherwise).
if (( UPGRADE )); then
    echo "==> stopping trinity.service before binary swap"
    systemctl stop trinity.service || true
fi
install -m 0755 "$STAGE/trinity" /usr/local/bin/trinity
rm -f "$STAGE/trinity"

if (( UPGRADE )); then
    # Hub installs: overlay web/dist so the browser app matches the new
    # server. Collector-only configs have no tracker.hub block.
    if grep -qE '^[[:space:]]+hub:[[:space:]]*$' /etc/trinity/config.yml \
        && [[ -d "$STAGE/web" ]]; then
        static_dir="$(awk '/static_dir:/ {print $2; exit}' /etc/trinity/config.yml)"
        if [[ -n "$static_dir" && -d "$static_dir" ]]; then
            echo "==> overlaying web bundle into $static_dir"
            cp -r "$STAGE/web/." "$static_dir/"
            svc_user="$(awk '/service_user:/ {print $2; exit}' /etc/trinity/config.yml)"
            [[ -n "$svc_user" ]] && chown -R "$svc_user:$svc_user" "$static_dir"
        fi
    fi
    echo "==> starting trinity.service"
    systemctl start trinity.service
    rm -rf "$STAGE"
    echo
    echo "Upgrade complete. $(/usr/local/bin/trinity version 2>/dev/null || echo 'trinity binary installed')."
    exit 0
fi

echo "==> /usr/local/bin/trinity installed; handing off to the wizard"
echo
trap - ERR

# Forward mode + expert-mode skips to the wizard. Built up here so
# the exec line stays readable. --skip-nginx implies --skip-cert in
# the wizard (no nginx == no LE plumbing); main.go's flag parser
# handles that.
init_args=(init)
(( ALLOW_HUB ))      && init_args+=(--allow-hub)
(( SKIP_CERT ))      && init_args+=(--skip-cert)
(( SKIP_FIREWALL ))  && init_args+=(--skip-firewall)
(( SKIP_NGINX ))     && init_args+=(--skip-nginx)
(( SKIP_LOGROTATE )) && init_args+=(--skip-logrotate)

# When run via `curl | sudo bash`, bash's stdin is the pipe from
# curl — so trinity init's TTY check would fail. Re-open stdin from
# /dev/tty (the controlling terminal sudo gave us, which exists even
# when stdin is a pipe). If there's no controlling terminal at all
# (e.g. ssh -T, container without a PTY), bail with the manual command
# rather than failing inside the wizard.
if [[ -r /dev/tty ]]; then
    exec </dev/tty env TRINITY_INIT_STAGE="$STAGE" /usr/local/bin/trinity "${init_args[@]}"
fi
echo "No controlling terminal detected. Run the wizard manually:" >&2
echo "  sudo TRINITY_INIT_STAGE=\"$STAGE\" trinity ${init_args[*]}" >&2
exit 0
