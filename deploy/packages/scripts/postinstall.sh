#!/bin/sh
# Register the service; never start it on first install (configure first), restart it on
# upgrade when it was running.
set -e

if [ -d /run/systemd/system ] && command -v systemctl >/dev/null 2>&1; then
  systemctl daemon-reload || true
  if systemctl is-active --quiet corsarr; then
    systemctl restart corsarr || true
  else
    echo "corsarr installed. Start it with: systemctl enable --now corsarr"
  fi
elif command -v rc-service >/dev/null 2>&1; then
  if rc-service corsarr status >/dev/null 2>&1; then
    rc-service corsarr restart || true
  else
    echo "corsarr installed. Start it with: rc-update add corsarr && rc-service corsarr start"
  fi
fi
echo "Config: /etc/corsarr/corsarr.yml  Data: /var/lib/corsarr  Web UI: http://localhost:8090"
