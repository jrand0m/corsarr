# Unraid

[`corsarr.xml`](corsarr.xml) is a Docker template for Unraid's Docker page (and the
Community Applications format). It runs corsarr as `nobody:users` (99:100), the owner of
Unraid shares, with a read-only root filesystem, tmpfs `/tmp`, no capabilities and a
memory limit.

**Prerequisites**: Unraid 6.12+ with Docker enabled; a data share holding downloads and
library (for example `/mnt/user/data/media`).

```sh
mkdir -p /mnt/user/appdata/corsarr && chown 99:100 /mnt/user/appdata/corsarr
wget -O /boot/config/plugins/dockerMan/templates-user/my-corsarr.xml \
  https://raw.githubusercontent.com/jrand0m/corsarr/master/deploy/unraid/corsarr.xml
```

Then **Docker → Add Container → Template: corsarr**, check the media path, **Apply**, and
open the WebUI. Mount the media share at `/media` in your download client too, so imports
are hardlinks.

**Data**: `/mnt/user/appdata/corsarr`.

**Upgrade**: edit the container and change the tag in **Repository**, then **Apply**.

**Backup**: stop the container and copy `/mnt/user/appdata/corsarr` (or include it in the
appdata backup plugin).
