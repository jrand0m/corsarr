#!/usr/bin/env bash
# Installs or upgrades corsarr from the GitHub release .deb inside a Debian/Ubuntu LXC
# (or any Debian/Ubuntu host): downloads SHA256SUMS and the package for this architecture,
# verifies the checksum, installs it and starts the hardened systemd service.
#
#   bash corsarr-install.sh                      # the pinned version
#   CORSARR_VERSION=0.5.0 bash corsarr-install.sh # another release (also: upgrade)
set -euo pipefail

VERSION="${CORSARR_VERSION:-0.4.0}"
REPO="${CORSARR_REPO:-jrand0m/corsarr}"
BASE="${CORSARR_BASE:-https://github.com/${REPO}/releases/download/v${VERSION#v}}"   # or a mirror

msg_info() { printf ' \033[33m-\033[0m %s\n' "$1"; }
msg_ok()   { printf ' \033[32m✓\033[0m %s\n' "$1"; }
msg_error() { printf ' \033[31m✗\033[0m %s\n' "$1" >&2; exit 1; }
fetch() { # fetch <url> <file>, with backoff for flaky networks
  for i in 1 2 3 4 5; do curl -fsSL -o "$2" "$1" && return 0; sleep $((i * 3)); done
  return 1
}

[ "$(id -u)" = 0 ] || msg_error "run as root"
command -v dpkg >/dev/null || msg_error "needs a Debian or Ubuntu system"
arch="$(dpkg --print-architecture)"
case "$arch" in amd64 | arm64) ;; *) msg_error "unsupported architecture $arch (amd64, arm64)" ;; esac

msg_info "Installing dependencies"
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq curl ca-certificates >/dev/null
msg_ok "Installed dependencies"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
msg_info "Downloading corsarr ${VERSION#v} ($arch)"
fetch "$BASE/SHA256SUMS" "$work/SHA256SUMS" || msg_error "no release v${VERSION#v} at $BASE"
# corsarr_<version>[-<release>]_<arch>.deb, as listed in SHA256SUMS
deb="$(grep -oE "corsarr_${VERSION#v}(-[0-9]+)?_${arch}\.deb\$" "$work/SHA256SUMS" | head -1)"
[ -n "$deb" ] || msg_error "release v${VERSION#v} has no .deb for $arch"
fetch "$BASE/$deb" "$work/$deb" || msg_error "download of $deb failed"
(cd "$work" && grep " \*\{0,1\}$deb\$" SHA256SUMS | sha256sum -c --quiet -) || msg_error "$deb does not match SHA256SUMS"
msg_ok "Downloaded and verified $deb"

msg_info "Installing corsarr"
apt-get install -y -qq "$work/$deb" >/dev/null
msg_ok "Installed $(corsarr version)"

# the LXC media bind mount is /media; the unit only allows writes where it is told to
if [ -d /media ] && [ ! -e /etc/systemd/system/corsarr.service.d/media.conf ]; then
  mkdir -p /etc/systemd/system/corsarr.service.d
  printf '[Service]\n# Shadow mode: ReadOnlyPaths=-/media instead\nReadWritePaths=-/media\n' \
    > /etc/systemd/system/corsarr.service.d/media.conf
  msg_ok "Allowed corsarr to write /media"
fi

if [ -d /run/systemd/system ]; then
  msg_info "Starting the corsarr service"
  systemctl enable -q corsarr
  systemctl restart corsarr
  for _ in $(seq 1 30); do corsarr healthcheck >/dev/null 2>&1 && break; sleep 1; done
  corsarr healthcheck >/dev/null 2>&1 || msg_error "corsarr is not healthy: journalctl -u corsarr"
  msg_ok "corsarr is running"
else
  msg_info "systemd is not running here: start corsarr with systemctl enable --now corsarr"
fi
ip="$(hostname -I 2>/dev/null | awk '{print $1}')"
echo "Open http://${ip:-localhost}:8090 — data: /var/lib/corsarr, config: /etc/corsarr/corsarr.yml"
