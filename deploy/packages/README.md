# Linux packages (.deb, .rpm, .apk)

Built with [nfpm](https://nfpm.goreleaser.com) from the release binary
([`nfpm.yaml`](nfpm.yaml)). Each package installs:

| Path | What |
|---|---|
| `/usr/bin/corsarr` | the static binary |
| `/etc/corsarr/corsarr.yml` | default config (kept when edited) from [`../systemd/corsarr.yml`](../systemd/corsarr.yml) |
| `/etc/corsarr/secrets/` | root-only directory for `LoadCredential=` files |
| `/usr/lib/systemd/system/corsarr.service` + `/usr/lib/sysusers.d/corsarr.conf` | hardened unit and system user (deb, rpm) |
| `/etc/init.d/corsarr` | OpenRC service (apk) |

The `corsarr` system user is created on install. The service is not started on first
install; upgrades restart it when it was running. Removal stops it and keeps
`/var/lib/corsarr`.

**Prerequisites**: Debian/Ubuntu, Fedora/RHEL/openSUSE or Alpine on amd64 or arm64.

```sh
sudo apt install ./corsarr_0.4.0_amd64.deb          # Debian, Ubuntu
sudo dnf install ./corsarr-0.4.0-1.x86_64.rpm       # Fedora, RHEL
sudo apk add --allow-untrusted ./corsarr_0.4.0_x86_64.apk   # Alpine
sudo systemctl enable --now corsarr                 # Alpine: rc-update add corsarr && rc-service corsarr start
```

Open http://localhost:8090. Media paths, groups and secrets: see
[`../systemd/README.md`](../systemd/README.md).

**Data**: `/var/lib/corsarr`.

**Upgrade**: install the newer package the same way.

**Backup**: stop the service, archive `/var/lib/corsarr`, start it again.

**Building locally** (from the repository root):

```sh
just build go linux/amd64
install -D bin/corsarr-linux-amd64 dist/package/corsarr
VERSION=0.4.0 ARCH=amd64 nfpm package --config deploy/packages/nfpm.yaml --packager deb --target dist/
```

Releases build the packages in the same pinned builder as the archives (`just build release`), so they are reproducible and listed in `SHA256SUMS` with provenance and SBOM attestations.
