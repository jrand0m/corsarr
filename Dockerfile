# syntax=docker/dockerfile:1
# Multi-arch image: the UI is built once and Go cross-compiles for each target
# on the build machine, so no emulation is needed. The runtime is scratch plus
# the binary, CA roots and a non-root user.

# Stage 1: UI bundle (platform independent)
FROM --platform=$BUILDPLATFORM node:22.23.5-alpine AS ui
WORKDIR /src/ui
COPY ui/package.json ui/package-lock.json ./
RUN for i in 1 2 3; do npm ci --no-audit --no-fund && break; sleep $((i*10)); done
COPY ui/ ./
RUN npm run build

# Stage 2: static Go binary with the UI embedded
FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine AS go
ARG VERSION=dev
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN for i in 1 2 3; do go mod download && break; sleep $((i*10)); done
COPY cmd/ cmd/
COPY internal/ internal/
COPY --from=ui /src/internal/web/dist/ internal/web/dist/
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /corsarr ./cmd/corsarr
# CA roots (golang:alpine ships them) and a non-root user, uid/gid 65532
RUN mkdir -p /rootfs/etc/ssl/certs /rootfs/data /rootfs/tmp \
 && cp /etc/ssl/certs/ca-certificates.crt /rootfs/etc/ssl/certs/ \
 && printf 'root:x:0:0:root:/root:/sbin/nologin\nnonroot:x:65532:65532:nonroot:/home/nonroot:/sbin/nologin\n' > /rootfs/etc/passwd \
 && printf 'root:x:0:\nnonroot:x:65532:\n' > /rootfs/etc/group \
 && chown 65532:65532 /rootfs/data && chmod 1777 /rootfs/tmp

# Stage 3: runtime
FROM scratch
COPY --from=go /rootfs/ /
COPY --from=go /corsarr /corsarr
# Defaults: data.dir=/data, server.address=:8090; configure with
# /etc/corsarr/corsarr.yml, --args or CORSARR_* environment variables.
ENV GOMEMLIMIT=96MiB GOGC=50
HEALTHCHECK --interval=30s --timeout=5s CMD ["/corsarr", "healthcheck"]
VOLUME ["/data"]
EXPOSE 8090
USER 65532:65532
ENTRYPOINT ["/corsarr"]
