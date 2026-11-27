# Docker Compose

| File | What it runs |
|---|---|
| `compose.yml` | corsarr alone |
| `compose.qbittorrent.yml` | corsarr and qBittorrent sharing one media filesystem |
| `corsarr.yml`, `corsarr.qbittorrent.yml` | the matching config files |
| `.env.example` | media path, time zone, secret values |

Both run corsarr as uid 65532 with a read-only root filesystem, tmpfs `/tmp`, no
capabilities, a memory limit with `GOMEMLIMIT`, the health check and `restart:
unless-stopped`.

**Prerequisites**: Docker with Compose v2.20+; a media directory writable by uid 65532.

```sh
cp .env.example .env                      # set MEDIA_DIR
docker compose up -d                      # or: docker compose -f compose.qbittorrent.yml up -d
```

Open http://localhost:8090 and create the admin account.

**qBittorrent variant**: start it once, read the temporary web UI password from
`docker compose -f compose.qbittorrent.yml logs qbittorrent`, set your own password and the
save path `/media/downloads` in its UI, then write the password to
`secrets/qbit_password.txt` and `docker compose -f compose.qbittorrent.yml restart corsarr`.

**Config**: [`corsarr.yml`](corsarr.yml) is the defaults
(generated from `corsarr config defaults --format=yaml`). Secrets are references:
`${env:NAME}` (value in `.env`, passed through `environment:`) or
`${file:/run/secrets/name}` (a Compose secret). See
[`../corsarr.example.yml`](../corsarr.example.yml) for every integration.

**Shadow mode**: switch the media mount to `:ro` and set the media kind's mode to `shadow`.

**Data**: the volume `corsarr_corsarr-data` at `/data`.

**Upgrade**: change the image tag, then `docker compose pull && docker compose up -d`.

**Backup**:

```sh
docker compose stop corsarr
docker run --rm -v corsarr_corsarr-data:/data:ro -v "$PWD":/backup busybox \
  tar -czf /backup/corsarr-data.tgz -C /data .
docker compose start corsarr
```
