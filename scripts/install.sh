#!/usr/bin/env bash
set -euo pipefail

# Bootstrap for `trinity init`: installs the few system packages Trinity
# needs, puts the prebuilt `trinity` binary in /usr/local/bin, then hands
# off to the wizard, which does everything else.
#
# Run it with curl:
#   curl -fsSL https://raw.githubusercontent.com/ernie/trinity-tracker/main/scripts/install.sh \
#       | sudo bash
#
# Or from a checkout:
#   sudo ./scripts/install.sh
#
# By default the host joins an existing hub as a collector. --allow-hub
# sets up a new hub here; see docs/distributed-deployment.md.

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
  --allow-hub              install a Trinity hub on this host; the wizard then offers
                           the Combined and HubOnly modes. Without it, the host joins
                           an existing hub as a collector.

Expert options (passed on to `trinity init`):
  --skip-cert              install nginx, render its config and reload it, but skip
                           certbot (for a host where /etc/letsencrypt/live/<host>/ is
                           already in place, as in a hub migration)
  --skip-firewall          leave ufw and firewalld alone, for hosts whose firewall is
                           managed another way (cloud dashboard, nftables and so on)
  --skip-nginx             leave nginx alone, for hosts running their own reverse proxy
                           (Caddy, Traefik, your own nginx and so on); implies
                           --skip-cert
  --skip-logrotate         skip /etc/logrotate.d/quake3, for hosts that rotate logs
                           another way (fluent-bit, vector, journald only)

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
    echo "ERROR: --upgrade needs an existing install; /etc/trinity/config.yml not found." >&2
    echo "       Drop --upgrade to do a fresh install." >&2
    exit 1
fi

if [[ $EUID -ne 0 ]]; then
    # Under curl|bash, $0 is "bash" and the script source can't be
    # re-read, so only a run from a checkout can re-exec itself.
    if [[ ! -f "$0" ]]; then
        echo "ERROR: this script needs root, and a piped curl|bash run can't" >&2
        echo "       restart itself under sudo. Run it again with sudo before" >&2
        echo "       bash:" >&2
        echo "         curl ... | sudo bash -s -- $*" >&2
        exit 1
    fi
    echo "This script needs root to install packages and write to /usr/local/bin." >&2
    echo "Running again with sudo…" >&2
    exec sudo -E "$0" "$@"
fi

# Under curl|bash, $0 is "bash" or "/dev/stdin" and there's no checkout.
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

if (( ! UPGRADE )); then
    if (( ALLOW_HUB )); then
        cat <<'BANNER'

================================================================
 Trinity Hub Installer (--allow-hub)
================================================================
 This script sets up a new Trinity hub on this host. The hub
 serves the website, the API and the live WebSocket feed. If you
 tell the wizard to expect remote collectors, it also runs a NATS
 broker on :4222, with TLS, for them to connect to.

 It will:

   - install OS packages (nginx, certbot, logrotate and a few more)
   - put the trinity binary in /usr/local/bin
   - download the Trinity engine release into /usr/lib/quake3/
   - create the 'quake' service user
   - write /etc/trinity/config.yml and systemd units
     (trinity.service, quake3-server@.service, quake3-servers.target)
   - get one Let's Encrypt certificate covering <hostname> and
     dl.<hostname> (skip it with --skip-cert if /etc/letsencrypt/
     is already in place, as in a host migration)
   - open 80, 443, 4222/tcp and 27960-28000/udp in ufw or firewalld

 It expects a fresh Debian, Ubuntu or Arch host. Other services on
 the same box, especially other web servers or anything else on
 :80 or :443, may need cleaning up by hand. There's no uninstaller.

 You will need:
   - a public hostname that already resolves to this box, plus a
     dl.<hostname> A or AAAA record for the same box
   - an email address for Let's Encrypt renewal notices
     (not asked with --skip-cert or --skip-nginx)

 Optional, to skip a path prompt during the pak install:
   - your retail Quake III Arena pak0.pk3 in the current directory
     as q3-pak0.pk3, and your Team Arena pak0.pk3 as mp-pak0.pk3
     if you'll run Team Arena game types
   - quake3-1.32-pk3s.zip in the current directory, if you have the
     1.32 patch bundle. The hub copies it into
     /var/lib/trinity/web/downloads/ so collectors can fetch it
     from here.
 To handle some of these steps yourself (your own reverse proxy,
 host firewall or log rotation), stop now and run this again with
 any of:
   --skip-cert        (skip certbot; put /etc/letsencrypt/ in place
                       yourself, as in a host migration)
   --skip-nginx       (skip nginx and Let's Encrypt; run your own
                       reverse proxy and TLS)
   --skip-firewall    (leave ufw and firewalld alone; manage the
                       firewall another way)
   --skip-logrotate   (skip /etc/logrotate.d/quake3; rotate logs
                       another way)
 See docs/distributed-deployment.md for running a hub.
================================================================

BANNER
    else
        cat <<'BANNER'

================================================================
 Trinity Collector Installer
================================================================
 This script connects your Quake III Arena host to a Trinity hub
 as a collector. It will:

   - install OS packages (nginx, certbot, logrotate and a few more)
   - put the trinity binary in /usr/local/bin
   - download the Trinity engine release into /usr/lib/quake3/
   - create the 'quake' service user
   - write /etc/trinity/{config.yml,source.creds} and systemd units
     (trinity.service, quake3-server@.service, quake3-servers.target)
   - get one Let's Encrypt certificate covering <hostname> and
     dl.<hostname>
   - open 80, 443/tcp and 27960-28000/udp in ufw or firewalld

 It expects a fresh Debian, Ubuntu or Arch host. Other services on
 the same box, especially other web servers or anything else on
 :80 or :443, may need cleaning up by hand. There's no uninstaller.

 You will need:
   - a public hostname that already resolves to this box, plus a
     dl.<hostname> A or AAAA record for the same box
   - an email address for Let's Encrypt renewal notices
   - a source ID and .creds file: log in on the hub's website and
     click "Add servers" (or "My servers", then "Add another
     source"). Once an admin approves your request, your source in
     "My servers" shows its name and a .creds download.

 Optional, to skip a path prompt during the pak install:
   - your retail Quake III Arena pak0.pk3 in the current directory
     as q3-pak0.pk3, and your Team Arena pak0.pk3 as mp-pak0.pk3
     if you'll run Team Arena game types

 To handle some of these steps yourself (your own reverse proxy,
 host firewall or log rotation), stop now and run this again with
 any of:
   --skip-nginx       (skip nginx and Let's Encrypt; run your own
                       reverse proxy and TLS)
   --skip-firewall    (leave ufw and firewalld alone; manage the
                       firewall another way)
   --skip-logrotate   (skip /etc/logrotate.d/quake3; rotate logs
                       another way)
 See docs/collector-setup.md for doing each of these by hand.
================================================================

BANNER
    fi

    # Read from /dev/tty since stdin is the script itself under curl|bash.
    if [[ -r /dev/tty ]]; then
        read -r -p "Proceed with installation? [y/N] " reply </dev/tty || reply=""
    else
        echo "ERROR: no controlling tty to confirm; run it again interactively." >&2
        exit 1
    fi
    case "$reply" in
        [yY]|[yY][eE][sS]) ;;
        *) echo "Aborted."; exit 0 ;;
    esac
fi

echo "==> installing baseline OS packages"
# Arch's base install lacks logrotate. bootstrap-nginx.sh installs nginx
# and certbot.
pkgs=(curl ca-certificates unzip)
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
    echo "ERROR: no supported package manager (apt or pacman) found." >&2
    echo "       Trinity supports Debian, Ubuntu and Arch. To try another" >&2
    echo "       platform, install these yourself and run this again from" >&2
    echo "       a checkout: ${pkgs[*]}" >&2
    exit 1
fi

# mise/asdf/manual go installs aren't on root's PATH under sudo.
if [[ -z "${GO_BIN:-}" ]] && command -v go >/dev/null 2>&1; then
    GO_BIN="$(command -v go)"
fi

# The final exec replaces this shell, so `trinity init` removes the
# stage (named in TRINITY_INIT_STAGE) once it has used web/ inside it.
STAGE="$(mktemp -d -p /tmp trinity-install.XXXXXX)"
chmod 0755 "$STAGE"

# Removes the stage if anything fails before the exec.
cleanup_stage() { rm -rf "$STAGE"; }
trap cleanup_stage ERR

if (( FROM_SOURCE )); then
    if [[ -z "${GO_BIN:-}" ]]; then
        echo "ERROR: --from-source requires Go but none was found." >&2
        echo "       Install Go (https://go.dev/dl/) and re-run with GO_BIN=/path/to/go." >&2
        exit 1
    fi
    # Distros often ship a Go a release or two older than go.mod needs.
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

    # Verified before anything in the tarball is extracted or run.
    echo "==> verifying checksum"
    if ! curl -fsSL --output "$STAGE/sha256sums.txt" "$base/sha256sums.txt"; then
        echo "ERROR: couldn't fetch $base/sha256sums.txt" >&2
        echo "       The selected release predates checksum verification." >&2
        echo "       Pick a newer release with --release-tag <vX.Y.Z>." >&2
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

    # The tarball holds a single trinity-linux-<arch>/ directory.
    tar -C "$STAGE" -xzf "$STAGE/$asset" --strip-components=1
    rm -f "$STAGE/$asset"
fi

# Writing over a running binary fails with ETXTBSY.
if (( UPGRADE )); then
    echo "==> stopping trinity.service before binary swap"
    systemctl stop trinity.service || true
fi
install -m 0755 "$STAGE/trinity" /usr/local/bin/trinity
rm -f "$STAGE/trinity"

if (( UPGRADE )); then
    # Only a hub serves the web bundle, and only hub configs have a
    # tracker.hub block.
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

# The wizard's flag parsing makes --skip-nginx imply --skip-cert.
init_args=(init)
(( ALLOW_HUB ))      && init_args+=(--allow-hub)
(( SKIP_CERT ))      && init_args+=(--skip-cert)
(( SKIP_FIREWALL ))  && init_args+=(--skip-firewall)
(( SKIP_NGINX ))     && init_args+=(--skip-nginx)
(( SKIP_LOGROTATE )) && init_args+=(--skip-logrotate)

# Under `curl | sudo bash`, stdin is the pipe from curl, and the wizard
# needs a terminal, so stdin comes from /dev/tty. With no controlling
# terminal at all (ssh -T, a container without a PTY), print the command.
if [[ -r /dev/tty ]]; then
    exec </dev/tty env TRINITY_INIT_STAGE="$STAGE" /usr/local/bin/trinity "${init_args[@]}"
fi
echo "No controlling terminal detected. Run the wizard manually:" >&2
echo "  sudo TRINITY_INIT_STAGE=\"$STAGE\" trinity ${init_args[*]}" >&2
exit 0
