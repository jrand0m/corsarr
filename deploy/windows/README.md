# Windows (Scoop, winget)

Both install the release binary as the `corsarr` command; no installer, no admin rights,
no service account.

**Prerequisites**: Windows 10/11 (x64 or arm64) with Scoop or winget.

```powershell
scoop install https://raw.githubusercontent.com/jrand0m/corsarr/master/deploy/windows/scoop/corsarr.json
# or
winget install jrand0m.corsarr          # once the manifest is in the community repository
# or, from a checkout
winget install --manifest deploy\windows\winget

corsarr --data.dir="$env:LOCALAPPDATA\corsarr"
```

Open http://localhost:8090.

**Run at logon**: [`corsarr-task.ps1`](corsarr-task.ps1) registers a scheduled task for your
user that starts corsarr at logon and restarts it on failure. A config file is optional:
corsarr reads `%USERPROFILE%\.config\corsarr\corsarr.yml` when present (generated from
`corsarr config defaults --format=yaml`; set `data.dir` there instead of the argument).

**Data**: `%LOCALAPPDATA%\corsarr`.

**Upgrade**: `scoop update corsarr` or `winget upgrade jrand0m.corsarr`, then restart the
task (`Stop-ScheduledTask corsarr; Start-ScheduledTask corsarr`).

**Backup**: stop the task, copy `%LOCALAPPDATA%\corsarr`, start it again.
