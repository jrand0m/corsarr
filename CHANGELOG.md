# Changelog

Short notes per release. Each release's assets, checksums and how to verify them are on its GitHub release page.

## v0.5.6 — 2026-11-28

- v0.5.5 was tagged but never published (its build failed); everything below ships here.
- Fix: `mcp stdio` relay could read the session id while it changed.
- Movies now show their posters: the movie list, poster grid, detail page and add dialog use the movie's artwork (posters of movies already in the library are filled in from stored metadata on upgrade).
- Artwork is served by corsarr from its own cache (`/api/v1/images/…`): fetched once through the shared client, shrunk and re-encoded, still shown when the provider is offline; the browser no longer contacts the metadata provider. Albums and artists get Cover Art Archive covers.
- New `cache` settings: on/off, folder, size cap, stored quality and widths, and how artwork shrinks near the cap (lower quality, then smaller, then the least recently viewed is removed, library artwork last).
- System › Health shows the artwork cache's use against its cap, hit rate and last shrink, with a Clear cache action.
- Movies and series keep the provider's backdrop as fanart.
- Migration plan: copy the shown rows as text or JSON, or download them.
- Naming settings per kind (`naming.tv|movie|music`): rename on or off, colon replacement, folder and file templates; unset values keep today's names.
- Migration: the sources' naming comes across as a `naming` snippet with a per-token report (exact, approximate, or no equivalent).
- Migration: indexers served through an indexer manager share one API key variable, reusing the reference your config already has for that manager.
- Migration: a kind whose mode the config file already sets as wanted is reported with nothing to change.
- Migration: Bluray-576p and AAC-VBR files keep their quality (new ladder rungs); downloadImported and albumImportIncomplete history rows are imported.
- Migration report: real start and finish times and how long each source took to read.
- Migration plan: the config-file snippet folds away as optional, and the environment variables for API keys are listed one per line with a copy button.
- Interactive search leads with the decision: each result says what automatic search would do (the single pick, accepted but outranked and why, or rejected with the main reason in one line); the full reason list folds under "Why it was rejected?", and the separate override notice is gone.
- Interactive search shows seeders against leechers with a ratio bar and health word, whether the whole payload is available (with file count, times downloaded and the tracker's seed ratio when the indexer reports them), and how close each release is to the profile, above or below target.
- Interactive search can sort by automation order, closest to profile, seeders, size, newest or quality; the choice stays in the page link.
- Indexers: a missing seeders/peers count is no longer read as zero; Torznab `files` and `minimumratio` are carried through.
- Plugin modules: add download clients as sandboxed WebAssembly files without a new corsarr build; drop them into the plugins folder or use `corsarr plugin load`, hot-swapped on upgrade with rollback to the previous version. See `docs/plugins.md`.
- `corsarr plugin list|test|load|unload|rollback`; `plugin test` verifies a module and runs the download-client conformance suite against it.
- Go SDK for modules (`sdk/go`) with a test harness and an example download client.
- New `plugins` settings: plugins folder, built-ins to switch off, and memory, time, concurrency and size limits for modules.
- Plugin settings forms are typed data checked on the server: a closed set of field types, plain text with a small emoji set, per-field errors.
- Transmission (3.x, 4.x) and rTorrent (0.9.8, 0.10, XML-RPC over HTTP) download clients as installable plugin modules, with every optional capability; see `plugins/transmission` and `plugins/rtorrent`.
- `just build plugins [id|all]` builds plugin modules reproducibly; the SDK gains a request adapter for protocol clients.
- Releases include the plugin modules (`<id>-<version>.wasm`) and `plugins-index.json` with its Ed25519 signature; modules are rebuilt on both release runners and compared, listed in `SHA256SUMS`, and attested with provenance and an SBOM.
- Release builds include the plugin SDK sources the binary needs.

## v0.5.4 — 2026-11-28

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
