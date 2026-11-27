# NAS: Synology, TrueNAS SCALE, QNAP, UGREEN

Every NAS below runs Compose projects; all use [`compose.yml`](compose.yml): one container,
non-root, read-only root filesystem, no capabilities, health check, memory limit. Unraid
has its own template in [`../unraid`](../unraid).

Before you start, on every NAS:

1. Create a folder for corsarr's data (for example `docker/corsarr`).
2. Find the user and group that own your media share and give that user write access to
   the data folder. Put the numeric ids in `user:` (the values after `:-`; paths likewise).
3. Keep downloads and the library in one share, and mount it at `/media` in your download
   client too, so imports are hardlinks.

## Synology DSM 7.2+ (Container Manager)

1. `id youruser` over SSH gives the ids (often `1026:100`).
2. **Container Manager → Project → Create**, path `/volume1/docker/corsarr`, source
   **Create compose.yml**, paste `compose.yml`, set `user:` and the two paths.
3. **Next → Done**; open `http://<nas>:8090`.

Upgrade: change the image tag in the project, **Build**. Backup: Hyper Backup task for
`/volume1/docker/corsarr` (stop the project first for a consistent copy).

## TrueNAS SCALE 24.10+

1. Datasets: `apps/corsarr` for data and your media dataset; grant the `apps` user
   (568:568) modify access, or use your own user's ids.
2. **Apps → Discover Apps → ⋮ → Install via YAML**, name `corsarr`, paste `compose.yml`
   with `/mnt/<pool>/…` paths and the user `568:568`.
3. **Save**; open `http://<nas>:8090`.

Upgrade: **Apps → corsarr → Edit**, change the tag. Backup: snapshot the `apps/corsarr`
dataset (periodic snapshot task).

## QNAP QTS / QuTS hero (Container Station 3)

1. Create the shares (for example `/share/Container/corsarr` and `/share/data/media`).
2. **Container Station → Applications → Create**, name `corsarr`, paste `compose.yml` with
   `/share/…` paths and your user's ids (`id` over SSH).
3. **Create**; open `http://<nas>:8090`.

Upgrade: edit the application, change the tag, **Update**. Backup: Hybrid Backup Sync job
for the data folder.

## UGREEN UGOS Pro (Docker app)

1. Create `docker/corsarr` in the Files app; `id` over SSH gives the ids (often `1000:10`).
2. **Docker → Project → Create**, paste `compose.yml` with `/volume1/…` paths.
3. **Deploy**; open `http://<nas>:8090`.

Upgrade: edit the project, change the tag, redeploy. Backup: back up the `docker/corsarr`
folder with the Sync & Backup app.

## Common

**Config**: optional. To use a file, add
`- /volume1/docker/corsarr/corsarr.yml:/etc/corsarr/corsarr.yml:ro` and start from
[`../compose/corsarr.yml`](../compose/corsarr.yml) (generated from
`corsarr config defaults --format=yaml`).

**Data**: the data folder mounted at `/data`.
