#!/bin/sh
# Create the corsarr system user and group before files are unpacked, so the config file
# can be group-owned by it. Idempotent; never touches an existing user.
set -e

if getent passwd corsarr >/dev/null 2>&1; then
  exit 0
fi

if command -v useradd >/dev/null 2>&1; then
  getent group corsarr >/dev/null 2>&1 || groupadd --system corsarr
  useradd --system --gid corsarr --home-dir /var/lib/corsarr --no-create-home \
    --shell /usr/sbin/nologin --comment "corsarr media automation" corsarr
elif command -v adduser >/dev/null 2>&1; then
  # BusyBox (Alpine)
  getent group corsarr >/dev/null 2>&1 || addgroup -S corsarr
  adduser -S -D -H -G corsarr -h /var/lib/corsarr -s /sbin/nologin -g "corsarr media automation" corsarr
else
  echo "corsarr: cannot create the corsarr user (no useradd or adduser)" >&2
  exit 1
fi
