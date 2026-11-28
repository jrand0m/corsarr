# rTorrent module

A download-client module for [rTorrent](https://github.com/rakshasa/rtorrent) over XML-RPC on
HTTP: the `/RPC2` endpoint that ruTorrent, Flood or a reverse proxy in front of rTorrent's SCGI
socket provides. It is published with every corsarr release as `rtorrent-<version>.wasm`;
`just build plugins rtorrent` builds the same file.

Modules have no raw sockets, so an SCGI port or socket cannot be used directly. Expose it over
HTTP first, for example with nginx:

```nginx
location /RPC2 {
    scgi_pass 127.0.0.1:5000;
    include scgi_params;
    auth_basic "rtorrent";
    auth_basic_user_file /etc/nginx/rtorrent.htpasswd;
}
```

## Install

```sh
corsarr plugin load rtorrent-1.0.0.wasm
```

```yaml
downloadClients:
  rt:
    type: rtorrent
    url: https://seedbox.example/RPC2
    username: corsarr
    password: ${env:RTORRENT_PASSWORD}
    pathMap:
      /home/rt/downloads: /mnt/media/downloads
```

## Settings

| Key | Type | Meaning |
| --- | --- | --- |
| `url` | text, required | The XML-RPC endpoint; an address without a path gets `/RPC2`. |
| `username`, `password` | text, secret | Basic auth of the endpoint, if any. |
| `pathMap` | map | Folder as rTorrent sees it = the same folder as corsarr sees it. |
| `labelField` | custom1 … custom4 | The custom field holding the label (category); web front ends use `custom1`. `custom5` stays free for the front end's data-erasing flag. |
| `timeout` | duration, 1s to 60s | How long to wait for one answer (corsarr's per-call module limit still applies). |

## Permissions

- Network: only the host of `url` (`$setting:url`).
- Secrets: `password`.

## Behaviour

- **Codec**: a strict XML-RPC encoder and decoder with size and depth caps and no entity
  expansion; calls are batched with `system.multicall` where the server has it.
- **Add**: magnet links (`load.start`, or `load.normal` when paused) and torrent file contents
  (`load.raw_start` / `load.raw`); the directory and the label are set as the download is
  created. Adding a download rTorrent already holds returns the same id. A bare http(s)
  `.torrent` address is refused: the module can only reach the rTorrent endpoint.
- **List**: one `d.multicall2` with the fields a list needs.
- **States**: stopped or paused → queued, active → downloading, active without transfer →
  stalled, complete → completed, hash checking → downloading (completed once complete). A
  download rTorrent stopped by itself with a message before it finished is stalled with that
  message as the reason, never failed, so it is not blocklisted.
- **Priority**: rTorrent has no queue order; moves map to priorities (top = high, bottom = low,
  up and down one step), levels low, normal and high set `d.priority`, force is high.
- **Remove**: `d.erase`. rTorrent never deletes data itself, so the module declares that removes
  keep the data and corsarr removes without it. A remove that still asks for the data uses the
  front end's `erasedata` handler when the server has one, and is refused before anything is
  removed when it does not. The settings form's test button says which applies.
- **Optional capabilities**: files, priority levels, pause all and resume all (pausing keeps
  downloads started, so resume all brings back exactly those and leaves stopped ones alone),
  global speed limits (rTorrent has no alternative limits: they read as off and turning them on
  is unsupported), hash check, reannounce where the build has it, change label, client
  statistics, trackers (scheme and host only).

## Tests

`plugins/rtorrent/fakexmlrpc` is a recorded, stateful fake of a plain rTorrent 0.9.8 and of
rTorrent 0.10 behind a web front end. The module's own tests run against both, and corsarr's
test suite runs `corsarr plugin test` on the built module against both.
