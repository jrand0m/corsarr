#!/usr/bin/env bash
# Runs the image the way restricted platforms do and checks that corsarr
# starts, answers its health probe, completes first-run setup and sees its
# library folder as writable. Nothing in the image is chowned or chmodded:
# volumes are prepared from outside, as a deployer would.
#
# Usage: scripts/container-test.sh <image> [engine] [case…]
#   engine  docker (default) or podman
#   cases   default | anyuid | readonly | refuse   (default: all)
set -euo pipefail

image="${1:?usage: scripts/container-test.sh <image> [docker|podman] [case…]}"
engine="${2:-docker}"
shift $(( $# >= 2 ? 2 : 1 ))
cases=("$@")
[ ${#cases[@]} -gt 0 ] || cases=(default anyuid readonly refuse)

helper="docker.io/library/busybox:1.36.1"   # only used to prepare volumes, never part of the image
tag="corsarr-ct-$$"
fail=0

r() { for i in 1 2 3 4 5; do "$@" && return 0; echo "retry $i…" >&2; sleep $((i * 5)); done; return 1; }

cleanup() {
  "$engine" ps -aq --filter "name=$tag" | xargs -r "$engine" rm -f >/dev/null 2>&1 || true
  "$engine" volume ls -q --filter "name=$tag" | xargs -r "$engine" volume rm -f >/dev/null 2>&1 || true
}
trap cleanup EXIT

# volume <name> <owner> <mode> — a named volume prepared from outside the image
volume() {
  local v="$tag-$1"
  "$engine" volume create "$v" >/dev/null
  "$engine" run --rm --user 0:0 -v "$v:/v" "$helper" sh -c "chown $2 /v && chmod $3 /v"
  echo "$v"
}

# start <name> <run args…> — runs the image detached on a random loopback port
start() {
  local name="$tag-$1"; shift
  "$engine" run -d --name "$name" -p 127.0.0.1::8090 \
    -e CORSARR_ROOTFOLDERS_0_PATH=/media -e CORSARR_ROOTFOLDERS_0_KIND=movie "$@" "$image" >/dev/null
  echo "$name"
}

port() { "$engine" port "$1" 8090/tcp | head -1 | sed 's/.*://'; }

wait_healthy() {
  local name="$1" url="$2"
  for _ in $(seq 1 60); do
    curl -fsS "$url/api/v1/health" >/dev/null 2>&1 && return 0
    [ "$("$engine" inspect -f '{{.State.Running}}' "$name")" = true ] || break
    sleep 0.5
  done
  echo "not healthy; logs:" >&2
  "$engine" logs "$name" >&2 || true
  return 1
}

# expect <text> <command…> — the command succeeds and its output contains text
expect() {
  local want="$1" out
  shift
  out="$("$@")"
  grep -qF "$want" <<< "$out" || { echo "expected $want, got: $out" >&2; return 1; }
}

# check <name> — health, the in-image probe, first-run setup, library folder
check() {
  local name="$1" url jar
  url="http://127.0.0.1:$(port "$name")"
  wait_healthy "$name" "$url"
  "$engine" exec "$name" /corsarr healthcheck
  expect '"required":true' curl -fsS "$url/api/v1/firstrun"
  jar="$(mktemp)"
  expect '"authenticated":true' curl -fsS -c "$jar" -H 'Content-Type: application/json' \
    -d '{"username":"admin","password":"container-check-1"}' "$url/api/v1/auth/setup"
  expect '"required":false' curl -fsS "$url/api/v1/firstrun"
  local roots=""
  for _ in $(seq 1 20); do
    roots="$(curl -fsS -b "$jar" "$url/api/v1/rootfolders")"
    grep -q '"writable":true' <<< "$roots" && break
    sleep 0.5
  done
  rm -f "$jar"
  grep -q '"path":"/media"' <<< "$roots" && grep -q '"writable":true' <<< "$roots" ||
    { echo "library folder not writable: $roots" >&2; return 1; }
}

# expect_refusal <name> <message> <run args…> — the container must exit non-zero with message
expect_refusal() {
  local name="$tag-$1" want="$2" code; shift 2
  code=0
  "$engine" run --name "$name" "$@" >/dev/null 2>&1 || code=$?
  local logs
  logs="$("$engine" logs "$name" 2>&1)"
  [ "$code" -ne 0 ] && grep -qF "$want" <<< "$logs" ||
    { echo "expected refusal ($want), got exit $code: $logs" >&2; return 1; }
}

case_default() {
  local data media
  data="$(volume data-a 65532:65532 0750)"; media="$(volume media-a 65532:65532 0775)"
  check "$(start default --user 65532:65532 -v "$data:/data:nocopy" -v "$media:/media:nocopy")"
}

case_anyuid() {
  local uid data media
  uid=$(( 10000 + RANDOM % 50000 ))  # inside the default rootless subuid range
  data="$(volume data-b 0:0 0770)"; media="$(volume media-b 0:0 0770)"
  check "$(start anyuid --user "$uid:0" -v "$data:/data:nocopy" -v "$media:/media:nocopy")"
}

case_readonly() {
  local data media
  data="$(volume data-c 65532:65532 0750)"; media="$(volume media-c 65532:65532 0775)"
  check "$(start readonly --user 65532:65532 --read-only --tmpfs /tmp:rw,mode=1777 \
    --cap-drop ALL --security-opt no-new-privileges -v "$data:/data:nocopy" -v "$media:/media:nocopy")"
}

case_refuse() {
  local data
  data="$(volume data-d 65532:65532 0750)"
  expect_refusal unwritable "data dir /data is not writable by uid 12345 gid 12345: chown it or mount a writable volume" \
    --user 12345:12345 -v "$data:/data:nocopy" "$image"
  expect_refusal root "refusing to run as root inside a container" --user 0:0 "$image"
}

r "$engine" pull -q "$helper" >/dev/null
for c in "${cases[@]}"; do
  printf -- '-- %s (%s)\n' "$c" "$engine"
  set +e
  ( set -e; "case_$c" )
  rc=$?
  set -e
  if [ "$rc" -eq 0 ]; then echo "ok   $c"; else echo "FAIL $c"; fail=1; fi
done
exit "$fail"
