# Changelog

Short notes per release. Each release's assets, checksums and how to verify them are on its GitHub release page.

## Unreleased

- Migration plan: copy the shown rows as text or JSON, or download them.
## v0.5.4 — 2026-11-28

- Interactive search leads with the decision: each result says what automatic search would do (the single pick, accepted but outranked and why, or rejected with the main reason in one line); the full reason list folds under "Why it was rejected?", and the separate override notice is gone.
- Interactive search shows seeders against leechers with a ratio bar and health word, whether the whole payload is available (with file count, times downloaded and the tracker's seed ratio when the indexer reports them), and how close each release is to the profile, above or below target.
- Interactive search can sort by automation order, closest to profile, seeders, size, newest or quality; the choice stays in the page link.
- Indexers: a missing seeders/peers count is no longer read as zero; Torznab `files` and `minimumratio` are carried through.
- Rate limits are learned per service (announced limits first, then where it throttles) and requests are kept below them; observed limits are re-checked gently about once a week while there is demand. See them in System › Health; settings under `server.rateLearning`.
- Media-type mode badges explain Disabled, Shadow and Active on hover.
- Config-managed fields show only a lock; hover names the key and where it is set.
- Add movie, series and artist forms line up on a two-column grid.
- Groundwork for Transmission and rTorrent download clients (protocol clients; installable modules come later).

## v0.5.3 — 2026-11-27

- Migration wizard: sources and path rewrites are checked as you type, each field has a Test button, and config-managed parts are flagged before the run.
- Config-managed values show as the value with a lock; hover names the key and where it is set (file, arguments or environment).
- Manual grabs can pick the download client.
- Assistants (MCP): add a download by URL, see why something wasn't downloaded (top rejection reasons), output schemas for every tool, three prompts (weekly digest, something to watch, why not downloaded), resource notifications through `mcp stdio`.
- Settings › General: read-only assistant key and assistant settings.

## v0.5.2 — 2026-11-27

- Fix: newer qBittorrent (5.2+) logins were read as errors.
- Fix: a second torrent could be skipped as "already added" when the client returned other torrents.

## v0.5.1 — 2026-11-26

- Native Windows service: `corsarr service install|start|stop|status|uninstall`.
- Download clients: priority levels (low, normal, high, force), ratio and upload speed in the queue, retry of a failed import, SABnzbd creates missing categories.
- Deploy samples: Proxmox LXC, NixOS flake, Ansible role, Portainer/CasaOS/Umbrel templates.
- CI actions pinned by commit; Helm chart attestation fixed.

## v0.5.0 — 2026-11-26

- Assistants (MCP): `/mcp` endpoint and `corsarr mcp stdio`; request movies, shows, music (down to a single track) and control download clients. Off by default.
- Download-client control: pause/resume all, speed limits, recheck, reannounce, category, per-download state history; shadow mode records instead of sending.
- New SABnzbd-compatible download client.
- `corsarr config defaults` and a generated configuration reference (`docs/configuration.md`).
- Runs unprivileged: refuses root without `--allow-root`, clear errors for unwritable folders, `data.umask`, optional outbound host allow list.
- Reproducible releases: rebuilt and compared on two architectures, signed image, SBOM and provenance for every file, deb/rpm/apk packages, Helm chart; `corsarr version --json` and an About screen.
- Deploy samples: docker, compose, Helm, Kubernetes, systemd, Podman, packages, Homebrew, Scoop, winget, launchd, Unraid, NAS, Nomad, Swarm, AUR.

## v0.4.0 — 2026-11-26

- Migration from Lidarr (artists, albums, track files, profiles, queue, history).
- Sonarr/Radarr/Lidarr/Prowlarr-compatible API under `/legacy/<app>` for Jellyseerr and Homepage widgets.

## v0.3.1 — 2026-11-25

- `corsarr admin reset-password` sets a forgotten admin password from stdin.

## v0.3.0 — 2026-11-25

- Music: better search queries, edition matching by payload, track matching by tags, pure-Go tag reading.
- Indexers: tolerant Torznab parsing, anti-bot pages reported as blocked, per-site encodings, form logins.
- Imports: path mapping for download clients, junk files skipped, torrents without a root folder handled.

## v0.2.0 — 2026-11-24

- Music library (artists, albums, editions) with MusicBrainz metadata and TrubadurCamp / MechanicalWavesCloud sources.
- Migration from Sonarr, Radarr and Prowlarr (`corsarr migrate` and System › Migration).
- Indexers managed in the UI, tracker definitions, Torznab feeds.

## v0.1.0 — 2026-11-23

- First public source: movies and TV, indexers, qBittorrent, imports, web UI.
