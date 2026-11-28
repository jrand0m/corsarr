# syntax=docker/dockerfile:1@sha256:572cebbc2b15e6f84c077742952674889cb63cd5e9b80abdffcbe587b6f19183
# check=skip=FromPlatformFlagConstDisallowed
# Reproducible build of corsarr: the release archives (target "archives") and
# the multi-arch runtime image (default target). Every input is pinned: base
# images by digest, Node and Go by version, dependencies by lockfile and go.sum.
# The UI is built once and Go cross-compiles on the build machine, so no
# emulation is needed. Two builds of one commit with the same build args give
# identical bytes; VERIFYING.md shows how to check a release.
#
# Build args (scripts/release.sh fills them from git):
#   VERSION, COMMIT, TAG, DIRTY   provenance stamped into the binary
#   SOURCE_DATE_EPOCH             commit time; archive mtimes, image timestamps, build date

# Dependencies, fixed by the lockfile and go.sum (the stages a rebuild may reuse)
FROM --platform=$BUILDPLATFORM node:22.23.5-alpine@sha256:1314fb30ec37592d5845e5a160d522c0e680972aa534549940b8d73bde88481a AS uideps
WORKDIR /src/ui
COPY ui/package.json ui/package-lock.json ./
RUN ok=; for i in 1 2 3 4 5; do npm ci --no-audit --no-fund && ok=1 && break; sleep $((i*10)); done; [ -n "$ok" ]

FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS godeps
ENV CGO_ENABLED=0 GOTOOLCHAIN=local GOFLAGS=-mod=readonly
WORKDIR /src
COPY go.mod go.sum ./
# Optional module mirror; go.sum pins every module, so the source cannot change the result.
ARG GOPROXY
RUN ok=; for i in 1 2 3 4 5; do go mod download && ok=1 && break; sleep $((i*10)); done; [ -n "$ok" ]

# Packaging tool, pinned by version and verified against the Go checksum database;
# `just release deps` checks it against NFPM_VERSION in the deploy-samples workflow
# and the newest release
FROM godeps AS tools
RUN ok=; for i in 1 2 3 4 5; do go install github.com/goreleaser/nfpm/v2/cmd/nfpm@v2.43.0 && ok=1 && break; sleep $((i*10)); done; [ -n "$ok" ]

# UI bundle (platform independent)
FROM uideps AS ui
COPY ui/ ./
RUN npm run build

# Go sources with the UI embedded, ready to cross-compile; the plugin SDK and
# the plugin modules' sources come along (the binary links the SDK's ABI types)
FROM godeps AS gosrc
COPY scripts/gobuild.sh scripts/plugins.sh scripts/
COPY scripts/repro/ scripts/repro/
COPY scripts/pluginindex/ scripts/pluginindex/
COPY cmd/ cmd/
COPY internal/ internal/
COPY sdk/ sdk/
COPY plugins/ plugins/
COPY --from=ui /src/internal/web/dist/ internal/web/dist/

# One binary for the image platform
FROM gosrc AS bin
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=
ARG TAG=
ARG DIRTY=false
ARG SOURCE_DATE_EPOCH=0
RUN scripts/gobuild.sh "$TARGETOS" "$TARGETARCH" /corsarr

# The six release archives, Linux packages (.deb, .rpm, .apk for amd64 and
# arm64; release versions only), the plugin modules (<id>-<version>.wasm, one
# file for every platform) with plugins-index.json, and SHA256SUMS
FROM gosrc AS pack
ARG VERSION=dev
ARG COMMIT=
ARG TAG=
ARG DIRTY=false
ARG SOURCE_DATE_EPOCH=0
COPY --from=tools /go/bin/nfpm /usr/local/bin/nfpm
COPY README.md LICENSE ./
COPY deploy/corsarr.example.yml deploy/
COPY deploy/systemd/ deploy/systemd/
COPY deploy/packages/ deploy/packages/
# file modes and times from the checkout depend on the umask and the clock;
# packages (the apk control scripts among them) take them as they are
RUN find README.md LICENSE deploy -type d -exec chmod 0755 {} + \
 && find README.md LICENSE deploy -type f -exec chmod 0644 {} + \
 && find README.md LICENSE deploy -exec touch -d "@$SOURCE_DATE_EPOCH" {} + \
 && go build -o /usr/local/bin/repro ./scripts/repro \
 && mkdir -p /dist \
 && for t in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do \
      os=${t%/*} arch=${t#*/} name="corsarr-${t%/*}-${t#*/}" ext= fmt=tar.gz; \
      [ "$os" = windows ] && ext=.exe fmt=zip; \
      mkdir -p "/stage/$name" && cp README.md LICENSE deploy/corsarr.example.yml "/stage/$name/" \
      && scripts/gobuild.sh "$os" "$arch" "/stage/$name/corsarr$ext" \
      && repro pack "/stage/$name" "/dist/$name.$fmt" || exit 1; \
    done \
 && case "$VERSION" in v[0-9]*) \
      for arch in amd64 arm64; do \
        install -D -m 0755 "/stage/corsarr-linux-$arch/corsarr" dist/package/corsarr \
        && for p in deb rpm apk; do \
             VERSION="${VERSION#v}" ARCH=$arch nfpm package --config deploy/packages/nfpm.yaml --packager $p --target /dist/ || exit 1; \
           done; \
      done ;; \
    esac \
 && scripts/plugins.sh build /stage/plugins all \
 && cp /stage/plugins/*.wasm /dist/ \
 && min= && case "$TAG" in v[0-9]*) min=${TAG#v} ;; esac \
 && go run ./scripts/pluginindex build -release "$VERSION" -min-corsarr "$min" -epoch "$SOURCE_DATE_EPOCH" \
      -o /dist/plugins-index.json /dist/*.wasm \
 && cd /dist && sha256sum corsarr[-_]* *.wasm plugins-index.json > SHA256SUMS

FROM scratch AS archives
COPY --from=pack /dist/ /

# CA roots always come from the amd64 variant of the pinned image, so the
# image bytes do not depend on the build machine. Nothing runs in this stage.
FROM --platform=linux/amd64 golang:1.27.4-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS certs

# Runtime file system: CA roots and a non-root user, uid/gid 65532
FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS rootfs
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /rootfs/etc/ssl/certs/
RUN mkdir -p /rootfs/data /rootfs/tmp \
 && printf 'root:x:0:0:root:/root:/sbin/nologin\nnonroot:x:65532:65532:nonroot:/home/nonroot:/sbin/nologin\n' > /rootfs/etc/passwd \
 && printf 'root:x:0:\nnonroot:x:65532:\n' > /rootfs/etc/group \
 && chown 65532:65532 /rootfs/data && chmod 1777 /rootfs/tmp

# Runtime: scratch plus the binary
FROM scratch
ARG VERSION=dev
ARG COMMIT=
COPY --from=rootfs /rootfs/ /
COPY --from=bin /corsarr /corsarr
LABEL org.opencontainers.image.source="https://github.com/jrand0m/corsarr" \
      org.opencontainers.image.licenses="AGPL-3.0-or-later" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}"
# Defaults: data.dir=/data, server.address=:8090; configure with
# /etc/corsarr/corsarr.yml, --args or CORSARR_* environment variables.
ENV GOMEMLIMIT=96MiB GOGC=50
HEALTHCHECK --interval=30s --timeout=5s CMD ["/corsarr", "healthcheck"]
VOLUME ["/data"]
EXPOSE 8090
USER 65532:65532
ENTRYPOINT ["/corsarr"]
