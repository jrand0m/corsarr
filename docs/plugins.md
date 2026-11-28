# Plugin modules

corsarr's integrations are plugins. Most are compiled into the binary; a
**plugin module** is one more that you add at run time: a single WebAssembly
file (WASI preview 1) plus a manifest. corsarr runs it in-process with a
pure-Go runtime, so one module file works on every OS and architecture
corsarr ships for, and the binary stays static.

Modules are sandboxed. A module has no filesystem, no sockets, no
environment, no arguments and no clock of its own. Everything it does goes
through a small set of host functions: logging, HTTP through corsarr's shared
client (only to hosts it declared), its own settings, the secrets it declared,
the time, sleeping and a few whitelisted events.

Today modules can implement the **downloader** kind (download clients). A
module downloader is used exactly like a built-in one: the queue, the
download-client screens and the assistant tools see the same operations and
the same optional capabilities.

## Installing a module

- **Drop-in**: copy `mymodule.wasm` (and `mymodule.json` when the manifest is
  not embedded) into the plugins folder, `<data.dir>/plugins` unless
  `plugins.dir` says otherwise. A running server installs it within ten
  seconds. A refused file is renamed to `mymodule.wasm.rejected` with the
  reason in `mymodule.wasm.rejected.txt`.
- **CLI**: `corsarr plugin load mymodule.wasm [--sha256=<hex>]` verifies and
  installs it, printing the hosts it may contact and the secrets it may read.
  A running server picks it up within seconds.

- **Catalogue**: Settings › Plugins & integrations › Catalogue lists the
  modules your release offers. Install shows the hosts a module may contact
  and the secrets it may read and asks you to confirm; corsarr downloads,
  verifies and loads it without a restart. Update, roll back and uninstall
  work the same way. An update that asks for new permissions shows what grew.
- **Config**: `plugins.install` lists modules (`id`, optional `version`) that
  corsarr installs from the catalogue when it starts, before the rest of the
  configuration is read.

Installing verifies the manifest, the imports and exports, starts the module
once and checks its capabilities against the manifest. The module's SHA-256
is recorded and checked again every time it is loaded.

| Command | What it does |
| --- | --- |
| `corsarr plugin list` | built-in plugins and installed modules with their status |
| `corsarr plugin test <file.wasm> [--settings=s.json]` | verify a module and run its kind's conformance suite |
| `corsarr plugin load <file.wasm> [--sha256=hex]` | install and activate a module |
| `corsarr plugin unload <id>` | deactivate a module; its files stay |
| `corsarr plugin rollback <id>` | make the previous version active again |

Installed modules live in `<plugins>/<id>/<version>/` (the module, its
manifest and its recorded digest). `<plugins>/<id>/active` names the active
version; the previous version is kept for rollback, older ones are deleted.
Installing a new version hot-swaps every running instance onto it: calls in
flight finish on the old version (or are stopped after 15 seconds), new calls
go to the new one. Unloading works the same way and the instances report the
module as unloaded.

A module may not take the id of a built-in plugin. To replace a built-in with
a module, list the built-in in `plugins.disabledBuiltins` first.

Once installed, a module is configured like any plugin: in the web UI, or in
the config file under its kind's section with `type: <module id>`:

```yaml
downloadClients:
  box:
    type: exampledl
    url: http://10.0.0.10:8080
    apiKey: ${env:BOX_API_KEY}
```

## The catalogue

Each release publishes `plugins-index.json`, its signature
`plugins-index.json.sig` and one `<id>-<version>.wasm` per module (with the
manifest embedded). corsarr reads the index of its own release unless
`plugins.catalogue.url` names another one.

Nothing from an index is used until its signature verifies: Ed25519 over the
bytes `corsarr-plugin-index-v1`, a newline, then the index exactly as
published, checked against the public keys compiled into corsarr
(`internal/catalogue/trusted/<keyId>.pub`). A missing or wrong signature, or a
key the build does not trust, refuses the whole catalogue. Each download must
then have the size and SHA-256 the index lists and an embedded manifest whose
identity and permissions equal its entry; anything else is refused and nothing
is installed. The last verified index is kept in the plugins folder, so the
catalogue stays browsable offline.

`go run ./scripts/pluginindex` builds, signs and verifies an index
(`build --release vX.Y.Z --out <dir> <module.wasm>…`, `sign --key <pem>`,
`verify --pub <keyId>.pub`, `keygen <dir>`).

With `server.egressAllow` set, allow the catalogue host and the host its
downloads redirect to (for GitHub releases, `github.com` and
`release-assets.githubusercontent.com`).

### Locking it down

`plugins.uiInstall: false` turns off every install, update, rollback and
uninstall from the web UI and the API (they answer 403); the catalogue stays
browsable and the lock is shown on the page. Modules then come only from
`plugins.install` and the plugins folder. The Kubernetes, Helm and systemd
samples ship with the lock on.

`plugins.autoUpdate` (`off`, `patch` or `minor`) installs newer catalogue
versions of modules installed from the catalogue once a day; a version that
asks for new permissions always waits for you.

Every install, update, rollback and uninstall is recorded in History with the
version and SHA-256. Uninstalling keeps the module's settings schema: the
instances that use it stay in the config, report "module not installed" and
run again once it is installed.

## Limits

Every plugin instance runs on a small pool of module instances. The owner
sets ceilings in the `plugins` config section; a module may ask for less in
its manifest (`limits`), never more.

| Limit | Default | What happens when it is exceeded |
| --- | --- | --- |
| memory per module instance | 64 MiB | the call fails, the module instance is replaced |
| time per call | 10 s | the running call is stopped and fails with `timeout` |
| calls in parallel per plugin instance | 4 | further calls wait for a free module instance |
| module file size | 32 MiB | the module is refused |
| any message or HTTP body | 8 MiB | the call fails with `parse` |
| log lines per call | 200 | further lines are dropped |

A failure only affects the call that caused it. After three failures in a row
the plugin instance rests for 30 seconds (its calls fail fast with
`server_error`) and then tries again with fresh module instances. A module
instance whose memory grows past three quarters of its cap is replaced after
the call, because WebAssembly memory never shrinks.

## Network access

A module reaches the network only through `http_do`, which goes through
corsarr's shared outbound client (timeouts, retries, per-host rate limits,
`server.egressAllow`). On top of that the host enforces the module's declared
hosts:

- `api.example.org`: exactly this host;
- `*.example.org`: any subdomain (not the domain itself);
- `$setting:url`: the host of the instance's `url` setting (a `string` field
  with the `url-http` or `hostname` pattern). Download clients usually live on
  your network; this is how a module may reach exactly the host you configure.

Private, loopback and link-local addresses are refused unless the host comes
from a `$setting:` pattern. The address is checked when the name is resolved
and again right before connecting, so a name that later resolves to a private
address is still refused. Redirects are not followed; a module that follows
one makes a new request that is checked the same way. Only `http` and `https`
URLs without credentials are accepted.

## Writing a module in Go

The SDK lives in this repository under `sdk/go`:

- `sdk/go/plugin`: host functions, the HTTP helper, the settings-schema
  builder and `Main`;
- `sdk/go/plugin/downloader`: implement `downloader.Downloader` (plus any
  optional interfaces) and call `downloader.Serve` from `init`;
- `sdk/go/plugintest`: run your module's operations natively in `go test`;
- `sdk/go/abi`: the wire types, if you want to work without the helpers.

`plugins/example-downloader` is a complete module for a small JSON API
(`plugins/example-downloader/fakeapi`). Start from it.

```go
package main

import (
	"github.com/jrand0m/corsarr/sdk/go/abi"
	"github.com/jrand0m/corsarr/sdk/go/plugin"
	"github.com/jrand0m/corsarr/sdk/go/plugin/downloader"
)

var manifest = plugin.Manifest{
	ID: "mydl", Name: "My client", Version: "1.0.0", Kind: abi.KindDownloader, ABIVersion: abi.Version,
	Permissions: abi.Permissions{HTTP: abi.HTTPPermission{Hosts: []string{"$setting:url"}}, Secrets: []string{"apiKey"}},
	Settings: plugin.Schema(
		plugin.String("url", "Address").Pattern(abi.PatternURLHTTP).Required(),
		plugin.SecretField("apiKey", "API key"),
	),
}

type client struct{}

// ... implement downloader.Downloader on client ...

func init() { downloader.Serve(client{}) }

func main() { plugin.Main(manifest) }
```

Build the module as a WebAssembly reactor, then embed the manifest:

```sh
GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o mydl.wasm .
go run . -embed mydl.wasm -o mydl.wasm   # or: go run . > mydl.json (sidecar)
corsarr plugin test mydl.wasm --settings=test-instance.json
```

`-buildmode=c-shared` matters: a plain `go build` produces a command module
that exits after `main`, and corsarr refuses it with that reason. Run natively,
`plugin.Main` prints the manifest (`go run .`) or writes a copy of a module
with the manifest embedded (`-embed`).

Handlers run one call at a time per module instance and have no context
argument: the host enforces the deadline and stops a call that runs too long.
Return classified errors (`plugin.Errorf(abi.ClassAuth, …)`,
`plugin.NotFound`, `plugin.Unsupported`) so corsarr reports health correctly;
`plugin.Client.JSON` classifies HTTP statuses for you. A panic in a handler is
answered as `server_error` instead of crashing the instance.

### Size and speed

A Go module with `encoding/json` is about 5 MB. The first load compiles it to
machine code, which takes one to two seconds; the compiled code is cached in
`<plugins>/.cache`, so later loads take about 50 ms. A module instance starts
in 5 to 25 ms and a call costs well under a millisecond plus the HTTP time.
TinyGo builds the same code into a much smaller module, but its WASI exports
differ; the SDK targets the standard Go toolchain.

### Testing

`corsarr plugin test <file.wasm>` runs the same load checks as the server
(with the interpreter, so there is no compile wait), checks the start time
and, given `--settings` with a test instance's
settings, runs the conformance suite of its kind against it (for a downloader:
add, list, pause, resume, priority moves, every optional capability, remove).
Point it at a test client: it adds a paused magnet in the
`corsarr-conformance` category and removes it again.

In unit tests, `plugintest.New(plugintest.Host{Settings: …, Secrets: …})`
installs an in-process host so `Call(op, req, &resp)` runs your handlers
natively, with real HTTP to a test server.

## The ABI (version 1)

Everything crossing the boundary is a JSON message in the module's linear
memory. Each answer is an envelope: `{"result": …}` or
`{"error": {"class": "…", "message": "…"}}`. Error classes are `auth`,
`rate_limited`, `timeout`, `unreachable`, `parse`, `server_error`, `config`,
`path`, `not_found`, `unsupported` and (from the host) `denied`.

### Exports (module → host)

| Export | Signature | Purpose |
| --- | --- | --- |
| `_initialize` | `() → ()` | runtime and package initialisation, called once per module instance |
| `corsarr_abi_v1` | `() → ()` | marks the ABI version |
| `corsarr_alloc` | `(size i32) → ptr i32` | a buffer the host writes a request into |
| `corsarr_free` | `(ptr i32) → ()` | releases a buffer |
| `corsarr_call` | `(opPtr, opLen, reqPtr, reqLen i32) → i64` | runs an operation; returns the answer as `ptr << 32 \| len` |

Operations are named strings. Every module answers `capabilities`
(`{"abi": 1, "kind": "downloader", "ops": […], "flags": […]}`), `test` and,
when its settings declare actions, `action` (`{"id": "test", "values": {…}}` →
`{"outcome": "ok|warn|fail", "text": "…"}`).

Downloader operations: `downloader.protocols`, `downloader.add`,
`downloader.list`, `downloader.remove`, `downloader.pause`,
`downloader.resume`, `downloader.set_priority`, `downloader.ensure_category`
(required), and the optional `downloader.files`,
`downloader.set_priority_level`, `downloader.pause_all`,
`downloader.resume_all`, `downloader.speed_limits`,
`downloader.set_speed_limits`, `downloader.alt_speed`,
`downloader.set_alt_speed`, `downloader.recheck`, `downloader.reannounce`,
`downloader.set_category`, `downloader.stats`, `downloader.trackers`. The
flag `removeKeepsData` declares a client that cannot delete files on removal.
The message shapes are the types in `sdk/go/abi/downloader.go`.

### Imports (host → module)

Import module `corsarr_v1`; every function takes `(reqPtr, reqLen i32)` and
returns the length of its JSON answer (or -1 when the request could not be
read), which the module copies with `take(ptr, len i32) → i32`.

| Import | Request | Result |
| --- | --- | --- |
| `log` | `{level, message, attrs}` | none |
| `http_do` | `{method, url, headers, body}` (body base64) | `{status, headers, body}` |
| `settings_get` | `{}` | `{values}`: non-secret settings, defaults filled in |
| `secret_get` | `{name}` (declared in `permissions.secrets`) | `{value}` |
| `now` | `{}` | `{unixNano}` |
| `sleep` | `{millis}` | none; stopped at the call deadline |
| `emit_event` | `{type, data}`; downloaders may emit `download.progress` and `download.state_changed` | none |

From `wasi_snapshot_preview1` the host provides what Go's runtime uses:
clocks and randomness come from the host, `fd_write` to stdout and stderr goes
to the plugin log, `poll_oneoff` sleeps within the call deadline, arguments
and environment are empty, there are no preopened directories, and file
functions answer "bad descriptor". A module that imports anything else is
refused with the name of the import.

## The manifest

`plugin.json`, embedded as the `corsarr.manifest` custom section or next to
the module as a sidecar:

| Field | Rules |
| --- | --- |
| `id` | 2 to 32 lower-case letters and digits, starting with a letter |
| `name`, `author` | plain text, at most 80 characters |
| `version` | semantic version |
| `kind` | one kind per module (`downloader`) |
| `abiVersion` | `1` |
| `description` | plain text, at most 500 characters |
| `license` | an SPDX expression |
| `homepage` | an `https://` address; shown as text |
| `permissions.http.hosts` | see Network access |
| `permissions.secrets` | names of `secret` fields the module may read |
| `limits` | `memoryMiB`, `callTimeoutMs`, `concurrency`; capped by the owner's settings |
| `settings` | the settings schema below |

Unknown fields are refused, and every problem is reported with its path, for
example `settings.fields[2].label: contains a zero-width character at
position 5`.

## The settings schema

A module never sends markup, styles, scripts or layout. It sends a typed
schema; corsarr renders it with its own form components and binds every value
as text. The server validates the schema when the module loads and every value
when it is saved.

Field types (a closed set):

| Type | Options |
| --- | --- |
| `string` | `minLength`, `maxLength`, `pattern`: `hostname`, `url-http`, `path-absolute` or `identifier` |
| `secret` | `minLength`, `maxLength`; never returned once set, no default |
| `bool` | |
| `int` | `min`, `max`, `step` (whole numbers) |
| `number` | `min`, `max`, `step`, `decimals` (0 to 3) |
| `enum` | `options`: up to 100 `{value, label}` |
| `duration` | `minDuration`, `maxDuration` (`30s`, `5m`) |
| `list<string>` | the string options plus `maxItems` |
| `list<int>` | the int options plus `maxItems` |
| `map<string,string>` | `maxEntries`, `keyRule` and `valueRule` (each `minLength`, `maxLength`, `pattern`); written as `key=value` entries in config files |

Every field has a `key`, a `type`, a `label` and optionally `description`,
`hint` (shown with validation errors), `required` and `default`.

- Keys and enum values match `^[a-z][a-zA-Z0-9]{0,47}$`.
- At most 50 fields, 10 groups (`{title, description, fields: [keys]}`, one
  level, each field in at most one group) and 3 actions (`test`, `discover`
  or an identifier, each with a label).
- Text (labels, descriptions, hints, titles, option labels, the manifest's
  name and description, action results) is plain Unicode, NFC-normalised, at
  most 80 characters for labels and 500 for descriptions. Allowed: letters,
  digits, punctuation, spaces, newlines in descriptions, common symbols, and
  these emoji: ✅ ❌ ⚠ ℹ 🔒 🔓 🔑 ⏱ ⏳ 📁 📂 🎬 📺 🎵 🎶 📚 📖 ⬇ ⬆ ⚡ 🐢 💡 🧪
  ⭐ ✨ 🔔 🔕 🔍 🔧 ⚙ 🛠 📡 🌐 💾 🗑 📝 📌 ➕ ➖ ✔ ✖ ❗ ❓ 🚫 ⛔ 🟢 🟡 🔴 🔵 ⏸ ▶ ⏹
  🔄 🔁 📥 📤 🕒 📅 🎧 🖥 📱 🧲 🏷 👍 👎 🙂 (each optionally followed by the emoji
  presentation selector). Refused: control characters, text-direction
  overrides and isolates, zero-width characters, other emoji and symbols.
  `<` and `>` are fine; they are never interpreted. There is no markdown and
  there are no links.
