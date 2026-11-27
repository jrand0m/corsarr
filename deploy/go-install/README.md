# Go toolchain (`go install`)

The Go counterpart of `cargo install`: builds corsarr from source into `$(go env GOPATH)/bin`.

**Prerequisites**: Go (the version in [`go.mod`](../../go.mod)); for the web UI also Node.js
(the version in [`.tool-versions`](../../.tool-versions)).

```sh
CGO_ENABLED=0 go install -trimpath github.com/jrand0m/corsarr/cmd/corsarr@v0.4.0
corsarr --data.dir="$HOME/.local/share/corsarr"
```

The web UI bundle is built by Node.js and is not part of the Go module, so this binary
serves the API, health endpoint and integrations without the browser UI. For the full
binary, build from a checkout:

```sh
git clone https://github.com/jrand0m/corsarr && cd corsarr
git checkout v0.4.0
just setup all && just build all       # bin/corsarr-<os>-<arch>, UI embedded
```

Or take the release archive, which always has the UI.

Run it as a service with [`../systemd`](../systemd) (Linux) or [`../launchd`](../launchd)
(macOS).

**Data**: whatever `--data.dir` (or `data.dir` in the config file) names.

**Upgrade**: run the same `go install` with the new version.

**Backup**: stop corsarr and archive the data directory.
