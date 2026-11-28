# Transmission module

A download-client module for [Transmission](https://transmissionbt.com) 3.x and 4.x over its
RPC interface (JSON over HTTP). It is published with every corsarr release as
`transmission-<version>.wasm`; `just build plugins transmission` builds the same file.

## Install

```sh
corsarr plugin load transmission-1.0.0.wasm
```

or drop the file into the plugins folder. Then add a download client of type `transmission`:

```yaml
downloadClients:
  seedbox:
    type: transmission
    url: http://transmission:9091
    username: corsarr
    password: ${env:TRANSMISSION_PASSWORD}
    categoryDirs:
      movies: /downloads/movies
    pathMap:
      /downloads: /mnt/media/downloads
```

## Settings

| Key | Type | Meaning |
| --- | --- | --- |
| `url` | text, required | Web interface or RPC address. Without a path ending in `/rpc`, `/transmission/rpc` is appended, so a reverse-proxy sub-path works. |
| `username`, `password` | text, secret | RPC credentials (basic auth); empty when authentication is off. |
| `categoryDirs` | map | Download directory per category, as Transmission sees it. Optional on 3.00 and later; needed on older releases, where the directory is how a category is recognised. |
| `pathMap` | map | Folder as Transmission sees it = the same folder as corsarr sees it. The longest prefix wins. |
| `addPaused` | on/off | Add every torrent stopped. |
| `timeout` | duration, 1s to 60s | How long to wait for one answer. corsarr's per-call limit for modules (`plugins.callTimeout`) still applies. |

## Permissions

- Network: only the host of `url` (`$setting:url`). Redirects to other hosts are refused.
- Secrets: `password`.

## Behaviour

- **Session**: the `X-Transmission-Session-Id` handshake is done automatically and the session
  is kept between calls of one instance.
- **Add**: torrent file contents, magnet links and http(s) addresses (Transmission fetches those
  itself). Adding a torrent Transmission already holds returns the same id. The category becomes
  a label (sent with the add on 4.x, set right after it on 3.00) and chooses the download
  directory when one is configured for it.
- **List**: one `torrent-get` asking only for the fields a list needs.
- **States**: stopped → queued, checking → downloading (completed once every wanted byte is there),
  downloading with no peers and no rate → stalled, seeding → completed. A torrent with a local
  error (a full disk, a missing drive, a permission problem) is stalled with the reason, never
  failed, so it is not blocklisted. Tracker warnings do not change the state.
- **Priority**: moves (top, up, down, bottom) are queue moves; levels low, normal and high set the
  bandwidth priority, force starts the torrent now.
- **Optional capabilities**: files, priority levels, pause all and resume all, speed limits with
  the alternative limits (turtle mode), verify, reannounce, change category (labels, 3.00+),
  client statistics, trackers. Tracker addresses are reported as scheme and host only, so
  passkeys never leave the module.
- **Remove** with or without data; removing an unknown torrent succeeds.

## Tests

`plugins/transmission/fakerpc` is a recorded, stateful fake of Transmission 3.00 and 4.0.6. The
module's own tests run against both, and corsarr's test suite runs `corsarr plugin test` on the
built module against both.
