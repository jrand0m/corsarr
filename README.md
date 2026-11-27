# corsarr

**A compact media automation server, compiled to native code for the machine it runs on.**

corsarr watches for the movies and series you want, searches your indexers, sends the best
release to your download client, and files the result into your library: renamed,
hardlinked and announced to your media server. Indexer management, quality decisions,
downloads, imports and notifications live in **one process with one SQLite file**.

Running the usual set of separate managed-runtime applications for this costs about a
gigabyte of RAM. corsarr does the same job in a single static binary that idles at
**~20 MiB**, which leaves a Raspberry Pi, a NAS or a small VPS free for everything else.

| | corsarr |
|---|---|
| Idle memory | ~16–25 MiB RSS |
| Busy (download running, web UI open) | ~45 MiB RSS |
| Binary | one static file, ~18 MiB, web UI embedded |
| Runtime dependencies | none: no runtime, interpreter or database server |
| Storage | one SQLite file plus one key file |
| Platforms | Linux, macOS, Windows on x86_64 and arm64 |

## Features

- **Movies**: add from TMDB, automatic search on add and on release, upgrades within your
  quality profile, RSS sync, interactive search that shows *why* each release was accepted
  or rejected, manual grab with confirmation for overrides.
- **Indexers**: Torznab/Newznab built in, health tracking, back-off on failing indexers.
- **Download clients**: qBittorrent; live queue with pause, resume, priority and remove.
- **Import**: hardlink-first import (copy fallback), naming templates, upgrades move the old
  file to a recycle folder, media-server refresh (Plex).
- **Safety**: releases whose payload contains executables (`.exe`, `.scr`, …) are removed,
  blocklisted and searched again automatically, before anything reaches your library.
- **Notifications**: shoutrrr-style service URLs (ntfy, Pushover, Telegram, Discord,
  e-mail, …), with a lean native build that ships only ntfy, Pushover and webhooks.
- **Shadow mode**: per media kind, corsarr can decide everything and record what it
  *would* grab without touching your download client or files. Use it to try corsarr next
  to an existing setup.
- **Plugins**: indexers, download clients, metadata sources, media servers and notifiers
  are plugins with typed settings. The web UI renders their forms from the plugin manifest.
- **Live web UI**: pushed updates over Server-Sent Events, no polling. Under 100 KiB
  gzipped on first load.

TV series are being built now. Music and books are planned.

## Install

### Binaries

Download the archive for your platform from the [releases](../../releases) page and unpack it.

```bash
./corsarr --data.dir=./data          # then open http://localhost:8090
```

Each release includes `SHA256SUMS` and a signed build-provenance attestation. To check
that a binary was built by this repository's release workflow from the tagged commit:

```bash
sha256sum -c SHA256SUMS --ignore-missing
gh attestation verify corsarr-linux-arm64.tar.gz --repo jrand0m/corsarr
```

### Container

```bash
docker run -d --name corsarr -p 8090:8090 \
  -v corsarr-data:/data -v /srv/media:/media \
  ghcr.io/jrand0m/corsarr:latest
```

The image is `scratch` plus the binary, CA roots and a non-root user (uid 65532), about
20 MB, for `linux/amd64` and `linux/arm64`. Mount downloads and library on **one
filesystem** at the same path your download client sees, so imports are hardlinks
instead of copies. See [`deploy/`](deploy) for Compose, Kubernetes, systemd and more.

When the download client sees the same folders under other paths (another host, or a
container with different mounts), give it a `pathMap`: one `remote=local` entry per
folder, for example `/downloads=/media/downloads`. Its connection test warns when a
save folder it reports cannot be found here.

## Configuration

Configuration has exactly one source per start, chosen in this order:

1. a YAML file: `--configFile=/etc/corsarr/corsarr.yml` (watched; most sections apply live);
2. command-line arguments: `--indexers.myindexer.type=torznab --indexers.myindexer.url=…`;
3. environment variables: `CORSARR_INDEXERS_MYINDEXER_TYPE=torznab`;
4. built-in defaults.

The three forms map one to one, and `corsarr config export --format=yaml|env` converts
between them. Secrets are never written in plain text: use `${env:NAME}` or
`${file:/run/secrets/name}` references. A full example is in
[`deploy/corsarr.example.yml`](deploy/corsarr.example.yml).

```bash
corsarr config validate --plan   # check a config and print what it would change
corsarr config schema            # every key, its type and default
corsarr healthcheck              # exit status for container health checks
```

Forgot the admin password? Pipe a new one in; the running server keeps going and every
session is signed out:

```bash
printf '%s\n' "$NEW_PASSWORD" | corsarr admin reset-password      # add --username=… to rename
printf '%s\n' "$NEW_PASSWORD" | docker exec -i corsarr /corsarr admin reset-password
```

## Using corsarr with Jellyseerr or Homepage

corsarr can answer the parts of the Sonarr, Radarr, Lidarr and Prowlarr APIs that request
managers and dashboards use, so those tools work without changes. Each one is off until you
switch it on (the section applies live):

```yaml
legacy:
  sonarr:   { enabled: true }   # TV:      /legacy/sonarr   (API v3)
  radarr:   { enabled: true }   # movies:  /legacy/radarr   (API v3)
  lidarr:   { enabled: true }   # music:   /legacy/lidarr   (API v1, read-only)
  prowlarr: { enabled: true }   # indexers: /legacy/prowlarr (API v1, read-only)
```

or `--legacy.sonarr.enabled=true` / `CORSARR_LEGACY_SONARR_ENABLED=true`. Then point the tool
at the matching base URL and use corsarr's API key (Settings › General, or
`GET /api/v1/auth/apikey`):

- **Jellyseerr**: add a Sonarr server with URL base `/legacy/sonarr` and a Radarr server with
  URL base `/legacy/radarr` on corsarr's host and port (`8090` by default), with corsarr's API
  key. Behind a base path, prefix it: `/corsarr/legacy/sonarr`.
- **Homepage**: widgets of type `sonarr`, `radarr`, `lidarr` and `prowlarr` with
  `url: http://corsarr:8090/legacy/<app>` and `key:` set to corsarr's API key.

Requests go through the same TV, movie and queue services as corsarr's own UI, so adding a
series here behaves exactly like adding it in the UI, shadow mode included. Only the endpoints
these tools use are implemented; anything else under `/legacy` answers 404.

## Build from source

Requirements: Go and Node.js (versions in `.tool-versions`) and [`just`](https://github.com/casey/just).

```bash
just setup                       # Go modules and UI dependencies
just build all                   # UI bundle + native binary in bin/
just build go linux/arm64        # cross-compile: linux|darwin|windows × amd64|arm64 (also armv7)
just test full                   # Go tests with -race, UI tests, size and memory budget
just run dev                     # serve on :8090 with ./data
just run storybook               # the component library
```

Builds are `CGO_ENABLED=0`, so cross-compiling needs no C toolchain.

## Project status

Early. The movie pipeline works end to end. TV, music and books, migration from existing
installations, and a compatibility API for third-party tools are in progress. Expect
breaking changes to the configuration and database until 1.0.

## License

corsarr is free software, licensed under the GNU Affero General Public License v3.0 or
later (AGPL-3.0-or-later). See [`LICENSE`](LICENSE) for the full text.

## Not affiliated

corsarr is an independent, clean-room implementation. It is not affiliated with or
endorsed by Sonarr, Radarr, Lidarr, Readarr, Prowlarr or their authors, and contains no
code from those projects.
