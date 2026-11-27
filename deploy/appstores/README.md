# Portainer, CasaOS, Umbrel

App templates for home-server dashboards. All run the container exactly like the
[NAS compose sample](../nas/compose.yml): non-root, read-only root filesystem, tmpfs `/tmp`,
no capabilities, the health check and a memory limit. [`icon.svg`](icon.svg) is the listing
icon.

| Platform | Template | Data | Media |
|---|---|---|---|
| Portainer 2.x | [`portainer/templates.json`](portainer/templates.json) (deploys `deploy/nas/compose.yml` from this repository) | `DATA_DIR` | `MEDIA_DIR` |
| CasaOS | [`casaos/corsarr/docker-compose.yml`](casaos/corsarr/docker-compose.yml) | `/DATA/AppData/corsarr/data` | `/DATA/Media` |
| umbrelOS 1.x | [`umbrel/`](umbrel) community app store | `${APP_DATA_DIR}/data` | Umbrel's downloads storage |

**Portainer**: **Settings → App Templates → URL**
`https://raw.githubusercontent.com/jrand0m/corsarr/master/deploy/appstores/portainer/templates.json`,
then **Templates → corsarr**, fill in `DATA_DIR`, `MEDIA_DIR`, `PUID`, `PGID` (a user that can
write both) and deploy the stack.

**CasaOS**: **App Store → Custom Install → Import** (top right), paste
`casaos/corsarr/docker-compose.yml`, check the user and paths, **Install**. Make
`/DATA/AppData/corsarr/data` writable by `PUID` first.

**Umbrel**: **App Store → ⋯ → Community App Stores**, add a repository whose root holds the
contents of `umbrel/` (`umbrel-app-store.yml` and `jrand0m-corsarr/`), then install corsarr
from it. Umbrel's app proxy puts its own login in front of the UI.

Open the app and create corsarr's admin account.

**Upgrade**: Portainer: edit the stack, **Pull and redeploy**. CasaOS: change the image tag in
the app's settings. Umbrel: updates arrive with the app store.

**Backup**: stop the app and copy its data directory.
