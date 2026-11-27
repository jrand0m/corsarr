# Verifying a corsarr release

Every release can be checked two ways:

1. **Without rebuilding**: the checksums, attestations and signature prove that a file or
   image came out of this repository's release workflow for a given tag.
2. **By rebuilding**: the tagged source plus one command reproduces the published bytes,
   so you do not have to trust the workflow at all.

The release notes of each version list the image digest, the builder (BuildKit) image
digest and the source date used for that build.

## What a release contains

| File | What it is |
| --- | --- |
| `corsarr-{linux,darwin}-{amd64,arm64}.tar.gz`, `corsarr-windows-{amd64,arm64}.zip` | the binary with `README.md`, `LICENSE` and an example config |
| `SHA256SUMS` | SHA-256 of the six archives |
| `corsarr-*.spdx.json` | an SPDX SBOM per archive and per image platform |
| `ghcr.io/jrand0m/corsarr:<tag>` | the multi-arch image (`linux/amd64`, `linux/arm64`) |

Every archive and the image have a build-provenance attestation (SLSA) and an SBOM
attestation, signed with the workflow's short-lived identity. The image also has a keyless
cosign signature. Nothing is signed with a long-lived key.

## 1. Check what you downloaded

Download an archive and `SHA256SUMS` from the release page, then:

```bash
# the file is the one the release lists
sha256sum -c SHA256SUMS --ignore-missing          # macOS: shasum -a 256 -c SHA256SUMS --ignore-missing

# it was built by this repository's release workflow, from the tagged commit
gh attestation verify corsarr-linux-amd64.tar.gz --repo jrand0m/corsarr

# its SBOM attestation (the dependency list it was built with)
gh attestation verify corsarr-linux-amd64.tar.gz --repo jrand0m/corsarr \
  --predicate-type https://spdx.dev/Document/v2.3
```

The output of `gh attestation verify` names the workflow, the tag and the commit. They must
be `.github/workflows/release.yml`, the version you expect, and that tag's commit.

For the container image, verify the tag you run (or better, pin its digest):

```bash
gh attestation verify oci://ghcr.io/jrand0m/corsarr:v1.2.3 --repo jrand0m/corsarr

cosign verify ghcr.io/jrand0m/corsarr:v1.2.3 \
  --certificate-identity-regexp '^https://github.com/jrand0m/corsarr/\.github/workflows/release\.yml@refs/tags/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

A running binary reports where it came from:

```bash
corsarr version --json
docker run --rm ghcr.io/jrand0m/corsarr:v1.2.3 version --json
```

```json
{
  "version": "v1.2.3",
  "commit": "<full commit of the tag>",
  "tag": "v1.2.3",
  "goVersion": "go1.27.4",
  "buildDate": "<commit time of the tag, UTC>",
  "dirty": false
}
```

The web UI shows the same under System › About. `buildDate` is the commit time, not the
time the build ran: that is what makes two builds identical.

## 2. Rebuild and compare

### What makes the build reproducible

- Every input is pinned: the Go and Node versions, the base images and the Dockerfile
  frontend by digest, the BuildKit image by digest, UI dependencies by `package-lock.json`
  and Go modules by `go.sum`.
- Binaries are built with `-trimpath`, `-buildvcs=false` and an empty build id; the version,
  commit, tag and build date are passed in as build arguments.
- `SOURCE_DATE_EPOCH` is the commit time of the tag. Archive entries are sorted, all carry
  that time, are owned by `0:0` with no user names, and have mode `0755` (directories and
  the binary) or `0644`. Gzip and zip headers carry no host name or build time.
- The image is exported with `rewrite-timestamp=true` (file times become
  `SOURCE_DATE_EPOCH`), OCI media types and forced gzip compression, without
  BuildKit's own provenance manifests (provenance is attached separately, as an
  attestation, so it does not change the image digest).

Every release is built twice, on an amd64 and an arm64 runner; the release fails if any of
the six archives or the image digest differs.

### With just

Requirements: git, Docker with buildx, curl, and Go (only for the comparison report; the
build itself runs in containers).

```bash
git clone https://github.com/jrand0m/corsarr && cd corsarr
just verify release v1.2.3
```

This checks out the tag in a temporary worktree, builds the six archives and the
two-platform image in the pinned builder, downloads the published archives and the
image's manifest list, and prints one line per artifact:

```
corsarr-darwin-amd64.tar.gz        identical
corsarr-darwin-arm64.tar.gz        identical
corsarr-linux-amd64.tar.gz         identical
corsarr-linux-arm64.tar.gz         identical
corsarr-windows-amd64.zip          identical
corsarr-windows-arm64.zip          identical
image sha256:0d4b5c1e2f3a          identical
```

When something differs it names the first difference: the first archive entry whose
content or header differs, or the first image platform whose manifest differs. Other
commands:

```bash
just verify rebuild               # build this checkout twice from scratch and compare
just verify rebuild v1.2.3        # the same for a tag
just build release                # one build into dist/archives and dist/image
just verify compare dir1 dir2     # compare two build outputs
```

### With plain docker

The same steps without just or Go. Use the BuildKit image named in the release notes (it
is also `BUILDKIT_IMAGE` in `scripts/release.sh` at that tag).

```bash
git clone https://github.com/jrand0m/corsarr && cd corsarr
git checkout v1.2.3

export VERSION=v1.2.3 TAG=v1.2.3 DIRTY=false
export COMMIT=$(git rev-parse HEAD)
export SOURCE_DATE_EPOCH=$(git log -1 --format=%ct)

docker buildx create --name corsarr-repro --driver docker-container \
  --driver-opt image=moby/buildkit:v0.34.2@sha256:c5f8862f52499773af85bc07695b2379ef08e8002352b0df2c4864c91d78dabe
args="--build-arg VERSION --build-arg COMMIT --build-arg TAG --build-arg DIRTY --build-arg SOURCE_DATE_EPOCH"

# the six archives and their SHA256SUMS
docker buildx build --builder corsarr-repro $args --target archives \
  --output type=local,dest=rebuilt .
curl -fsSLO https://github.com/jrand0m/corsarr/releases/download/v1.2.3/SHA256SUMS
diff SHA256SUMS rebuilt/SHA256SUMS && echo "archives identical"

# the image
docker buildx build --builder corsarr-repro $args \
  --platform linux/amd64,linux/arm64 --provenance=false --sbom=false \
  --output type=oci,dest=rebuilt-image,tar=false,oci-mediatypes=true,rewrite-timestamp=true,compression=gzip,force-compression=true .
grep -o '"digest": *"sha256:[0-9a-f]*"' rebuilt-image/index.json | head -1
docker buildx imagetools inspect ghcr.io/jrand0m/corsarr:v1.2.3 --format '{{json .Manifest.Digest}}'
```

The two digests printed at the end must be equal. `--no-cache` forces a build that reuses
nothing from earlier runs.

### If a rebuild differs

Check first that the tree is clean (`git status`), that the build args match the release
notes, and that you used the BuildKit image the release names: a different BuildKit
version can compress image layers differently. Archives do not depend on BuildKit. If it
still differs, please open an issue with the output of `just verify release <tag>`.
