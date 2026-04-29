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
# This script ONLY sets up a collector — the common case, joining a
# Trinity server to the trinity.run hub network. Standing up your own
# hub is an expert path: install the binary by hand and run
# `sudo trinity init --allow-hub`. See docs/distributed-deployment.md.
#
# What this does NOT do: pick a mode, write any config, install the
# trinity-engine release, install systemd units, manage q3 server
# files. All of that is `trinity init`.

usage() {
    cat <<'EOF'
Usage: sudo ./scripts/install.sh [--from-source] [--release-tag VERSION] [--upgrade]

Options:
  --from-source            build trinity from this checkout instead of fetching a release
                           (only valid when running from a git checkout, not via curl|bash)
  --release-tag VERSION    pin a trinity-tracker release tag (default: latest)
  --upgrade                replace the binary on an existing install and restart
                           trinity.service. Skips the wizard. Requires /etc/trinity/config.yml.
  -h, --help               show this help
EOF
}

FROM_SOURCE=0
UPGRADE=0
RELEASE_TAG="latest"
TRACKER_REPO="ernie/trinity-tracker"

while (( $# )); do
    case "$1" in
        --from-source)        FROM_SOURCE=1; shift ;;
        --upgrade)            UPGRADE=1; shift ;;
        --release-tag)        RELEASE_TAG="$2"; shift 2 ;;
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
   - obtain a Let's Encrypt cert for the public hostname you give
   - open 80, 443, 27970/tcp and 27960-28000/udp on UFW or firewalld

 Designed for a fresh Debian/Ubuntu or Arch host. Coexisting with
 other services on the same box (especially other web servers or
 anything else holding :80/:443) may need manual cleanup. There is
 no built-in uninstall.

 You will need:
   - a public hostname pointing at this box (DNS already in place)
   - an admin email for Let's Encrypt renewal alerts
   - a hub source ID and .creds file from your hub admin
================================================================

BANNER
fi

echo "==> installing baseline OS packages"
# logrotate is missing from Arch's base install.
pkgs=(curl ca-certificates unzip screen logrotate)
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

# When run via `curl | sudo bash`, bash's stdin is the pipe from
# curl — so trinity init's TTY check would fail. Re-open stdin from
# /dev/tty (the controlling terminal sudo gave us, which exists even
# when stdin is a pipe). If there's no controlling terminal at all
# (e.g. ssh -T, container without a PTY), bail with the manual command
# rather than failing inside the wizard.
if [[ -r /dev/tty ]]; then
    exec </dev/tty env TRINITY_INIT_STAGE="$STAGE" /usr/local/bin/trinity init
fi
echo "No controlling terminal detected. Run the wizard manually:" >&2
echo "  sudo TRINITY_INIT_STAGE=\"$STAGE\" trinity init" >&2
exit 0
