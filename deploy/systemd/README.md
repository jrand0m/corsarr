# systemd (native binary)

`corsarr.service` runs the static binary as the dedicated `corsarr` system user with
systemd's sandbox: read-only system (`ProtectSystem=strict`), no home directories, private
`/tmp` and devices, no capabilities, a system-call allow-list, and write access only to
`/var/lib/corsarr` and the media path. The `.deb`, `.rpm` and `.apk` packages install
these same files.

**Prerequisites**: Linux with systemd 250+; the release archive for your architecture.

```sh
sudo install -m 0755 corsarr /usr/bin/corsarr
sudo install -m 0644 corsarr.sysusers /usr/lib/sysusers.d/corsarr.conf && sudo systemd-sysusers
sudo install -d -m 0755 /etc/corsarr
sudo install -m 0640 -g corsarr corsarr.yml /etc/corsarr/corsarr.yml
sudo install -m 0644 corsarr.service /etc/systemd/system/corsarr.service
sudo systemctl daemon-reload && sudo systemctl enable --now corsarr
```

Open http://localhost:8090. Logs: `journalctl -u corsarr -f`. Health:
`corsarr healthcheck --configFile=/etc/corsarr/corsarr.yml`.

**Local changes** go in a drop-in (`sudo systemctl edit corsarr`), for example another
media path or a shared group:

```ini
[Service]
ReadWritePaths=/mnt/media
SupplementaryGroups=media
```

**Secrets**: put each in a root-only file under `/etc/corsarr/secrets/`, add
`LoadCredential=name:/etc/corsarr/secrets/name`, and reference it in `corsarr.yml` as
`${file:/run/credentials/corsarr.service/name}`.

**Config**: [`corsarr.yml`](corsarr.yml), generated from
`corsarr config defaults --format=yaml` with `data.dir: /var/lib/corsarr`.

**Data**: `/var/lib/corsarr`.

**Upgrade**: replace `/usr/bin/corsarr` and `sudo systemctl restart corsarr` (packages do
this for you).

**Backup**:

```sh
sudo systemctl stop corsarr
sudo tar -czf corsarr-data.tgz -C /var/lib/corsarr .
sudo systemctl start corsarr
```
