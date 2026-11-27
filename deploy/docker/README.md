# Docker CLI

One `docker run` with the hardened defaults: uid 65532, read-only root filesystem with a
tmpfs `/tmp`, all capabilities dropped, no privilege escalation, a memory limit with a
matching `GOMEMLIMIT`, and the built-in health check.

**Prerequisites**: Docker 20.10+ (or any OCI runtime with the same flags); a media directory
writable by uid 65532 or by a group it belongs to.

```sh
sudo chown -R 65532:65532 /srv/media      # or grant a shared group write access
MEDIA_DIR=/srv/media ./run.sh
```

Open http://localhost:8090 and create the admin account. Variables: `IMAGE`, `NAME`,
`PORT`, `VOLUME`, `MEDIA_DIR`, `MEDIA_MODE=ro` (shadow mode), `CONFIG` (a config file to
mount; for one that references secrets, such as
[`../compose/corsarr.yml`](../compose/corsarr.yml), use the compose sample, which wires
them up).

**Data**: the named volume `corsarr-data` at `/data` (SQLite database and key file).

**Upgrade**: `docker rm -f corsarr`, then `IMAGE=ghcr.io/jrand0m/corsarr:vX.Y.Z ./run.sh`.
The data volume is kept; migrations run on start.

**Backup**:

```sh
docker stop corsarr
docker run --rm -v corsarr-data:/data:ro -v "$PWD":/backup busybox \
  tar -czf /backup/corsarr-data.tgz -C /data .
docker start corsarr
```
