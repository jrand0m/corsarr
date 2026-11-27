#!/usr/bin/env bash
# Reproducible release builds: build, compare and verify against what was published.
#
#   scripts/release.sh build <out-dir> [ref]   archives + image (OCI layout) for ref (default: the working tree)
#   scripts/release.sh compare <left> <right>  identical / first difference per artifact
#   scripts/release.sh twice [ref]             build twice without cache and compare
#   scripts/release.sh verify <tag>            rebuild a published release and compare with its downloads
#
# Everything is built by the Dockerfile inside a BuildKit pinned by digest, so
# the only host requirements are bash, git, docker with buildx, curl and Go (for
# the comparison report). Environment:
#   PLATFORMS  image platforms (default linux/amd64,linux/arm64)
#   REPO       owner/name on GitHub (default jrand0m/corsarr)
#   IMAGE      published image (default ghcr.io/$REPO)
#   GOPROXY    Go module mirror for the build (go.sum still pins every module)
#   HTTP_PROXY, HTTPS_PROXY, NO_PROXY   passed to the build's downloads
set -euo pipefail

BUILDKIT_IMAGE="moby/buildkit:v0.34.2@sha256:c5f8862f52499773af85bc07695b2379ef08e8002352b0df2c4864c91d78dabe"
BUILDER="corsarr-repro"
PLATFORMS="${PLATFORMS:-linux/amd64,linux/arm64}"
REPO="${REPO:-jrand0m/corsarr}"
IMAGE="${IMAGE:-ghcr.io/$REPO}"
ARCHIVES=(corsarr-linux-amd64.tar.gz corsarr-linux-arm64.tar.gz corsarr-darwin-amd64.tar.gz
  corsarr-darwin-arm64.tar.gz corsarr-windows-amd64.zip corsarr-windows-arm64.zip)

top="$(cd "$(dirname "$0")/.." && pwd)"
r() { for i in 1 2 3 4 5; do "$@" && return 0; echo "retry $i: $*" >&2; sleep $((i * 5)); done; return 1; }
gorun() { if command -v mise >/dev/null 2>&1; then (cd "$top" && mise exec -- go "$@"); else (cd "$top" && go "$@"); fi; }

# builder makes sure the pinned BuildKit exists; the BuildKit version shapes image layers.
builder() {
  if ! docker buildx inspect "$BUILDER" >/dev/null 2>&1; then
    r docker buildx create --name "$BUILDER" --driver docker-container \
      --driver-opt "image=$BUILDKIT_IMAGE" >/dev/null
  fi
  r docker buildx inspect --bootstrap "$BUILDER" >/dev/null
}

# provenance prints the build args for the tree in $1.
provenance() {
  local src=$1 commit tag dirty=false
  git -C "$src" rev-parse HEAD >/dev/null 2>&1 || { echo "VERSION=dev"; return; }
  commit=$(git -C "$src" rev-parse HEAD)
  tag=$(git -C "$src" describe --tags --exact-match 2>/dev/null || true)
  [ -z "$(git -C "$src" status --porcelain)" ] || dirty=true
  echo "VERSION=${tag:-$(git -C "$src" describe --tags --always --dirty)}"
  echo "COMMIT=$commit"
  echo "TAG=$tag"
  echo "DIRTY=$dirty"
  echo "SOURCE_DATE_EPOCH=$(git -C "$src" log -1 --format=%ct)"
}

# build <out> [ref]: build from a clean checkout of ref, or from the working tree.
build() (
  out=$1 ref=${2:-} src=$top
  [ -n "$out" ] || { echo "usage: release.sh build <out-dir> [ref]" >&2; exit 2; }
  mkdir -p "$out" && out=$(cd "$out" && pwd)
  if [ -n "$ref" ]; then
    tmp=$(mktemp -d) && src="$tmp/src"
    git -C "$top" worktree add --quiet --detach "$src" "$ref"
    trap 'git -C "$top" worktree remove --force "$src"; rm -rf "$tmp"' EXIT
  fi
  args=()
  while IFS= read -r line; do args+=(--build-arg "$line"); done < <(provenance "$src")
  [ -z "${GOPROXY:-}" ] || args+=(--build-arg "GOPROXY=$GOPROXY")
  # proxy settings are predefined build args: they reach downloads but neither
  # the cache keys nor the output
  for p in HTTP_PROXY HTTPS_PROXY NO_PROXY; do [ -z "${!p:-}" ] || args+=(--build-arg "$p=${!p}"); done
  builder
  # NO_CACHE: rebuild everything except the downloaded dependencies (pinned by
  # lockfile and go.sum); NO_CACHE=all drops those too.
  nc=()
  case "${NO_CACHE:-}" in
    "") ;;
    all) nc=(--no-cache) ;;
    *) nc=(--no-cache-filter ui,gosrc,bin,pack,archives,rootfs) ;;
  esac
  rm -rf "$out/archives" "$out/image"
  echo "building archives from $src" >&2
  docker buildx build --builder "$BUILDER" ${nc[@]+"${nc[@]}"} "${args[@]}" \
    --target archives --output "type=local,dest=$out/archives" "$src"
  echo "building image for $PLATFORMS" >&2
  docker buildx build --builder "$BUILDER" ${nc[@]+"${nc[@]}"} "${args[@]}" \
    --platform "$PLATFORMS" --provenance=false --sbom=false \
    --output "type=oci,dest=$out/image,tar=false,oci-mediatypes=true,rewrite-timestamp=true,compression=gzip,force-compression=true" \
    "$src"
  { provenance "$src"; echo "BUILDKIT=$BUILDKIT_IMAGE"; echo "IMAGE_DIGEST=$(index_digest "$out/image")"; } > "$out/build.env"
  cat "$out/build.env"
)

# index_digest prints the digest of the image index in an OCI layout.
index_digest() { grep -o '"digest": *"sha256:[0-9a-f]*"' "$1/index.json" | head -1 | cut -d'"' -f4; }

# sumcheck checks SHA256SUMS in the current directory (GNU coreutils or macOS shasum).
sumcheck() { if command -v sha256sum >/dev/null; then sha256sum -c --quiet SHA256SUMS; else shasum -a 256 -c --quiet SHA256SUMS; fi; }

compare() {
  [ $# -eq 2 ] || { echo "usage: release.sh compare <left> <right>" >&2; exit 2; }
  gorun run ./scripts/repro compare "$(cd "$1" && pwd)" "$(cd "$2" && pwd)"
}

twice() {
  local work; work=$(mktemp -d)
  NO_CACHE=1 build "$work/a" "${1:-}"
  NO_CACHE=1 build "$work/b" "${1:-}"
  echo "== comparing two independent builds ($work)"
  compare "$work/a" "$work/b"
}

# verify <tag>: rebuild the tag and compare with the published archives and image.
verify() {
  local tag=${1:-} work
  [ -n "$tag" ] || { echo "usage: release.sh verify <tag>" >&2; exit 2; }
  work=$(mktemp -d)
  r git -C "$top" fetch --quiet --tags origin 2>/dev/null || true
  git -C "$top" rev-parse --verify --quiet "refs/tags/$tag" >/dev/null || { echo "tag $tag not found; fetch it first" >&2; exit 1; }
  echo "== downloading $tag from github.com/$REPO" >&2
  mkdir -p "$work/published/archives" "$work/published/image"
  for f in "${ARCHIVES[@]}" SHA256SUMS; do
    r curl -fsSL -o "$work/published/archives/$f" "https://github.com/$REPO/releases/download/$tag/$f"
  done
  (cd "$work/published/archives" && sumcheck) || { echo "published files do not match their SHA256SUMS" >&2; exit 1; }
  r docker buildx imagetools inspect "$IMAGE:$tag" --format '{{json .Manifest}}' > "$work/published/image/image.json"
  echo "== rebuilding $tag" >&2
  PLATFORMS=linux/amd64,linux/arm64 build "$work/rebuilt" "$tag"
  echo "== $tag: rebuilt (left) vs published (right)"
  compare "$work/rebuilt" "$work/published"
}

cmd=${1:-}; shift || true
case "$cmd" in
  build) build "${1:-}" "${2:-}" ;;
  compare) compare "$@" ;;
  twice) twice "${1:-}" ;;
  verify) verify "${1:-}" ;;
  env) provenance "$top" ;;
  *) echo "usage: release.sh build|compare|twice|verify|env …" >&2; exit 2 ;;
esac
