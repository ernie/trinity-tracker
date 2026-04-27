#!/usr/bin/env bash
set -euo pipefail

# Installs the Trinity stack (engine + tracker + systemd units) on a
# fresh Linux host. See docs/collector-setup.md for the full walkthrough.

usage() {
    cat <<'EOF'
Usage: sudo SOURCE_NAME=... PUBLIC_URL=... CREDS_FILE=... \
       ./scripts/install-collector.sh

Required env:
  SOURCE_NAME    Admin-chosen source name your hub knows you by
                 (matches the .creds file's subject).
  PUBLIC_URL     Publicly-reachable https URL for this host. Hostname is
                 advertised to the hub as your q3 server address and used
                 as the demo download base.
  CREDS_FILE     Path to the source.creds file the hub admin issued.

Optional env:
  HUB_HOST       Bare hostname of the trinity hub (default: trinity.run).
  ENGINE_VERSION Pin a trinity-engine release tag (default: latest).

What it does: apt-installs deps, creates the `quake` user, unpacks the
trinity-engine release into /usr/lib/quake3/, builds trinity-tracker from
this checkout into /usr/local/bin/trinity, installs the .creds file at
/etc/trinity/source.creds, drops /etc/trinity/config.yml.example, installs
systemd units, and installs /etc/logrotate.d/quake3.

What it does NOT do: start any service, generate per-server cfg/env
files, supply a retail pak0.pk3, configure nginx (see bootstrap-nginx.sh),
or provision the source on the hub (hub admin's job).
EOF
}

SOURCE_NAME="${SOURCE_NAME:-}"
PUBLIC_URL="${PUBLIC_URL:-}"
CREDS_FILE="${CREDS_FILE:-}"

missing=()
[[ -z "$SOURCE_NAME" ]] && missing+=("SOURCE_NAME")
[[ -z "$PUBLIC_URL"  ]] && missing+=("PUBLIC_URL")
[[ -z "$CREDS_FILE"  ]] && missing+=("CREDS_FILE")
if (( ${#missing[@]} )); then
    echo "ERROR: missing required env: ${missing[*]}" >&2
    echo >&2
    usage >&2
    exit 1
fi

HUB_HOST="${HUB_HOST:-trinity.run}"
ENGINE_VERSION="${ENGINE_VERSION:-latest}"
ENGINE_REPO="ernie/trinity-engine"
INSTALL_DIR="/usr/lib/quake3"
LOG_DIR="/var/log/quake3"

HERE="$(cd "$(dirname "$0")" && pwd)"
SRC_DIR="${SRC_DIR:-$(cd "$HERE/.." && pwd)}"

# mise/asdf/manual go installs aren't on root's PATH under sudo; capture
# the absolute path here and pass it through.
if [[ -z "${GO_BIN:-}" ]] && command -v go >/dev/null 2>&1; then
    GO_BIN="$(command -v go)"
fi

if [[ $EUID -ne 0 ]]; then
    cat <<EOF
This script needs root to install packages, create system users, write
to /usr/lib, /etc, /var/lib, and /etc/systemd/system. It will re-exec
itself under sudo, preserving the env vars above.

EOF
    read -r -p "Proceed with sudo? [y/N] " reply
    case "$reply" in
        y|Y|yes|YES) ;;
        *) echo "Aborted." >&2; exit 1 ;;
    esac
    exec sudo -E SOURCE_NAME="$SOURCE_NAME" PUBLIC_URL="$PUBLIC_URL" \
        CREDS_FILE="$CREDS_FILE" HUB_HOST="$HUB_HOST" \
        ENGINE_VERSION="$ENGINE_VERSION" SRC_DIR="$SRC_DIR" \
        GO_BIN="${GO_BIN:-}" \
        "$0" "$@"
fi

if [[ ! -f "$CREDS_FILE" ]]; then
    echo "ERROR: CREDS_FILE not found: $CREDS_FILE" >&2
    echo "Get a .creds file from the hub admin (they provision the source)." >&2
    exit 1
fi

# The .creds file carries an NKey + JWT. Warn if its source path is
# group- or world-accessible — the bytes have already leaked, but the
# operator may want to rotate before continuing.
creds_mode="$(stat -c '%a' "$CREDS_FILE" 2>/dev/null || true)"
if [[ -n "$creds_mode" ]] && (( 8#$creds_mode & 077 )); then
    echo "WARN: $CREDS_FILE has group/world-accessible perms ($creds_mode) — the secret may have leaked at rest." >&2
    echo "      Consider asking the hub admin to rotate, or at minimum: chmod 0600 $CREDS_FILE" >&2
fi

# --- Map host arch to engine release asset -----------------------------
case "$(uname -m)" in
    x86_64|amd64)    ARCH_ASSET="trinity-linux-x86_64.zip"; ARCH_BIN="trinity.ded.x86_64" ;;
    aarch64|arm64)   ARCH_ASSET="trinity-linux-arm64.zip" ; ARCH_BIN="trinity.ded.aarch64" ;;
    armv7l)          ARCH_ASSET="trinity-linux-armv7.zip" ; ARCH_BIN="trinity.ded.armv7l"  ;;
    i386|i686)       ARCH_ASSET="trinity-linux-x86.zip"   ; ARCH_BIN="trinity.ded.x86"    ;;
    *) echo "ERROR: unsupported arch $(uname -m); see ${ENGINE_REPO} releases" >&2; exit 1 ;;
esac

echo "==> installing OS packages"
pkgs=(curl ca-certificates make unzip)
if [[ -z "${GO_BIN:-}" ]]; then
    pkgs+=(golang)
else
    echo "    using existing go at $GO_BIN (skipping golang package)"
fi
apt-get update
DEBIAN_FRONTEND=noninteractive apt-get install -y "${pkgs[@]}"
GO_BIN="${GO_BIN:-$(command -v go)}"

# Cross-check Go version against go.mod's minimum. Distros frequently
# ship something a release or two behind; Go's auto-toolchain will try
# to download a newer one but only if outbound HTTPS to proxy.golang.org
# is open, which isn't a guaranteed prereq for the collector host.
required_go="$(awk '/^go [0-9]/{print $2; exit}' "$SRC_DIR/go.mod")"
current_go="$("$GO_BIN" env GOVERSION 2>/dev/null | sed 's/^go//')"
if [[ -n "$required_go" && -n "$current_go" ]]; then
    older="$(printf '%s\n%s\n' "$required_go" "$current_go" | sort -V | head -1)"
    if [[ "$older" != "$required_go" ]]; then
        echo "ERROR: go $current_go at $GO_BIN is older than go.mod's required $required_go." >&2
        echo "       Install a newer Go (https://go.dev/dl/) and re-run with GO_BIN=/path/to/go." >&2
        exit 1
    fi
fi

echo "==> creating quake user"
if ! id quake &>/dev/null; then
    useradd -r -m -d /home/quake -s /bin/bash quake
fi

echo "==> creating /etc/trinity, /var/lib/trinity, /var/log/quake3"
install -d -o root  -g quake -m 0750 /etc/trinity
install -d -o quake -g quake "$LOG_DIR" "/var/lib/trinity" \
    /var/lib/trinity/web/demos \
    /var/lib/trinity/web/assets/levelshots \
    /var/lib/trinity/web/demopk3s/maps

echo "==> downloading trinity-engine release ($ENGINE_VERSION) for $(uname -m)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
if command -v gh >/dev/null 2>&1; then
    if [[ "$ENGINE_VERSION" == "latest" ]]; then
        gh release download --repo "$ENGINE_REPO" --pattern "$ARCH_ASSET" --dir "$TMP"
    else
        gh release download "$ENGINE_VERSION" --repo "$ENGINE_REPO" --pattern "$ARCH_ASSET" --dir "$TMP"
    fi
else
    if [[ "$ENGINE_VERSION" == "latest" ]]; then
        URL="https://github.com/${ENGINE_REPO}/releases/latest/download/${ARCH_ASSET}"
    else
        URL="https://github.com/${ENGINE_REPO}/releases/download/${ENGINE_VERSION}/${ARCH_ASSET}"
    fi
    curl -fsSL --output "$TMP/$ARCH_ASSET" "$URL"
fi

echo "==> extracting into $INSTALL_DIR"
install -d -o quake -g quake "$INSTALL_DIR"
unzip -oq "$TMP/$ARCH_ASSET" -d "$INSTALL_DIR"

echo "==> linking trinity.ded → $ARCH_BIN"
if [[ ! -e "$INSTALL_DIR/$ARCH_BIN" ]]; then
    echo "ERROR: $ARCH_BIN not found inside the release zip; release may have changed" >&2
    ls "$INSTALL_DIR" >&2
    exit 1
fi
chmod +x "$INSTALL_DIR/$ARCH_BIN"
ln -snf "$ARCH_BIN" "$INSTALL_DIR/trinity.ded"

echo "==> redirecting baseq3/logs and missionpack/logs to $LOG_DIR"
for d in baseq3 missionpack; do
    install -d -o quake -g quake "$INSTALL_DIR/$d"
    # Only rm a real directory written by the unzip; leave existing
    # symlinks (idempotent re-runs) and never destroy operator content.
    if [[ -d "$INSTALL_DIR/$d/logs" && ! -L "$INSTALL_DIR/$d/logs" ]]; then
        rm -rf "$INSTALL_DIR/$d/logs"
    fi
    ln -snf "$LOG_DIR" "$INSTALL_DIR/$d/logs"
done

echo "==> chowning $INSTALL_DIR to quake:quake"
chown -R quake:quake "$INSTALL_DIR"

echo "==> building trinity-tracker from $SRC_DIR"
# Pin Go's caches to the throwaway $TMP. Without this, sudo -E preserves
# the operator's $HOME, so root-owned cache files end up in their
# ~/.cache/go-build and ~/go/pkg/mod and the next user-mode build fails.
( cd "$SRC_DIR" && \
    GOPATH="$TMP/go" GOCACHE="$TMP/go-cache" \
    "$GO_BIN" build -ldflags "-X main.version=collector-install" -o /usr/local/bin/trinity ./cmd/trinity )

echo "==> installing creds at /etc/trinity/source.creds"
install -m 0640 -o root -g quake "$CREDS_FILE" /etc/trinity/source.creds

echo "==> dropping /etc/trinity/config.yml.example"
install -m 0640 -o root -g quake "$HERE/config.yml.example" /etc/trinity/config.yml.example
if [[ -e /etc/trinity/config.yml ]]; then
    echo "    /etc/trinity/config.yml already exists — left alone."
else
    echo "    /etc/trinity/config.yml is NOT created; copy from the example and edit."
fi

echo "==> installing systemd units"
install -m 0644 "$HERE/systemd/trinity.service"        /etc/systemd/system/trinity.service
install -m 0644 "$HERE/systemd/quake3-server@.service" /etc/systemd/system/quake3-server@.service
install -m 0644 "$HERE/systemd/quake3-servers.target"  /etc/systemd/system/quake3-servers.target
systemctl daemon-reload

echo "==> installing /etc/logrotate.d/quake3"
# copytruncate is required: the q3 server holds the log fd open and has
# no signal handler to reopen it, so renaming would leave it writing to
# a deleted inode.
install -m 0644 "$HERE/logrotate.quake3" /etc/logrotate.d/quake3

echo
echo "Installed. Operator-owned next steps (see docs/collector-setup.md §3-7):"
echo "  - drop retail pak0.pk3 into $INSTALL_DIR/baseq3/ (+ missionpack/)"
echo "  - configure $INSTALL_DIR/baseq3/autoexec.cfg + per-server cfg/env files"
echo "  - cp /etc/trinity/config.yml.example /etc/trinity/config.yml and edit"
echo "  - sudo systemctl enable --now quake3-server@<key>.service trinity.service"
