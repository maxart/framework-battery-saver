#!/bin/sh
#
# Framework Battery Saver installer.
#
#   curl -fsSL https://raw.githubusercontent.com/maxart/framework-battery-saver/main/install.sh | sh
#
# Downloads the right prebuilt binary for your platform, installs `fbs` and its
# `battery-saver.sh` helper side by side, and launches the dashboard.
#
# Env overrides:
#   FBS_INSTALL_DIR   install location (default: $HOME/.local/bin)
#   FBS_VERSION       release tag to install (default: latest)
#   FBS_NO_LAUNCH=1   install only, don't launch

set -eu

REPO="maxart/framework-battery-saver"
INSTALL_DIR="${FBS_INSTALL_DIR:-$HOME/.local/bin}"

info() { printf '\033[1;36m::\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m!!\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31mxx\033[0m %s\n' "$*" >&2; exit 1; }

# --- platform detection -----------------------------------------------------

os="$(uname -s)"
case "$os" in
    Linux) os="linux" ;;
    *) die "unsupported OS '$os' — this tool reads Linux sysfs/procfs and only runs on Linux." ;;
esac

arch="$(uname -m)"
case "$arch" in
    x86_64 | amd64) arch="amd64" ;;
    aarch64 | arm64) arch="arm64" ;;
    *) die "unsupported architecture '$arch'." ;;
esac

# --- downloader -------------------------------------------------------------

if command -v curl >/dev/null 2>&1; then
    fetch() { curl -fsSL "$1" -o "$2"; }
elif command -v wget >/dev/null 2>&1; then
    fetch() { wget -qO "$2" "$1"; }
else
    die "need curl or wget to download."
fi

asset="framework-battery-saver_${os}_${arch}.tar.gz"
if [ -n "${FBS_VERSION:-}" ]; then
    url="https://github.com/${REPO}/releases/download/${FBS_VERSION}/${asset}"
else
    url="https://github.com/${REPO}/releases/latest/download/${asset}"
fi

# --- download & install -----------------------------------------------------

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

info "Downloading ${asset}"
fetch "$url" "$tmp/$asset" || die "download failed: $url"

info "Unpacking"
tar -xzf "$tmp/$asset" -C "$tmp" || die "could not extract archive"

mkdir -p "$INSTALL_DIR"
install -m 0755 "$tmp/fbs" "$INSTALL_DIR/fbs"
install -m 0755 "$tmp/battery-saver.sh" "$INSTALL_DIR/battery-saver.sh"
info "Installed fbs + battery-saver.sh to $INSTALL_DIR"

# --- post-install checks ----------------------------------------------------

case ":$PATH:" in
    *":$INSTALL_DIR:"*) ;;
    *) warn "$INSTALL_DIR is not on your PATH — add it, e.g.:"
       warn "    echo 'export PATH=\"$INSTALL_DIR:\$PATH\"' >> ~/.bashrc" ;;
esac

missing=""
for t in cpupower powerprofilesctl; do
    command -v "$t" >/dev/null 2>&1 || missing="$missing $t"
done
if [ -n "$missing" ]; then
    warn "toggling power saver needs:$missing"
    warn "    sudo pacman -S cpupower power-profiles-daemon"
fi

# --- launch -----------------------------------------------------------------

if [ "${FBS_NO_LAUNCH:-}" = "1" ]; then
    info "Done. Run 'fbs' to launch."
    exit 0
fi

if [ ! -t 0 ] || [ ! -t 1 ]; then
    # Piped from curl: no controlling terminal for the TUI.
    info "Done. Run 'fbs' to launch the dashboard."
    exit 0
fi

info "Launching fbs…"
exec "$INSTALL_DIR/fbs"
