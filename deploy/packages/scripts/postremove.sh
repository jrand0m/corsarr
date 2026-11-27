#!/bin/sh
# Data in /var/lib/corsarr and the corsarr user are kept; remove them by hand if wanted.
set -e

if [ -d /run/systemd/system ] && command -v systemctl >/dev/null 2>&1; then
  systemctl daemon-reload || true
fi
