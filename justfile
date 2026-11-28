# corsarr — all project interaction goes through just.
# Convention: just <domain> <subdomain> <targets…> <options>

set shell := ["bash", "-euo", "pipefail", "-c"]
set positional-arguments

x := "mise exec --"

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
# just build [all|ui|go|storybook|image|release] [os/arch]   e.g. just build go linux/arm64
# (release: the six archives and the OCI image in dist/, built in the pinned builder)
build target="all" platform="":
    #!/usr/bin/env bash
    set -euo pipefail
    platform="{{platform}}"
    # version, commit, tag, build date and dirty flag of this checkout
    set -a; eval "$(scripts/release.sh env)"; set +a
    case "{{target}}" in
      ui)
        cd ui && {{x}} npm run build ;;
      storybook)
        cd ui && {{x}} npm run build-storybook ;;
      go)
        plat="{{platform}}"; [ -z "$plat" ] && plat="$({{x}} go env GOOS)/$({{x}} go env GOARCH)"
        os="${plat%/*}"; arch="${plat#*/}"
        mkdir -p bin
        out="bin/corsarr-${os}-${arch}"
        {{x}} scripts/gobuild.sh "$os" "$arch" "$out"
        ls -lh "$out" ;;
      all)
        just build ui; just build go {{platform}} ;;
      image)
        docker buildx build --platform "${platform:-linux/amd64,linux/arm64}" \
          --build-arg VERSION --build-arg COMMIT --build-arg TAG --build-arg DIRTY --build-arg SOURCE_DATE_EPOCH -t corsarr:dev . ;;
      release)
        PLATFORMS="${platform:-linux/amd64,linux/arm64}" scripts/release.sh build dist ;;
      *) echo "unknown target {{target}} (all|ui|go|storybook|image|release)"; exit 2 ;;
    esac

# ── verify ───────────────────────────────────────────────────────────────────
# just verify release <tag>        rebuild a published release; identical / first difference per artifact
# just verify rebuild [ref]        build twice from scratch and compare (default: this working tree)
# just verify compare <dir> <dir>  compare two build outputs (archives/ and image/ in each)
verify target ref="" other="":
    #!/usr/bin/env bash
    set -euo pipefail
    case "{{target}}" in
      release) [ -n "{{ref}}" ] || { echo "usage: just verify release <tag>"; exit 2; }; scripts/release.sh verify "{{ref}}" ;;
      rebuild) scripts/release.sh twice "{{ref}}" ;;
      compare) scripts/release.sh compare "{{ref}}" "{{other}}" ;;
      *) echo "unknown target {{target}} (release|rebuild|compare)"; exit 2 ;;
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
        set -a; eval "$(scripts/release.sh env)"; set +a
        "$engine" build --build-arg VERSION --build-arg COMMIT --build-arg TAG --build-arg DIRTY --build-arg SOURCE_DATE_EPOCH \
          -t corsarr:container-test .
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
      go)      test -z "$({{x}} gofmt -l cmd internal sdk plugins)" || { {{x}} gofmt -l cmd internal sdk plugins; exit 1; }; {{x}} go vet ./... ;;
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

# ── deploy ───────────────────────────────────────────────────────────────────
# just deploy [validate|smoke] [all|<target>]   e.g. just deploy validate helm, just deploy smoke compose
deploy action="validate" target="all":
    {{x}} scripts/deploy-samples.sh {{action}} {{target}}

# ── release ──────────────────────────────────────────────────────────────────
# just release pins <vX.Y.Z> [SHA256SUMS]   move every deploy/ sample to a published release
# just release deps                         build tool pins (nfpm, BuildKit): consistent and current
release action version="" sums="":
    #!/usr/bin/env bash
    set -euo pipefail
    case "{{action}}" in
      pins) scripts/pins.sh "{{version}}" {{sums}} ;;
      deps) scripts/release.sh deps ;;
      *) echo "unknown action {{action}} (pins|deps)"; exit 2 ;;
    esac
