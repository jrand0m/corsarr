#!/usr/bin/env bash
# Fails when the binary depends on a package from a folder the Dockerfile's
# source stage does not copy, so the release build cannot miss it.
set -euo pipefail
mod="$(go list -m)"
copied="$(sed -n 's#^COPY \([a-z][a-z0-9_-]*\)/ .*#\1#p' Dockerfile | sort -u)"
missing=0
while read -r pkg; do
  top="${pkg#"$mod"/}"; top="${top%%/*}"
  if ! grep -qx "$top" <<<"$copied"; then
    echo "Dockerfile does not copy $top/ but the binary imports $pkg"; missing=1
  fi
done < <(go list -deps ./cmd/corsarr | grep "^$mod/" | sort -u)
exit "$missing"
