# Proxmox VE (LXC)

[`corsarr-lxc.sh`](corsarr-lxc.sh) creates an unprivileged Debian 12 container with
`--onboot`, 1 core, 512 MiB and a 4 GiB disk, bind-mounts your media and runs
[`corsarr-install.sh`](corsarr-install.sh) in it. That one downloads the release `.deb`
and `SHA256SUMS`, refuses a package whose checksum does not match, installs it (binary,
`corsarr` system user, hardened systemd unit, `/etc/corsarr/corsarr.yml`) and starts the
service. Works with releases that publish Linux packages.

**Prerequisites**: Proxmox VE 8 or later, a shell on the host as root, network access
from the container to GitHub.

```sh
# on the Proxmox host
curl -fsSLO https://raw.githubusercontent.com/jrand0m/corsarr/master/deploy/proxmox/corsarr-lxc.sh
less corsarr-lxc.sh                         # read before running as root
var_media=/srv/media bash corsarr-lxc.sh    # var_media= for no media mount
```

Open `http://<container ip>:8090`. Override the defaults in the environment: `var_ctid`,
`var_hostname`, `var_cpu`, `var_ram`, `var_disk`, `var_storage`, `var_tmpl_storage`,
`var_bridge`, `var_net` (`dhcp` or `192.0.2.50/24,gw=192.0.2.1`), `CORSARR_VERSION`.

**Media**: the host path is `/media` in the container. In an unprivileged container the
`corsarr` user is host uid `100000 + uid` (`pct exec <ctid> -- id corsarr`); give that uid or
a matching group write access on the host. The installer allows the service to write
`/media` with a drop-in (`/etc/systemd/system/corsarr.service.d/media.conf`; switch it to
`ReadOnlyPaths=` for shadow mode).

**Existing container or VM**: `bash corsarr-install.sh` inside any Debian or Ubuntu system.

**Data**: `/var/lib/corsarr` in the container.

**Upgrade**: `CORSARR_VERSION=<new> bash corsarr-lxc.sh update <ctid>` on the host (or
`corsarr-install.sh` inside the container).

**Backup**: `vzdump <ctid>`, or stop the service and archive `/var/lib/corsarr`.
