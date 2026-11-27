# corsarr — all project interaction goes through just.
# Convention: just <domain> <subdomain> <targets…> <options>

set shell := ["bash", "-euo", "pipefail", "-c"]
set positional-arguments

x := "mise exec --"
version := `git describe --tags --always --dirty 2>/dev/null || echo dev`
ldflags := "-s -w -X main.version=" + version

default:
    @just --list --unsorted

# ── setup ────────────────────────────────────────────────────────────────────
# just setup [all|tools|ui|go]
setup target="all":
    #!/usr/bin/env bash
    set -euo pipefail
    r() { for i in 1 2 3 4 5; do "$@" && return 0; echo "retry $i…" >&2; sleep $((i*5)); done; return 1; }
    case "{{target}}" in
      tools) r mise install ;;
      go)    r {{x}} go mod download ;;
      ui)    cd ui && r {{x}} npm ci --no-audit --no-fund ;;
      all)   just setup tools; just setup go; just setup ui ;;
      *) echo "unknown target {{target}} (all|tools|go|ui)"; exit 2 ;;
    esac

# ── build ────────────────────────────────────────────────────────────────────
# just build [all|ui|go|storybook|image] [os/arch]   e.g. just build go linux/arm64
build target="all" platform="":
    #!/usr/bin/env bash
    set -euo pipefail
    platform="{{platform}}"
    case "{{target}}" in
      ui)
        cd ui && {{x}} npm run build ;;
      storybook)
        cd ui && {{x}} npm run build-storybook ;;
      go)
        plat="{{platform}}"; [ -z "$plat" ] && plat="$({{x}} go env GOOS)/$({{x}} go env GOARCH)"
        os="${plat%/*}"; arch="${plat#*/}"; arm=""
        case "$arch" in armv7) arch=arm; arm=7;; armv6) arch=arm; arm=6;; esac
        mkdir -p bin
        out="bin/corsarr-${os}-${arch}${arm:+v$arm}"
        CGO_ENABLED=0 GOOS=$os GOARCH=$arch GOARM=$arm {{x}} go build -trimpath -ldflags "{{ldflags}}" -o "$out" ./cmd/corsarr
        ls -lh "$out" ;;
      all)
        just build ui; just build go {{platform}} ;;
      image)
        docker buildx build --platform "${platform:-linux/amd64,linux/arm64}" --build-arg VERSION={{version}} -t corsarr:dev . ;;
      *) echo "unknown target {{target}} (all|ui|go|storybook|image)"; exit 2 ;;
    esac

# ── run ──────────────────────────────────────────────────────────────────────
# just run [dev|ui|storybook]
run target="dev":
    #!/usr/bin/env bash
    set -euo pipefail
    case "{{target}}" in
      dev)       mkdir -p data && {{x}} go run ./cmd/corsarr --data.dir=./data ;;
      ui)        cd ui && {{x}} npm run dev ;;
      storybook) cd ui && {{x}} npm run storybook ;;
      *) echo "unknown target {{target}} (dev|ui|storybook)"; exit 2 ;;
    esac

# ── test ─────────────────────────────────────────────────────────────────────
# just test [fast|full|go|ui|budget] [package-pattern]
# just test container [docker|podman]   build the image and run it unprivileged, any-uid and read-only
test kind="fast" pkg="./...":
    #!/usr/bin/env bash
    set -euo pipefail
    case "{{kind}}" in
      fast)   {{x}} go test -short -timeout 300s {{pkg}} ;;
      go)     {{x}} go test -race -timeout 300s -coverprofile=coverage.out {{pkg}} ;;
      ui)     cd ui && {{x}} npm run test ;;
      budget) just build go && scripts/budget.sh ;;
      full)   just test go; just test ui; just test budget ;;
      container)
        engine="{{pkg}}"; [ "$engine" = "./..." ] && engine=docker
        "$engine" build --build-arg VERSION={{version}} -t corsarr:container-test .
        scripts/container-test.sh corsarr:container-test "$engine" ;;
      *) echo "unknown kind {{kind}} (fast|full|go|ui|budget|container)"; exit 2 ;;
    esac

# ── docs ─────────────────────────────────────────────────────────────────────
# just docs [config] [out-dir]   regenerate the configuration reference and example file
docs target="config" out=".":
    #!/usr/bin/env bash
    set -euo pipefail
    case "{{target}}" in
      config)
        mkdir -p "{{out}}/docs" "{{out}}/deploy"
        {{x}} go run ./cmd/corsarr config defaults --format=markdown > "{{out}}/docs/configuration.md"
        {{x}} go run ./cmd/corsarr config defaults --format=yaml > "{{out}}/deploy/corsarr.example.yml" ;;
      *) echo "unknown target {{target}} (config)"; exit 2 ;;
    esac

# ── lint ─────────────────────────────────────────────────────────────────────
# just lint [all|go|ui|docs|quality]
lint target="all":
    #!/usr/bin/env bash
    set -euo pipefail
    case "{{target}}" in
      go)      test -z "$({{x}} gofmt -l cmd internal)" || { {{x}} gofmt -l cmd internal; exit 1; }; {{x}} go vet ./... ;;
      ui)      cd ui && {{x}} npm run check ;;
      docs)
        tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT
        just docs config "$tmp"
        for f in docs/configuration.md deploy/corsarr.example.yml; do
          diff -u "$f" "$tmp/$f" || { echo "$f is out of date: run just docs config"; exit 1; }
        done ;;
      quality) scripts/quality.sh ;;
      all)     just lint go; just lint ui; just lint docs ;;
      *) echo "unknown target {{target}} (all|go|ui|docs|quality)"; exit 2 ;;
    esac
