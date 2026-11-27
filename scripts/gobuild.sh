#!/bin/sh
# Builds one static corsarr binary with the release flags. POSIX sh so it runs
# inside the Alpine builder as well as on a developer machine.
#
#   scripts/gobuild.sh <goos> <goarch> <out>
#
# Provenance comes from the environment (all optional for dev builds):
#   VERSION            version string, e.g. v1.2.3        (default dev)
#   COMMIT             full source commit
#   TAG                release tag the commit carries
#   SOURCE_DATE_EPOCH  commit time in seconds; it is the build date
#   DIRTY              true when the tree had uncommitted changes
set -eu
[ $# -eq 3 ] || { echo "usage: gobuild.sh <goos> <goarch> <out>" >&2; exit 2; }
goos=$1 goarch=$2 out=$3
goarm=
case "$goarch" in armv7) goarch=arm goarm=7 ;; armv6) goarch=arm goarm=6 ;; esac
bi=github.com/jrand0m/corsarr/internal/buildinfo
ldflags="-s -w -buildid= -X main.version=${VERSION:-dev} -X $bi.commit=${COMMIT:-} -X $bi.tag=${TAG:-}"
ldflags="$ldflags -X $bi.epoch=${SOURCE_DATE_EPOCH:-} -X $bi.dirty=${DIRTY:-false}"
CGO_ENABLED=0 GOOS=$goos GOARCH=$goarch GOARM=$goarm \
  go build -trimpath -buildvcs=false -ldflags "$ldflags" -o "$out" ./cmd/corsarr
