# Ansible

The [`corsarr`](roles/corsarr) role installs corsarr in one of two modes:

- **binary** (default): downloads the release archive for the host's architecture, checks it
  against the pinned SHA-256, installs `/usr/local/bin/corsarr`, a `corsarr` system user,
  `/etc/corsarr/corsarr.yml` (validated by `corsarr config validate` before it is written)
  and the hardened unit from [`../systemd/corsarr.service`](../systemd/corsarr.service) with a
  drop-in for the binary path, media path and environment file.
- **container**: runs the pinned image under Docker as uid 65532 with a read-only root
  filesystem, no capabilities, tmpfs `/tmp`, a memory limit and the health check (needs the
  `community.docker` collection, see [`requirements.yml`](requirements.yml)).

Both are idempotent: a second run changes nothing, and a change to the config or the
version restarts corsarr.

**Prerequisites**: Ansible 2.15+; target hosts with systemd (binary) or Docker (container),
amd64 or arm64.

```sh
cd deploy/ansible
printf '[corsarr]\nmediaserver.example.com\n' > inventory.ini
ansible-galaxy collection install -r requirements.yml    # container mode only
ansible-playbook -i inventory.ini playbook.yml
```

Open `http://<host>:8090`.

| Variable | Default | |
|---|---|---|
| `corsarr_install_mode` | `binary` | or `container` |
| `corsarr_version` | `0.4.0` | with `corsarr_archive_sha256` for the binary |
| `corsarr_port` | `8090` | |
| `corsarr_media_dir` | `/srv/media` | downloads and library, one filesystem; `/media` in the container |
| `corsarr_media_readonly` | `false` | shadow mode |
| `corsarr_config` | `{}` | merged into `corsarr.yml`; `corsarr config defaults --format=yaml` lists every key |
| `corsarr_env` | `{}` | values for `${env:NAME}` references, in `/etc/corsarr/corsarr.env` (0600); keep them in Ansible Vault |
| `corsarr_user` / `corsarr_group` | `corsarr` | binary mode; a shared media group lets corsarr write the library |
| `corsarr_service_manage` | `true` | `false` installs without starting (hosts without systemd) |

**Data**: `/var/lib/corsarr` (binary) or the `corsarr-data` Docker volume (container).

**Upgrade**: raise `corsarr_version` (and the checksums, from the release's `SHA256SUMS`) and
run the playbook again.

**Backup**: stop corsarr, archive the data directory or volume, start it again.

**Test**: `ansible-lint` in this directory; `molecule test` in `roles/corsarr` runs binary mode
in a Debian container (Docker and the molecule Docker driver required).
