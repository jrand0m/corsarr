#!/usr/bin/env bash
# Creates an unprivileged Debian 12 LXC on a Proxmox VE host and installs corsarr in it
# from the release .deb (see corsarr-install.sh). Run on the Proxmox host as root:
#
#   bash corsarr-lxc.sh                     # create a new container
#   bash corsarr-lxc.sh update <ctid>       # upgrade corsarr inside an existing one
#
# Defaults can be overridden in the environment:
#   var_ctid (next free id)  var_hostname=corsarr  var_cpu=1  var_ram=512  var_disk=4
#   var_storage=local-lvm  var_tmpl_storage=local  var_bridge=vmbr0  var_net=dhcp
#   var_media=/srv/media  (host path bind-mounted at /media; empty for none)
#   CORSARR_VERSION=0.4.0
set -euo pipefail

APP="corsarr"
VERSION="${CORSARR_VERSION:-0.4.0}"
var_hostname="${var_hostname:-corsarr}"
var_cpu="${var_cpu:-1}"
var_ram="${var_ram:-512}"
var_disk="${var_disk:-4}"
var_storage="${var_storage:-local-lvm}"
var_tmpl_storage="${var_tmpl_storage:-local}"
var_bridge="${var_bridge:-vmbr0}"
var_net="${var_net:-dhcp}"
var_media="${var_media-/srv/media}"
RAW="${CORSARR_RAW:-https://raw.githubusercontent.com/jrand0m/corsarr/master/deploy/proxmox}"

msg_info() { printf ' \033[33m-\033[0m %s\n' "$1"; }
msg_ok()   { printf ' \033[32m✓\033[0m %s\n' "$1"; }
msg_error() { printf ' \033[31m✗\033[0m %s\n' "$1" >&2; exit 1; }

header() { printf '\n  corsarr %s - Proxmox VE LXC\n\n' "$VERSION"; }

# install_script: the installer next to this file, or the published one
install_script() {
  local here; here="$(cd "$(dirname "${BASH_SOURCE[0]}")" 2>/dev/null && pwd || true)"
  if [ -n "$here" ] && [ -f "$here/corsarr-install.sh" ]; then
    cp "$here/corsarr-install.sh" "$1"
  else
    for i in 1 2 3 4 5; do curl -fsSL -o "$1" "$RAW/corsarr-install.sh" && return 0; sleep $((i * 3)); done
    msg_error "cannot download corsarr-install.sh from $RAW"
  fi
}

run_install() { # run_install <ctid>
  local s; s="$(mktemp)"
  install_script "$s"
  pct push "$1" "$s" /root/corsarr-install.sh --perms 0755
  rm -f "$s"
  pct exec "$1" -- env CORSARR_VERSION="$VERSION" bash /root/corsarr-install.sh
}

for t in pct pveam pvesh; do command -v "$t" >/dev/null || msg_error "run this on a Proxmox VE host ($t not found)"; done
[ "$(id -u)" = 0 ] || msg_error "run as root"

if [ "${1:-}" = update ]; then
  ctid="${2:?usage: corsarr-lxc.sh update <ctid>}"
  msg_info "Updating $APP in CT $ctid to $VERSION"
  run_install "$ctid"
  msg_ok "Updated $APP in CT $ctid"
  exit 0
fi

header
ctid="${var_ctid:-$(pvesh get /cluster/nextid)}"

msg_info "Fetching the Debian 12 template"
pveam update >/dev/null
tmpl="$(pveam available --section system | awk '/debian-12-standard/ {print $2}' | sort -V | tail -1)"
[ -n "$tmpl" ] || msg_error "no debian-12-standard template available"
pveam list "$var_tmpl_storage" | grep -q "$tmpl" || pveam download "$var_tmpl_storage" "$tmpl" >/dev/null
msg_ok "Template $tmpl"

msg_info "Creating CT $ctid"
net="name=eth0,bridge=$var_bridge,ip=$var_net"
[ "$var_net" = dhcp ] && net="$net,ip6=auto"
args=(--hostname "$var_hostname" --cores "$var_cpu" --memory "$var_ram" --swap 512
  --rootfs "$var_storage:$var_disk" --net0 "$net" --unprivileged 1 --features nesting=1
  --onboot 1 --tags "$APP" --description "corsarr media automation server, http://<ip>:8090")
if [ -n "$var_media" ]; then
  [ -d "$var_media" ] || msg_error "media path $var_media does not exist (set var_media, or var_media= for none)"
  args+=(--mp0 "$var_media,mp=/media")
fi
pct create "$ctid" "$var_tmpl_storage:vztmpl/$tmpl" "${args[@]}" >/dev/null
pct start "$ctid"
for _ in $(seq 1 30); do pct exec "$ctid" -- getent hosts deb.debian.org >/dev/null 2>&1 && break; sleep 2; done
msg_ok "Created CT $ctid"

run_install "$ctid"
msg_ok "Completed: corsarr runs in CT $ctid"
if [ -n "$var_media" ]; then
  echo "Media: $var_media is /media in the container. The container is unprivileged, so the"
  echo "corsarr user maps to host uid 100000 + its uid: pct exec $ctid -- id corsarr"
fi
