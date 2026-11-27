# Docker Compose

| File | What it runs |
|---|---|
| `compose.yml` | corsarr, talking to a qBittorrent you already run |
| `compose.qbittorrent.yml` | corsarr and qBittorrent sharing one media filesystem |
| `corsarr.yml` | a minimal working config: TMDB metadata, one Torznab indexer, qBittorrent |
| `.env.example` | media path, time zone, API keys |
| `secrets/` | the qBittorrent password file (git-ignored) |

Both run corsarr as uid 65532 with a read-only root filesystem, tmpfs `/tmp`, no
capabilities, a memory limit with `GOMEMLIMIT`, the health check and `restart:
unless-stopped`.

**Prerequisites**: Docker with Compose v2.20+; a media directory writable by uid 65532
(`sudo chown -R 65532:65532 /srv/media`, or a shared group); a TMDB API key and a Torznab
indexer.

```sh
cp .env.example .env                       # MEDIA_DIR, TMDB_API_KEY, INDEXER_API_KEY
echo 'qbittorrent-password' > secrets/qbit_password.txt
# edit corsarr.yml: indexer URL; qBittorrent address when it is not the compose service
docker compose up -d                       # or: docker compose -f compose.qbittorrent.yml up -d
```

Open http://localhost:8090 and create the admin account.

**qBittorrent variant**: on first start read its temporary web UI password with
`docker compose -f compose.qbittorrent.yml logs qbittorrent`, set your own password (the one
in `secrets/qbit_password.txt`) and the save path `/media/downloads` in its UI.

**Config**: [`corsarr.yml`](corsarr.yml); the server, data and media-kind settings are
generated from `corsarr config defaults --format=yaml`, and
[`../corsarr.example.yml`](../corsarr.example.yml) lists every option. Secrets are
references only: `${env:NAME}` (value in `.env`, passed through `environment:`) or
`${file:/run/secrets/name}` (a Compose secret).

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
