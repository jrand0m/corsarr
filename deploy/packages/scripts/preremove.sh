#!/bin/sh
# Stop the service when the package is removed (not on upgrade). deb passes "remove",
# rpm passes 0 remaining installs, apk runs this only on removal.
set -e

case "${1:-remove}" in
  remove|0)
    if [ -d /run/systemd/system ] && command -v systemctl >/dev/null 2>&1; then
      systemctl disable --now corsarr >/dev/null 2>&1 || true
    elif command -v rc-service >/dev/null 2>&1; then
      rc-service corsarr stop >/dev/null 2>&1 || true
      rc-update del corsarr >/dev/null 2>&1 || true
    fi
    ;;
esac
