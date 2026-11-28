#!/bin/sh
# Builds plugin modules reproducibly: one <id>-<version>.wasm per plugin with
# its plugin.json embedded, plus that manifest as <id>-<version>.json for
# reading. POSIX sh so it runs inside the Alpine builder as well as on a
# developer machine.
#
#   scripts/plugins.sh list                     ids of the published plugins
#   scripts/plugins.sh build <out-dir> [id|all] build one plugin or all of them
#
# A plugin is plugins/<id>/ holding a main package. Directories named
# example-* are examples: built by tests, never published.
set -eu
cd "$(dirname "$0")/.."

list() {
  for f in plugins/*/main.go; do
    id=${f#plugins/}; id=${id%/main.go}
    case "$id" in example-*) ;; *) echo "$id" ;; esac
  done
}

sha() { if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1"; else shasum -a 256 "$1"; fi | cut -d' ' -f1; }

build_one() {
  out=$1 id=$2
  [ -f "plugins/$id/main.go" ] || { echo "no plugin $id (plugins/$id/main.go)" >&2; exit 2; }
  tmp=$(mktemp -d)
  # same flags as the release binary: no paths, no VCS stamp, no build id
  CGO_ENABLED=0 GOOS=wasip1 GOARCH=wasm \
    go build -buildmode=c-shared -trimpath -buildvcs=false -ldflags "-s -w -buildid=" -o "$tmp/raw.wasm" "./plugins/$id"
  go run "./plugins/$id" > "$tmp/plugin.json"
  # the file is named after the manifest's id and version, as the index expects
  name=$(sed -n 's/^  "id": "\(.*\)",$/\1/p' "$tmp/plugin.json" | head -1)
  version=$(sed -n 's/^  "version": "\(.*\)",$/\1/p' "$tmp/plugin.json" | head -1)
  [ -n "$name" ] && [ -n "$version" ] || { echo "plugins/$id: no id or version in its manifest" >&2; exit 1; }
  go run "./plugins/$id" -embed "$tmp/raw.wasm" -o "$out/$name-$version.wasm"
  cp "$tmp/plugin.json" "$out/$name-$version.json"
  rm -rf "$tmp"
  echo "$name-$version.wasm $(wc -c < "$out/$name-$version.wasm" | tr -d ' ') bytes sha256 $(sha "$out/$name-$version.wasm")"
}

cmd=${1:-}
case "$cmd" in
  list) list ;;
  build)
    out=${2:-}; target=${3:-all}
    [ -n "$out" ] || { echo "usage: plugins.sh build <out-dir> [id|all]" >&2; exit 2; }
    mkdir -p "$out"
    if [ "$target" = all ]; then
      for id in $(list); do build_one "$out" "$id"; done
    else
      build_one "$out" "$target"
    fi ;;
  *) echo "usage: plugins.sh list | build <out-dir> [id|all]" >&2; exit 2 ;;
esac
