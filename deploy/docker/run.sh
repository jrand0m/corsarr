#!/bin/sh
# Start corsarr with one `docker run`: non-root, read-only root filesystem, no
# capabilities, health-checked, restarted by Docker.
#
#   MEDIA_DIR=/srv/media ./run.sh            then open http://localhost:8090
#
# Optional: CONFIG=/path/to/corsarr.yml mounts a config file (see ../compose/corsarr.yml);
# without one corsarr runs on its defaults and is configured from the web UI.
set -eu

IMAGE="${IMAGE:-ghcr.io/jrand0m/corsarr:v0.4.0}"
NAME="${NAME:-corsarr}"
VOLUME="${VOLUME:-corsarr-data}"   # named volume for /data
PORT="${PORT:-8090}"
MEDIA_DIR="${MEDIA_DIR:-/srv/media}"
MEDIA_MODE="${MEDIA_MODE:-rw}"   # ro for shadow mode: corsarr never writes media

set -- --volume "$MEDIA_DIR:/media:$MEDIA_MODE"
if [ -n "${CONFIG:-}" ]; then
  set -- "$@" --volume "$CONFIG:/etc/corsarr/corsarr.yml:ro"
fi

docker volume create "$VOLUME" >/dev/null
docker run --detach --name "$NAME" \
  --restart unless-stopped \
  --user 65532:65532 \
  --read-only --tmpfs /tmp:rw,size=64m,mode=1777 \
  --cap-drop ALL --security-opt no-new-privileges \
  --memory 256m --pids-limit 256 --env GOMEMLIMIT=96MiB \
  --health-cmd '/corsarr healthcheck' --health-interval 30s \
  --health-timeout 5s --health-start-period 15s --health-retries 3 \
  --publish "$PORT:8090" \
  --volume "$VOLUME:/data" \
  "$@" \
  "$IMAGE"
