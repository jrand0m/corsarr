# Windows (Scoop, winget, Windows service)

Scoop and winget install the release binary as the `corsarr` command. `corsarr service
install` then registers it with the Service Control Manager: it starts at boot, restarts
after a crash and runs as its own virtual account (`NT SERVICE\corsarr`), which has no
password, cannot log on and may only write its data directory.

**Prerequisites**: Windows 10/11 or Windows Server 2019+ (x64 or arm64) with Scoop or winget;
an administrator shell for the service commands.

```powershell
# machine-wide, so the service account can read the binary
scoop install -g https://raw.githubusercontent.com/jrand0m/corsarr/master/deploy/windows/scoop/corsarr.json
# or
winget install --scope machine jrand0m.corsarr     # once the manifest is in the community repository
# or, from a checkout
winget install --scope machine --manifest deploy\windows\winget

corsarr service install       # in an administrator shell
corsarr service start
```

Open http://localhost:8090.

| Command | What it does |
|---|---|
| `corsarr service install [--account=…] [config args]` | registers the service (automatic, delayed start; restart on failure), the event log source and the data directory's access rights |
| `corsarr service start` / `stop` | starts or stops it and waits until it is running or stopped |
| `corsarr service status` | prints the state; exit code 0 running, 3 not running, 4 not installed |
| `corsarr service uninstall` | stops and removes the service; data is kept |

**Configuration**: without arguments the service runs on its defaults with the data
directory `%ProgramData%\corsarr\data`. To use a config file, generate it with
`corsarr config defaults --format=yaml > C:\ProgramData\corsarr\corsarr.yml`, set
`data.dir` in it and install with
`corsarr service install --configFile=C:\ProgramData\corsarr\corsarr.yml`. Arguments given to
`install` are used on every start. Secrets go in as `${env:NAME}` references; set them for
the service only with
`Set-ItemProperty HKLM:\SYSTEM\CurrentControlSet\Services\corsarr -Name Environment -Type MultiString -Value 'TMDB_API_KEY=…'`.

**Account**: `--account=` takes another account that needs no password, e.g.
`"NT AUTHORITY\LocalService"`. The account gets modify rights on `%ProgramData%\corsarr`
and on an explicit `--data.dir`; grant it access to your media folders yourself
(`icacls D:\media /grant "NT SERVICE\corsarr:(OI)(CI)M"`).

**Logs**: the Windows event log (Application, source `corsarr`) and `corsarr.log` in the
data directory (moved to `corsarr.log.1` past 10 MiB at start).

**Data**: `%ProgramData%\corsarr\data`.

**Upgrade**: `scoop update -g corsarr` or `winget upgrade jrand0m.corsarr`, then
`Restart-Service corsarr`.

**Backup**: `corsarr service stop`, copy `%ProgramData%\corsarr`, `corsarr service start`.

**Without admin rights**: [`corsarr-task.ps1`](corsarr-task.ps1) registers a scheduled task
for your user instead, starting `corsarr` at logon with data in `%LOCALAPPDATA%\corsarr`
(`powershell -ExecutionPolicy Bypass -File corsarr-task.ps1`). A per-user `scoop install` or
`winget install` is enough for it.
