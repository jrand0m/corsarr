# Arch Linux (AUR `corsarr-bin`)

[`PKGBUILD`](PKGBUILD) packages the release binary with the hardened systemd unit and the
`corsarr` system user from [`../systemd`](../systemd); those three files are published next
to the PKGBUILD in the AUR repository. The binary hashes come from the release
`SHA256SUMS`; the local-file hashes are checked by `just deploy validate aur` (refresh them
with `updpkgsums` after changing `../systemd`).

**Prerequisites**: Arch Linux (x86_64 or aarch64), `base-devel`.

```sh
cp ../systemd/corsarr.service ../systemd/corsarr.sysusers ../systemd/corsarr.yml .
makepkg -si                       # or, once published: yay -S corsarr-bin
sudo systemctl enable --now corsarr
```

**Data**: `/var/lib/corsarr`; config `/etc/corsarr/corsarr.yml` (kept on upgrade).

**Upgrade**: bump `pkgver` and the hashes (`updpkgsums`), `makepkg -si`, or let your AUR
helper do it.

**Backup**: stop the service, archive `/var/lib/corsarr`, start it again.
