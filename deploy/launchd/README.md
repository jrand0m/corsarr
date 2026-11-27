# macOS launchd (plain binary)

A LaunchAgent that starts corsarr at login as your user and restarts it if it exits with
an error. With Homebrew, prefer [`../homebrew`](../homebrew) (`brew services`).

**Prerequisites**: macOS 13+; the `corsarr-darwin-arm64` (Apple silicon) or
`corsarr-darwin-amd64` (Intel) release archive, unpacked.

```sh
mkdir -p ~/.local/bin ~/.config/corsarr "$HOME/Library/Application Support/corsarr"
install -m 0755 corsarr ~/.local/bin/corsarr
xattr -d com.apple.quarantine ~/.local/bin/corsarr 2>/dev/null || true
sed "s#__HOME__#$HOME#g" corsarr.yml > ~/.config/corsarr/corsarr.yml
sed "s#__HOME__#$HOME#g" com.github.jrand0m.corsarr.plist > ~/Library/LaunchAgents/com.github.jrand0m.corsarr.plist
launchctl bootstrap "gui/$(id -u)" ~/Library/LaunchAgents/com.github.jrand0m.corsarr.plist
```

Open http://localhost:8090. Log: `~/Library/Logs/corsarr.log`.

**Config**: [`corsarr.yml`](corsarr.yml), generated from `corsarr config defaults --format=yaml`.

**Data**: `~/Library/Application Support/corsarr`.

**Upgrade**: replace `~/.local/bin/corsarr`, then
`launchctl kickstart -k "gui/$(id -u)/com.github.jrand0m.corsarr"`.

**Backup**:

```sh
launchctl bootout "gui/$(id -u)/com.github.jrand0m.corsarr"
tar -czf corsarr-data.tgz -C "$HOME/Library/Application Support/corsarr" .
launchctl bootstrap "gui/$(id -u)" ~/Library/LaunchAgents/com.github.jrand0m.corsarr.plist
```
