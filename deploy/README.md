# Deploying corsarr

Pick your platform, copy the sample, set the media path and start it. Every sample runs
corsarr unprivileged (uid/gid 65532 in containers, a dedicated `corsarr` user natively),
with a read-only root filesystem and tmpfs `/tmp` where the platform allows, no
capabilities, a health check on `GET /api/v1/health` (`corsarr healthcheck` in
containers), a restart policy and a pinned version. Secrets are only ever references
(`${env:NAME}`, `${file:/path}`) to environment variables, Docker/Kubernetes secrets or
systemd credentials.

| Platform | Sample | One command |
|---|---|---|
| Docker | [`docker/`](docker) | `MEDIA_DIR=/srv/media docker/run.sh` |
| Docker Compose | [`compose/`](compose) (also with qBittorrent) | `docker compose up -d` |
| Kubernetes, Helm | [`helm/`](helm) | `helm install corsarr deploy/helm/corsarr -n corsarr --create-namespace` |
| Kubernetes, Kustomize | [`kubernetes/`](kubernetes) | `kubectl apply -k deploy/kubernetes/base` |
| Linux, systemd | [`systemd/`](systemd) | `systemctl enable --now corsarr` |
| Podman, rootless | [`podman/`](podman) | `systemctl --user start corsarr` |
| Debian, Fedora, Alpine packages | [`packages/`](packages) | `apt install ./corsarr_*.deb` |
| Homebrew (macOS, Linux) | [`homebrew/`](homebrew) | `brew install jrand0m/tap/corsarr` |
| Windows (Scoop, winget, service) | [`windows/`](windows) | `scoop install -g …/corsarr.json; corsarr service install` |
| macOS, launchd | [`launchd/`](launchd) | `launchctl bootstrap gui/$(id -u) …plist` |
| From source (`go install`) | [`go-install/`](go-install) | `go install github.com/jrand0m/corsarr/cmd/corsarr@v0.4.0` |
| Unraid | [`unraid/`](unraid) | Docker → Add Container → corsarr |
| Synology, TrueNAS SCALE, QNAP, UGREEN | [`nas/`](nas) | paste `nas/compose.yml` as a project |
| Docker Swarm | [`swarm/`](swarm) | `docker stack deploy -c stack.yml corsarr` |
| HashiCorp Nomad | [`nomad/`](nomad) | `nomad job run corsarr.nomad.hcl` |
| Arch Linux (AUR) | [`aur/`](aur) | `makepkg -si` |
| Proxmox VE (LXC) | [`proxmox/`](proxmox) | `bash corsarr-lxc.sh` on the host |
| NixOS (flake) | [`nixos/`](nixos) | `services.corsarr.enable = true;` |
| Ansible | [`ansible/`](ansible) | `ansible-playbook -i inventory.ini playbook.yml` |
| Portainer, CasaOS, Umbrel | [`appstores/`](appstores) | add the template / app store, install corsarr |

| Default | Container | Native |
|---|---|---|
| Port | 8090 | 8090 |
| Data | `/data` | `/var/lib/corsarr` |
| Config | `/etc/corsarr/corsarr.yml` | `/etc/corsarr/corsarr.yml` |
| User | 65532:65532 | `corsarr` |
| Media | `/media`, read-write (read-only for shadow mode) | your path |

Configuration: a config file is the only source when present, so samples that embed one
mark which settings are generated from `corsarr config defaults --format=yaml`;
[`corsarr.example.yml`](corsarr.example.yml) lists every option. Without a file corsarr runs
on its defaults (or `CORSARR_*` variables) and is configured in the web UI.

Downloads and the library belong on **one filesystem**, mounted at the **same path** in
corsarr and the download client, so imports are hardlinks.

Check the samples: `just deploy validate all` (lint, render, dry-run; tools that are not
installed are skipped) and `just deploy smoke <docker|compose|helm|kubernetes|podman|packages|proxmox>`.
`just release pins <vX.Y.Z>` moves every sample, checksums included, to a published release.
