# Configuration reference

<!-- Generated from the built-in schema by `just docs config`; edit the schema, not this file. -->

corsarr reads its settings from exactly one source each time it starts, picked in this order:

1. **A YAML file**: the one named by `--configFile=…` or `CORSARR_CONFIGFILE`, otherwise the first
   `corsarr.yml` (or `corsarr.yaml`) found in `/etc/corsarr/`, `$XDG_CONFIG_HOME/corsarr/`,
   `~/.config/corsarr/` and the working folder. The file is watched: settings marked live apply
   without a restart.
2. **Command-line arguments**, such as `--server.address=:8090`.
3. **Environment variables** starting with `CORSARR_`, such as `CORSARR_SERVER_ADDRESS=:8090`.
4. **The built-in defaults** listed below.

Sources are never mixed: when a file is used, arguments and variables are ignored (corsarr warns
about them). Anything you leave out keeps its default. The key `server.address` is written
`server: { address: … }` in the file, `--server.address=…` as an argument and
`CORSARR_SERVER_ADDRESS=…` as a variable. Keys match in any letter case.

Secrets are best given as a reference: `${env:NAME}` reads an environment variable and
`${file:/run/secrets/name}` reads a file. Their values are never printed.

In keys, `<name>` is a name you choose (lower-case letters and digits, starting with a letter)
and `<n>` is a list position starting at 0. **Live** says whether a change in the watched file
applies at once (yes) or needs a restart (no).

The same reference is printed by `corsarr config defaults` (add `--format=markdown`, `yaml`
or `env`). A commented file with every default is in
[`deploy/corsarr.example.yml`](../deploy/corsarr.example.yml).

## Settings

| Key | Default | Env | Flag | Live | Explanation |
|---|---|---|---|---|---|
| `version` | `1` | `CORSARR_VERSION` | `--version=` | no | Format version of the configuration; leave it at 1. |
| `server.address` | `:8090` | `CORSARR_SERVER_ADDRESS` | `--server.address=` | no | Address and port the web UI and API listen on; change the port when 8090 is taken, or add a host to listen on one network interface only. |
| `server.basePath` | — | `CORSARR_SERVER_BASEPATH` | `--server.basePath=` | no | Path prefix when a reverse proxy serves corsarr under a sub-path, for example /corsarr; leave empty when it has its own host name. |
| `server.egressAllow.<n>` | — | `CORSARR_SERVER_EGRESSALLOW_<N>` | `--server.egressAllow.<n>=` | no | Hosts corsarr may contact (exact name, *.domain or an IP range); leave empty to allow every host, or list hosts to lock outgoing traffic down. |
| `server.rateLearning.enabled` | `true` | `CORSARR_SERVER_RATELEARNING_ENABLED` | `--server.rateLearning.enabled=` | no | Learn where each service starts throttling and keep requests below it; turn off to use only the configured limits. true or false. |
| `server.rateLearning.headroom` | `0.85` | `CORSARR_SERVER_RATELEARNING_HEADROOM` | `--server.rateLearning.headroom=` | no | Share of a learned limit corsarr uses, between 0.1 and 1; lower it to stay further from the limit. |
| `server.rateLearning.probeEvery` | `168h0m0s` | `CORSARR_SERVER_RATELEARNING_PROBEEVERY` | `--server.rateLearning.probeEvery=` | no | How often a limit found by throttling is gently re-checked while requests are waiting; 0 never re-checks. A duration such as 90s, 15m or 2h. |
| `data.dir` | `/data` | `CORSARR_DATA_DIR` | `--data.dir=` | no | Folder for the database, the secret key, backups and logs; point it at storage that survives restarts and upgrades. |
| `data.umask` | `002` | `CORSARR_DATA_UMASK` | `--data.umask=` | no | Permission mask for every file corsarr creates, in octal; 002 lets a shared media group write, 022 keeps files writable by corsarr only. |
| `log.level` | `info` | `CORSARR_LOG_LEVEL` | `--log.level=` | no | How much is logged; use debug while tracking down a problem, warn or error to keep logs short. One of: debug, info, warn, error. |
| `auth.mode` | `password` | `CORSARR_AUTH_MODE` | `--auth.mode=` | no | How people sign in: password asks for the admin login or the API key, local lets devices on private networks in without one, proxy trusts the user name your reverse proxy sends. One of: password, local, proxy. |
| `plugins.dir` | — | `CORSARR_PLUGINS_DIR` | `--plugins.dir=` | no | Folder for plugin modules; leave empty for the plugins folder inside data.dir. Drop a .wasm file here to install it. |
| `plugins.disabledBuiltins.<n>` | — | `CORSARR_PLUGINS_DISABLEDBUILTINS_<N>` | `--plugins.disabledBuiltins.<n>=` | no | Built-in plugin ids to switch off, so a plugin module with the same id can replace them; leave empty to keep every built-in. |
| `plugins.memoryMiB` | `64` | `CORSARR_PLUGINS_MEMORYMIB` | `--plugins.memoryMiB=` | no | Most memory one running module instance may use, in MiB; a module may ask for less, never more. |
| `plugins.callTimeout` | `10s` | `CORSARR_PLUGINS_CALLTIMEOUT` | `--plugins.callTimeout=` | no | Longest one call into a module may run before it is stopped; a module may ask for less, never more. A duration such as 90s, 15m or 2h. |
| `plugins.concurrency` | `4` | `CORSARR_PLUGINS_CONCURRENCY` | `--plugins.concurrency=` | no | Most calls one plugin instance runs at the same time; each runs in its own module instance. |
| `plugins.maxModuleMiB` | `32` | `CORSARR_PLUGINS_MAXMODULEMIB` | `--plugins.maxModuleMiB=` | no | Largest module file accepted, in MiB. |
| `mediaKinds.tv.mode` | `active` | `CORSARR_MEDIAKINDS_TV_MODE` | `--mediaKinds.tv.mode=` | yes | How corsarr handles TV series: disabled turns it off, shadow decides everything but only records what it would grab, active grabs and imports. One of: disabled, shadow, active. |
| `mediaKinds.movie.mode` | `active` | `CORSARR_MEDIAKINDS_MOVIE_MODE` | `--mediaKinds.movie.mode=` | yes | How corsarr handles movies: disabled turns it off, shadow decides everything but only records what it would grab, active grabs and imports. One of: disabled, shadow, active. |
| `mediaKinds.music.mode` | `active` | `CORSARR_MEDIAKINDS_MUSIC_MODE` | `--mediaKinds.music.mode=` | yes | How corsarr handles music: disabled turns it off, shadow decides everything but only records what it would grab, active grabs and imports. One of: disabled, shadow, active. |
| `mediaKinds.book.mode` | `disabled` | `CORSARR_MEDIAKINDS_BOOK_MODE` | `--mediaKinds.book.mode=` | yes | How corsarr handles books: disabled turns it off, shadow decides everything but only records what it would grab, active grabs and imports. One of: disabled, shadow, active. |
| `rootFolders.<n>.path` | — | `CORSARR_ROOTFOLDERS_<N>_PATH` | `--rootFolders.<n>.path=` | yes | Full path of a library folder as corsarr sees it, for example /media/movies. |
| `rootFolders.<n>.kind` | — | `CORSARR_ROOTFOLDERS_<N>_KIND` | `--rootFolders.<n>.kind=` | yes | Which kind of media lives in this folder. One of: tv, movie, music, book. |
| `naming.tv.rename` | `true` | `CORSARR_NAMING_TV_RENAME` | `--naming.tv.rename=` | yes | Rename imported files with the file template; false keeps each file's own name inside the item's folder. true or false. |
| `naming.tv.colon` | `smart` | `CORSARR_NAMING_TV_COLON` | `--naming.tv.colon=` | yes | What replaces a colon in names: delete drops it, dash writes -, spaceDash writes " - ", smart writes " - " after a word and drops it between digits. One of: delete, dash, spaceDash, smart. |
| `naming.tv.seriesFolder` | `{Series Title}{ (Release Year)}` | `CORSARR_NAMING_TV_SERIESFOLDER` | `--naming.tv.seriesFolder=` | yes | Folder of a series added from now on, inside its root folder. |
| `naming.tv.seasonFolder` | `Season {Season}` | `CORSARR_NAMING_TV_SEASONFOLDER` | `--naming.tv.seasonFolder=` | yes | Season folder inside the series folder (series that use season folders). |
| `naming.tv.episodeFile` | `{Series Title} - S{Season}E{Episode}{ - Episode Title}{ [Quality Full]}` | `CORSARR_NAMING_TV_EPISODEFILE` | `--naming.tv.episodeFile=` | yes | Name of an imported episode file, without the extension. |
| `naming.movie.rename` | `true` | `CORSARR_NAMING_MOVIE_RENAME` | `--naming.movie.rename=` | yes | Rename imported files with the file template; false keeps each file's own name inside the item's folder. true or false. |
| `naming.movie.colon` | `smart` | `CORSARR_NAMING_MOVIE_COLON` | `--naming.movie.colon=` | yes | What replaces a colon in names: delete drops it, dash writes -, spaceDash writes " - ", smart writes " - " after a word and drops it between digits. One of: delete, dash, spaceDash, smart. |
| `naming.movie.movieFolder` | `{Movie Title}{ (Release Year)}` | `CORSARR_NAMING_MOVIE_MOVIEFOLDER` | `--naming.movie.movieFolder=` | yes | Folder of a movie added from now on, inside its root folder. |
| `naming.movie.movieFile` | `{Movie Title}{ (Release Year)}{ - Edition}{ [Quality Full]}{ - Release Group}` | `CORSARR_NAMING_MOVIE_MOVIEFILE` | `--naming.movie.movieFile=` | yes | Name of an imported movie file, without the extension. |
| `naming.music.rename` | `true` | `CORSARR_NAMING_MUSIC_RENAME` | `--naming.music.rename=` | yes | Rename imported files with the file template; false keeps each file's own name inside the item's folder. true or false. |
| `naming.music.colon` | `smart` | `CORSARR_NAMING_MUSIC_COLON` | `--naming.music.colon=` | yes | What replaces a colon in names: delete drops it, dash writes -, spaceDash writes " - ", smart writes " - " after a word and drops it between digits. One of: delete, dash, spaceDash, smart. |
| `naming.music.artistFolder` | `{Artist Name}` | `CORSARR_NAMING_MUSIC_ARTISTFOLDER` | `--naming.music.artistFolder=` | yes | Folder of an artist added from now on, inside its root folder. |
| `naming.music.albumFolder` | `{Album Title}{ (Release Year)}` | `CORSARR_NAMING_MUSIC_ALBUMFOLDER` | `--naming.music.albumFolder=` | yes | Album folder inside the artist folder. |
| `naming.music.trackFile` | `{Artist Name} - {Album Title} - {Track} - {Track Title}` | `CORSARR_NAMING_MUSIC_TRACKFILE` | `--naming.music.trackFile=` | yes | Name of an imported track file, without the extension. |
| `automation.rules.<name>.mode` | — | `CORSARR_AUTOMATION_RULES_<NAME>_MODE` | `--automation.rules.<name>.mode=` | yes | off never runs the rule, dry-run only records what it would do, live acts; leave unset to keep the rule's own default. Try dry-run first. One of: off, dry-run, live. |
| `legacy.sonarr.enabled` | `false` | `CORSARR_LEGACY_SONARR_ENABLED` | `--legacy.sonarr.enabled=` | yes | Turn on to let request managers and dashboards use this API; they sign in with the corsarr API key. true or false. |
| `legacy.radarr.enabled` | `false` | `CORSARR_LEGACY_RADARR_ENABLED` | `--legacy.radarr.enabled=` | yes | Turn on to let request managers and dashboards use this API; they sign in with the corsarr API key. true or false. |
| `legacy.lidarr.enabled` | `false` | `CORSARR_LEGACY_LIDARR_ENABLED` | `--legacy.lidarr.enabled=` | yes | Turn on to let request managers and dashboards use this API; they sign in with the corsarr API key. true or false. |
| `legacy.prowlarr.enabled` | `false` | `CORSARR_LEGACY_PROWLARR_ENABLED` | `--legacy.prowlarr.enabled=` | yes | Turn on to let request managers and dashboards use this API; they sign in with the corsarr API key. true or false. |
| `mcp.enabled` | `false` | `CORSARR_MCP_ENABLED` | `--mcp.enabled=` | yes | Serve the MCP endpoint (authenticated with the corsarr API key or the read-only key). true or false. |
| `mcp.scope` | `read` | `CORSARR_MCP_SCOPE` | `--mcp.scope=` | yes | read: only tools that change nothing; write: also tools that add, search, monitor or remove. One of: read, write. |
| `mcp.tools.<n>` | — | `CORSARR_MCP_TOOLS_<N>` | `--mcp.tools.<n>=` | yes | A tool to expose by name, e.g. find_media; leave the list empty to expose every tool the scope allows. |
| `mcp.rateLimit` | `60` | `CORSARR_MCP_RATELIMIT` | `--mcp.rateLimit=` | yes | Most MCP calls per minute per API key; 0 means no limit. |
| `mcp.allowedOrigins.<n>` | — | `CORSARR_MCP_ALLOWEDORIGINS_<N>` | `--mcp.allowedOrigins.<n>=` | yes | A browser origin allowed to call the endpoint besides the server's own, as scheme://host[:port], e.g. http://localhost:6274. |
| `indexers.<name>.type` | — | `CORSARR_INDEXERS_<NAME>_TYPE` | `--indexers.<name>.type=` | yes | Which plugin this entry uses; its own settings are listed per plugin. One of: knaben, thepiratebay, torznab. |
| `indexers.<name>.name` | — | `CORSARR_INDEXERS_<NAME>_NAME` | `--indexers.<name>.name=` | yes | Name shown in the web UI. |
| `indexers.<name>.enabled` | `true` | `CORSARR_INDEXERS_<NAME>_ENABLED` | `--indexers.<name>.enabled=` | yes | Set to false to keep the entry but stop using it. true or false. |
| `indexers.<name>.queryLimit` | `0` | `CORSARR_INDEXERS_<NAME>_QUERYLIMIT` | `--indexers.<name>.queryLimit=` | yes | Most searches sent to this indexer per queryLimitPer, for sites that limit API use; 0 means no limit. Past the limit the indexer is skipped until the window frees up. |
| `indexers.<name>.queryLimitPer` | `day` | `CORSARR_INDEXERS_<NAME>_QUERYLIMITPER` | `--indexers.<name>.queryLimitPer=` | yes | Whether queryLimit counts per hour or per day. One of: hour, day. |
| `indexers.<name>.<setting>` | — | `CORSARR_INDEXERS_<NAME>_<SETTING>` | `--indexers.<name>.<setting>=` | yes | Settings of the chosen indexer plugin; each plugin's settings are listed below. |
| `downloadClients.<name>.type` | — | `CORSARR_DOWNLOADCLIENTS_<NAME>_TYPE` | `--downloadClients.<name>.type=` | yes | Which plugin this entry uses; its own settings are listed per plugin. One of: qbittorrent, sabnzbd. |
| `downloadClients.<name>.name` | — | `CORSARR_DOWNLOADCLIENTS_<NAME>_NAME` | `--downloadClients.<name>.name=` | yes | Name shown in the web UI. |
| `downloadClients.<name>.enabled` | `true` | `CORSARR_DOWNLOADCLIENTS_<NAME>_ENABLED` | `--downloadClients.<name>.enabled=` | yes | Set to false to keep the entry but stop using it. true or false. |
| `downloadClients.<name>.<setting>` | — | `CORSARR_DOWNLOADCLIENTS_<NAME>_<SETTING>` | `--downloadClients.<name>.<setting>=` | yes | Settings of the chosen download client plugin; each plugin's settings are listed below. |
| `metadata.<name>.type` | — | `CORSARR_METADATA_<NAME>_TYPE` | `--metadata.<name>.type=` | yes | Which plugin this entry uses; its own settings are listed per plugin. One of: musicbrainz, tmdb. |
| `metadata.<name>.name` | — | `CORSARR_METADATA_<NAME>_NAME` | `--metadata.<name>.name=` | yes | Name shown in the web UI. |
| `metadata.<name>.enabled` | `true` | `CORSARR_METADATA_<NAME>_ENABLED` | `--metadata.<name>.enabled=` | yes | Set to false to keep the entry but stop using it. true or false. |
| `metadata.<name>.<setting>` | — | `CORSARR_METADATA_<NAME>_<SETTING>` | `--metadata.<name>.<setting>=` | yes | Settings of the chosen metadata plugin; each plugin's settings are listed below. |
| `mediaServers.<name>.type` | — | `CORSARR_MEDIASERVERS_<NAME>_TYPE` | `--mediaServers.<name>.type=` | yes | Which plugin this entry uses; its own settings are listed per plugin. One of: plex. |
| `mediaServers.<name>.name` | — | `CORSARR_MEDIASERVERS_<NAME>_NAME` | `--mediaServers.<name>.name=` | yes | Name shown in the web UI. |
| `mediaServers.<name>.enabled` | `true` | `CORSARR_MEDIASERVERS_<NAME>_ENABLED` | `--mediaServers.<name>.enabled=` | yes | Set to false to keep the entry but stop using it. true or false. |
| `mediaServers.<name>.<setting>` | — | `CORSARR_MEDIASERVERS_<NAME>_<SETTING>` | `--mediaServers.<name>.<setting>=` | yes | Settings of the chosen media server plugin; each plugin's settings are listed below. |
| `notifications.<name>.type` | — | `CORSARR_NOTIFICATIONS_<NAME>_TYPE` | `--notifications.<name>.type=` | yes | Which plugin this entry uses; its own settings are listed per plugin. One of: native, shoutrrr. |
| `notifications.<name>.name` | — | `CORSARR_NOTIFICATIONS_<NAME>_NAME` | `--notifications.<name>.name=` | yes | Name shown in the web UI. |
| `notifications.<name>.enabled` | `true` | `CORSARR_NOTIFICATIONS_<NAME>_ENABLED` | `--notifications.<name>.enabled=` | yes | Set to false to keep the entry but stop using it. true or false. |
| `notifications.<name>.template.title` | — | `CORSARR_NOTIFICATIONS_<NAME>_TEMPLATE_TITLE` | `--notifications.<name>.template.title=` | yes | Message title as a template, for example [corsarr] {{.Title}}; leave empty for the built-in title. |
| `notifications.<name>.template.body` | — | `CORSARR_NOTIFICATIONS_<NAME>_TEMPLATE_BODY` | `--notifications.<name>.template.body=` | yes | Message text as a template; leave empty for the built-in text. |
| `notifications.<name>.on.<n>` | — | `CORSARR_NOTIFICATIONS_<NAME>_ON_<N>` | `--notifications.<name>.on.<n>=` | yes | Which events send a message; leave empty for grab, import, upgrade, downloadFailed, importFailed and manualAction. One of: grab, import, upgrade, rename, delete, itemAdded, itemDeleted, downloadFailed, importFailed, manualAction, health, automation, config. |
| `notifications.<name>.<setting>` | — | `CORSARR_NOTIFICATIONS_<NAME>_<SETTING>` | `--notifications.<name>.<setting>=` | yes | Settings of the chosen notification plugin; each plugin's settings are listed below. |
| `sources.<name>.type` | — | `CORSARR_SOURCES_<NAME>_TYPE` | `--sources.<name>.type=` | yes | Which plugin this entry uses; its own settings are listed per plugin. One of: mechanicalwavescloud, trubadurcamp. |
| `sources.<name>.name` | — | `CORSARR_SOURCES_<NAME>_NAME` | `--sources.<name>.name=` | yes | Name shown in the web UI. |
| `sources.<name>.enabled` | `true` | `CORSARR_SOURCES_<NAME>_ENABLED` | `--sources.<name>.enabled=` | yes | Set to false to keep the entry but stop using it. true or false. |
| `sources.<name>.<setting>` | — | `CORSARR_SOURCES_<NAME>_<SETTING>` | `--sources.<name>.<setting>=` | yes | Settings of the chosen source plugin; each plugin's settings are listed below. |

## Plugin settings

Each entry under a connection section names its plugin with `type`; the plugin adds the settings below to that entry.

### Indexer plugins (`indexers`)

#### Knaben (`type: knaben`)

| Key | Default | Env | Flag | Live | Explanation |
|---|---|---|---|---|---|
| `indexers.<name>.baseUrl` | `https://api.knaben.org/` | `CORSARR_INDEXERS_<NAME>_BASEURL` | `--indexers.<name>.baseUrl=` | yes | Address of the site. Change it only when the site moves. |
| `indexers.<name>.hideUnsafe` | `true` | `CORSARR_INDEXERS_<NAME>_HIDEUNSAFE` | `--indexers.<name>.hideUnsafe=` | yes | Leave out results the service marks as unsafe. true or false. |
| `indexers.<name>.priority` | `25` | `CORSARR_INDEXERS_<NAME>_PRIORITY` | `--indexers.<name>.priority=` | yes | Lower numbers win when the same release is found on several indexers (1 to 50). |

#### The Pirate Bay (`type: thepiratebay`)

| Key | Default | Env | Flag | Live | Explanation |
|---|---|---|---|---|---|
| `indexers.<name>.baseUrl` | `https://apibay.org/` | `CORSARR_INDEXERS_<NAME>_BASEURL` | `--indexers.<name>.baseUrl=` | yes | Address of the site. Change it only when the site moves. |
| `indexers.<name>.priority` | `25` | `CORSARR_INDEXERS_<NAME>_PRIORITY` | `--indexers.<name>.priority=` | yes | Lower numbers win when the same release is found on several indexers (1 to 50). |

#### Torznab (`type: torznab`)

| Key | Default | Env | Flag | Live | Explanation |
|---|---|---|---|---|---|
| `indexers.<name>.url` | — | `CORSARR_INDEXERS_<NAME>_URL` | `--indexers.<name>.url=` | yes | Base address of the Torznab indexer, for example http://jackett:9117/api/v2.0/indexers/all/results/torznab. |
| `indexers.<name>.apiPath` | `/api` | `CORSARR_INDEXERS_<NAME>_APIPATH` | `--indexers.<name>.apiPath=` | yes | Path appended to the address. Leave as /api unless the indexer documents another path; use an empty value when the address already ends in the API endpoint. |
| `indexers.<name>.apiKey` | (secret, unset) | `CORSARR_INDEXERS_<NAME>_APIKEY` | `--indexers.<name>.apiKey=` | yes | API key issued by the indexer. Public indexers may not need one. Give the value, ${env:NAME} or ${file:/path}. |
| `indexers.<name>.categories.<name>` | `book=7000 movie=2000 music=3000 tv=5000` | `CORSARR_INDEXERS_<NAME>_CATEGORIES_<NAME>` | `--indexers.<name>.categories.<name>=` | yes | Torznab category ids to search for each media kind (movie, tv, music, book). Defaults use the standard parents 2000, 5000, 3000 and 7000. |
| `indexers.<name>.priority` | `25` | `CORSARR_INDEXERS_<NAME>_PRIORITY` | `--indexers.<name>.priority=` | yes | Lower numbers win when the same release is found on several indexers (1 to 50). |

### Download client plugins (`downloadClients`)

#### qBittorrent (`type: qbittorrent`)

| Key | Default | Env | Flag | Live | Explanation |
|---|---|---|---|---|---|
| `downloadClients.<name>.url` | — | `CORSARR_DOWNLOADCLIENTS_<NAME>_URL` | `--downloadClients.<name>.url=` | yes | Address of the qBittorrent Web UI, for example http://qbittorrent:8080. A reverse-proxy sub-path is allowed. |
| `downloadClients.<name>.username` | — | `CORSARR_DOWNLOADCLIENTS_<NAME>_USERNAME` | `--downloadClients.<name>.username=` | yes | Web UI user name. Leave empty when authentication is bypassed for this host. |
| `downloadClients.<name>.password` | (secret, unset) | `CORSARR_DOWNLOADCLIENTS_<NAME>_PASSWORD` | `--downloadClients.<name>.password=` | yes | Web UI password. Give the value, ${env:NAME} or ${file:/path}. |
| `downloadClients.<name>.categories.<name>` | `book=books movie=movies music=music tv=tv` | `CORSARR_DOWNLOADCLIENTS_<NAME>_CATEGORIES_<NAME>` | `--downloadClients.<name>.categories.<name>=` | yes | qBittorrent category used for each media kind (movie, tv, music, book). Imports only look at these categories. |
| `downloadClients.<name>.pathMap.<n>` | — | `CORSARR_DOWNLOADCLIENTS_<NAME>_PATHMAP_<N>` | `--downloadClients.<name>.pathMap.<n>=` | yes | Needed when qBittorrent sees its download folders under other paths than corsarr (another host, or containers with different mounts). One remote=local entry per folder, for example /downloads=/mnt/media/downloads. The longest matching prefix wins. |

#### SABnzbd-compatible (`type: sabnzbd`)

| Key | Default | Env | Flag | Live | Explanation |
|---|---|---|---|---|---|
| `downloadClients.<name>.url` | — | `CORSARR_DOWNLOADCLIENTS_<NAME>_URL` | `--downloadClients.<name>.url=` | yes | Address of the SABnzbd web interface, for example http://sabnzbd:8080. A reverse-proxy sub-path is allowed. |
| `downloadClients.<name>.apiKey` | (secret, unset) | `CORSARR_DOWNLOADCLIENTS_<NAME>_APIKEY` | `--downloadClients.<name>.apiKey=` | yes | The full API key from the SABnzbd general settings (not the NZB key). Give the value, ${env:NAME} or ${file:/path}. |
| `downloadClients.<name>.categories.<name>` | `book=books movie=movies music=music tv=tv` | `CORSARR_DOWNLOADCLIENTS_<NAME>_CATEGORIES_<NAME>` | `--downloadClients.<name>.categories.<name>=` | yes | SABnzbd category used for each media kind (movie, tv, music, book). A missing category is created in SABnzbd. |
| `downloadClients.<name>.pathMap.<n>` | — | `CORSARR_DOWNLOADCLIENTS_<NAME>_PATHMAP_<N>` | `--downloadClients.<name>.pathMap.<n>=` | yes | Needed when SABnzbd sees its folders under other paths than corsarr. One remote=local entry per folder, for example /downloads=/mnt/media/downloads. The longest matching prefix wins. |

### Metadata plugins (`metadata`)

#### MusicBrainz (`type: musicbrainz`)

| Key | Default | Env | Flag | Live | Explanation |
|---|---|---|---|---|---|
| `metadata.<name>.baseUrl` | `https://musicbrainz.org/ws/2` | `CORSARR_METADATA_<NAME>_BASEURL` | `--metadata.<name>.baseUrl=` | yes | Change this only to use a MusicBrainz mirror; a mirror may allow a higher request rate. |
| `metadata.<name>.contact` | `https://github.com/jrand0m/corsarr` | `CORSARR_METADATA_<NAME>_CONTACT` | `--metadata.<name>.contact=` | yes | An address or URL sent with every request, as MusicBrainz asks of applications. |
| `metadata.<name>.requestsPerSecond` | `1` | `CORSARR_METADATA_<NAME>_REQUESTSPERSECOND` | `--metadata.<name>.requestsPerSecond=` | yes | The public service allows one request per second; higher values only apply to a mirror. |

#### TMDB (`type: tmdb`)

| Key | Default | Env | Flag | Live | Explanation |
|---|---|---|---|---|---|
| `metadata.<name>.apiKey` | (secret, unset) | `CORSARR_METADATA_<NAME>_APIKEY` | `--metadata.<name>.apiKey=` | yes | TMDB v3 API key, or the v4 read access token (a long value starting with eyJ). Both come from your TMDB account's API settings. Give the value, ${env:NAME} or ${file:/path}. |
| `metadata.<name>.language` | `en-US` | `CORSARR_METADATA_<NAME>_LANGUAGE` | `--metadata.<name>.language=` | yes | Language of titles and overviews as an IETF tag, for example en-US or de-DE. |
| `metadata.<name>.region` | — | `CORSARR_METADATA_<NAME>_REGION` | `--metadata.<name>.region=` | yes | Two-letter country code (for example US) whose cinema, digital and physical release dates are used. Empty uses the earliest date worldwide. |
| `metadata.<name>.baseUrl` | `https://api.themoviedb.org/3` | `CORSARR_METADATA_<NAME>_BASEURL` | `--metadata.<name>.baseUrl=` | yes | Only change this to reach TMDB through a mirror or proxy. |

### Media server plugins (`mediaServers`)

#### Plex (`type: plex`)

| Key | Default | Env | Flag | Live | Explanation |
|---|---|---|---|---|---|
| `mediaServers.<name>.url` | — | `CORSARR_MEDIASERVERS_<NAME>_URL` | `--mediaServers.<name>.url=` | yes | Address of your Plex server, for example http://plex:32400. It stays exactly as typed; corsarr never replaces it with an address Plex advertises. |
| `mediaServers.<name>.token` | (secret, unset) | `CORSARR_MEDIASERVERS_<NAME>_TOKEN` | `--mediaServers.<name>.token=` | yes | Plex authentication token. Paste one, or use Sign in with Plex when that flow is available. Give the value, ${env:NAME} or ${file:/path}. |
| `mediaServers.<name>.machineId` | — | `CORSARR_MEDIASERVERS_<NAME>_MACHINEID` | `--mediaServers.<name>.machineId=` | yes | Machine identifier of the paired server. When set, tests fail if another server answers at this address. |
| `mediaServers.<name>.sectionMap.<name>` | — | `CORSARR_MEDIASERVERS_<NAME>_SECTIONMAP_<NAME>` | `--mediaServers.<name>.sectionMap.<name>=` | yes | Maps path prefixes as corsarr sees them to the same folders as Plex sees them, for example /data/movies to /media/movies. Needed only when the two differ. |

### Notification plugins (`notifications`)

#### Built-in notifications (`type: native`)

| Key | Default | Env | Flag | Live | Explanation |
|---|---|---|---|---|---|
| `notifications.<name>.urls.<n>` | (secret, unset) | `CORSARR_NOTIFICATIONS_<NAME>_URLS_<N>` | `--notifications.<name>.urls.<n>=` | yes | One service URL per entry. URLs usually contain keys or tokens, so they are stored as secrets and shown masked. |

#### Notification services (shoutrrr) (`type: shoutrrr`)

| Key | Default | Env | Flag | Live | Explanation |
|---|---|---|---|---|---|
| `notifications.<name>.urls.<n>` | (secret, unset) | `CORSARR_NOTIFICATIONS_<NAME>_URLS_<N>` | `--notifications.<name>.urls.<n>=` | yes | One service URL per entry. URLs usually contain keys or tokens, so they are stored as secrets and shown masked. |

### Source plugins (`sources`)

#### MechanicalWavesCloud (`type: mechanicalwavescloud`)

| Key | Default | Env | Flag | Live | Explanation |
|---|---|---|---|---|---|
| `sources.<name>.baseUrl` | — | `CORSARR_SOURCES_<NAME>_BASEURL` | `--sources.<name>.baseUrl=` | yes | Address of the MechanicalWavesCloud source service corsarr searches and downloads through. |
| `sources.<name>.token` | (secret, unset) | `CORSARR_SOURCES_<NAME>_TOKEN` | `--sources.<name>.token=` | yes | An access token of your MechanicalWavesCloud account; it unlocks liked and private uploads. Give the value, ${env:NAME} or ${file:/path}. |
| `sources.<name>.format` | `original` | `CORSARR_SOURCES_<NAME>_FORMAT` | `--sources.<name>.format=` | yes | original keeps the uploaded file where the uploader allows it; mp3 takes the streaming copy. One of: original, mp3. |
| `sources.<name>.downloadDir` | — | `CORSARR_SOURCES_<NAME>_DOWNLOADDIR` | `--sources.<name>.downloadDir=` | yes | Where fetched audio is written before import; on the same file system as the music library it is hardlinked. |

#### TrubadurCamp (`type: trubadurcamp`)

| Key | Default | Env | Flag | Live | Explanation |
|---|---|---|---|---|---|
| `sources.<name>.baseUrl` | — | `CORSARR_SOURCES_<NAME>_BASEURL` | `--sources.<name>.baseUrl=` | yes | Address of the TrubadurCamp source service corsarr searches and downloads through. |
| `sources.<name>.identity` | (secret, unset) | `CORSARR_SOURCES_<NAME>_IDENTITY` | `--sources.<name>.identity=` | yes | The session cookie of your TrubadurCamp account; it unlocks the albums in your collection. Give the value, ${env:NAME} or ${file:/path}. |
| `sources.<name>.format` | `flac` | `CORSARR_SOURCES_<NAME>_FORMAT` | `--sources.<name>.format=` | yes | The format albums are downloaded in; FLAC keeps the original quality. One of: flac, mp3-320, mp3-v0, alac, aac-hi, vorbis, wav. |
| `sources.<name>.downloadDir` | — | `CORSARR_SOURCES_<NAME>_DOWNLOADDIR` | `--sources.<name>.downloadDir=` | yes | Where fetched albums are written before import; on the same file system as the music library they are hardlinked. |
