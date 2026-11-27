# Homebrew (macOS, Linux)

[`corsarr.rb`](corsarr.rb) installs the release binary for your platform and registers a
`brew services` service (launchd on macOS, systemd user unit on Linux) running as your
user.

**Prerequisites**: Homebrew 4+.

```sh
brew install jrand0m/tap/corsarr
brew services start corsarr
```

Open http://localhost:8090. From a checkout, without the tap:
`brew install --formula deploy/homebrew/corsarr.rb`.

**Config**: `$(brew --prefix)/etc/corsarr/corsarr.yml` (generated from
`corsarr config defaults --format=yaml`, data dir under Homebrew's `var`).

**Data**: `$(brew --prefix)/var/corsarr`; log `$(brew --prefix)/var/log/corsarr.log`.

**Upgrade**: `brew upgrade corsarr && brew services restart corsarr`.

**Backup**:

```sh
brew services stop corsarr
tar -czf corsarr-data.tgz -C "$(brew --prefix)/var/corsarr" .
brew services start corsarr
```
