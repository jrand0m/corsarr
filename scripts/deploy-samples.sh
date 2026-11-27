#!/usr/bin/env bash
# Validate and smoke-run the deployment samples in deploy/.
#
#   scripts/deploy-samples.sh validate <target|all>   lint, render and dry-run
#   scripts/deploy-samples.sh smoke <target>          start it, check health and the first-run page
#
# A check whose tool is missing is skipped and reported; STRICT=1 turns skips into failures
# (CI). Smoke runs use the image pinned in the samples; SMOKE_IMAGE=<local image> is
# re-tagged as that reference first, so a freshly built image is what runs.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
dep="$root/deploy"
cd "$root"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

validate_targets=(config docker compose helm kubernetes systemd podman packages homebrew windows launchd go-install unraid nas swarm nomad aur)
smoke_targets=(docker compose helm kubernetes podman packages)

pass=() skip=() fail=()
ok()    { pass+=("$1"); echo "  ok    $1"; }
skipt() { skip+=("$1 ($2)"); echo "  skip  $1: $2"; }
bad()   { fail+=("$1"); echo "  FAIL  $1" >&2; }
has()   { command -v "$1" >/dev/null 2>&1; }
# check <name> <command…>: run a check and record the result
check() { local n="$1"; shift; if "$@" >"$work/out" 2>&1; then ok "$n"; else cat "$work/out" >&2; bad "$n"; fi; }
need()  { local n="$1"; shift; for t in "$@"; do has "$t" || { skipt "$n" "$t not installed"; return 1; }; done; }
retry() { for i in 1 2 3 4 5; do "$@" && return 0; echo "retry $i…" >&2; sleep $((i * 5)); done; return 1; }

pinned_image() { sed -n 's/^ *image: *\(ghcr.io\/jrand0m\/corsarr:[^ ]*\).*/\1/p' "$dep/compose/compose.yml" | head -1; }

corsarr_bin() {
  if [ -n "${CORSARR_BIN:-}" ]; then echo "$CORSARR_BIN"; return; fi
  if [ ! -x "$work/corsarr" ]; then
    CGO_ENABLED=0 go build -o "$work/corsarr" ./cmd/corsarr >&2
  fi
  echo "$work/corsarr"
}

kubeconform_run() {
  kubeconform -strict -summary -kubernetes-version "${KUBE_VERSION:-1.31.0}" \
    -schema-location default \
    -schema-location 'https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json' "$@"
}

# ── validate ─────────────────────────────────────────────────────────────────

v_config() {
  echo "config files (corsarr config validate)"
  has go || [ -n "${CORSARR_BIN:-}" ] || { skipt config "go not installed"; return; }
  local bin; bin="$(corsarr_bin)"
  mkdir -p "$work/secrets" && echo secret > "$work/secrets/qbit_password"
  for f in compose/corsarr.yml kubernetes/base/corsarr.yml kubernetes/overlays/shadow/corsarr.yml \
           podman/corsarr.yml systemd/corsarr.yml launchd/corsarr.yml; do
    sed -e "s#/run/secrets/#$work/secrets/#g" -e "s#__HOME__#$HOME#g" "$dep/$f" > "$work/cfg.yml"
    check "config $f" env "${sample_env[@]}" "$bin" config validate --configFile="$work/cfg.yml"
  done
  if has helm; then
    helm template t "$dep/helm/corsarr" --show-only templates/configmap.yaml \
      | sed -n '/corsarr.yml: |/,$p' | tail -n +2 | sed 's/^    //' > "$work/helm.yml"
    check "config helm values" "$bin" config validate --configFile="$work/helm.yml"
  fi
}

v_docker() {
  echo "docker"
  check "docker run.sh syntax" sh -n "$dep/docker/run.sh"
  if has shellcheck; then check "docker run.sh shellcheck" shellcheck "$dep/docker/run.sh"; else skipt "docker shellcheck" "shellcheck not installed"; fi
}

# placeholder values for the secrets the samples reference
sample_env=(TMDB_API_KEY=placeholder INDEXER_API_KEY=placeholder)

compose_config() { env "${sample_env[@]}" docker compose --project-directory "$(dirname "$1")" -f "$1" config -q; }

v_compose() {
  echo "compose"
  need compose docker || return 0
  docker compose version >/dev/null 2>&1 || { skipt compose "docker compose plugin not installed"; return; }
  for f in compose/compose.yml compose/compose.qbittorrent.yml docker-compose.yml; do
    check "compose $f" compose_config "$dep/$f"
  done
}

v_helm() {
  echo "helm"
  need helm helm || return 0
  local c="$dep/helm/corsarr"
  check "helm lint" helm lint --strict "$c"
  check "helm lint full values" helm lint --strict "$c" -f "$c/ci/full-values.yaml"
  helm template t "$c" > "$work/helm-default.yaml"
  helm template t "$c" -f "$c/ci/full-values.yaml" > "$work/helm-full.yaml"
  ok "helm template"
  check "helm appVersion matches pinned image" test "corsarr:$(sed -n 's/^appVersion: *"\(.*\)"/\1/p' "$c/Chart.yaml")" = "$(pinned_image | sed 's#.*/##')"
  if has kubeconform; then
    check "helm kubeconform" kubeconform_run "$work/helm-default.yaml" "$work/helm-full.yaml"
  else skipt "helm kubeconform" "kubeconform not installed"; fi
}

v_kubernetes() {
  echo "kubernetes (kustomize)"
  local k
  if has kubectl; then k="kubectl kustomize"; elif has kustomize; then k="kustomize build"; else skipt kubernetes "kubectl not installed"; return; fi
  for o in base overlays/shadow overlays/ingress; do
    check "kustomize $o" sh -c "$k '$dep/kubernetes/$o' > '$work/k-${o//\//-}.yaml'"
  done
  check "kubernetes namespace enforces restricted" grep -q 'pod-security.kubernetes.io/enforce: restricted' "$work/k-base.yaml"
  if has kubeconform; then
    check "kubernetes kubeconform" kubeconform_run "$work"/k-*.yaml
  else skipt "kubernetes kubeconform" "kubeconform not installed"; fi
}

v_systemd() {
  echo "systemd"
  need systemd systemd-analyze || return 0
  # verify needs ExecStart to exist: point it at any binary when corsarr is not installed
  local unit="$work/systemd/corsarr.service" exe=/usr/bin/corsarr
  mkdir -p "$work/systemd"
  [ -x "$exe" ] || exe="$(command -v true)"
  sed "s#^ExecStart=/usr/bin/corsarr#ExecStart=$exe#" "$dep/systemd/corsarr.service" > "$unit"
  check "systemd-analyze verify" systemd-analyze verify "$unit"
  check "systemd-analyze security (exposure <= ${SYSTEMD_EXPOSURE:-2.5})" \
    systemd-analyze security --offline=true --threshold="${SYSTEMD_EXPOSURE:-2.5}" "$unit"
  if has systemd-sysusers; then
    check "sysusers syntax" systemd-sysusers --dry-run "$dep/systemd/corsarr.sysusers"
  fi
}

v_podman() {
  echo "podman quadlet"
  local q=""
  for p in /usr/libexec/podman/quadlet /usr/lib/podman/quadlet; do [ -x "$p" ] && q="$p"; done
  [ -n "$q" ] || { skipt podman "quadlet not installed"; return; }
  mkdir -p "$work/quadlet"
  cp "$dep/podman/corsarr.container" "$dep/podman/corsarr-data.volume" "$work/quadlet/"
  check "quadlet dry-run" sh -c "QUADLET_UNIT_DIRS='$work/quadlet' '$q' -dryrun -user > '$work/quadlet.out' && grep -q 'ExecStart=.*podman run' '$work/quadlet.out'"
}

# nfpm_build <packager> <binary> <out-dir>: package from a staging tree, so nothing is
# written into the checkout (nfpm reads dist/package/corsarr relative to its working dir)
nfpm_build() {
  local stage="$work/nfpm-stage"
  mkdir -p "$stage/dist/package" "$3"
  ln -sfn "$dep" "$stage/deploy"; cp "$root/README.md" "$root/LICENSE" "$stage/"
  cp "$2" "$stage/dist/package/corsarr"
  (cd "$stage" && VERSION=0.0.0 ARCH=amd64 nfpm package --config deploy/packages/nfpm.yaml --packager "$1" --target "$3/")
}

v_packages() {
  echo "packages (nfpm)"
  need packages nfpm || return 0
  printf '#!/bin/sh\necho corsarr\n' > "$work/fake-corsarr" && chmod +x "$work/fake-corsarr"
  for p in deb rpm apk; do
    check "nfpm $p" nfpm_build "$p" "$work/fake-corsarr" "$work/pkg"
  done
  for s in "$dep"/packages/scripts/*.sh "$dep/packages/openrc/corsarr"; do
    check "sh -n ${s#"$dep"/}" sh -n "$s"
  done
}

v_homebrew() {
  echo "homebrew"
  need homebrew ruby || return 0
  check "homebrew formula syntax" ruby -c "$dep/homebrew/corsarr.rb"
  if has brew && [ "${BREW_AUDIT:-0}" = 1 ]; then
    local tap=corsarr-ci/samples
    brew untap "$tap" >/dev/null 2>&1 || true
    brew tap-new --no-git "$tap" >/dev/null
    brew trust "$tap" >/dev/null 2>&1 || true   # newer Homebrew only loads trusted local taps
    cp "$dep/homebrew/corsarr.rb" "$(brew --repository "$tap")/Formula/"
    check "brew audit" brew audit --strict --formula "$tap/corsarr"
    brew untap "$tap" >/dev/null 2>&1 || true
  else skipt "brew audit" "set BREW_AUDIT=1 (creates a temporary local tap)"; fi
}

v_windows() {
  echo "windows manifests"
  if has jq; then
    check "scoop manifest json" jq -e '.version and .architecture."64bit".hash and .bin' "$dep/windows/scoop/corsarr.json"
  else skipt "scoop manifest" "jq not installed"; fi
  if has yamllint; then
    check "winget manifests yaml" yamllint -d '{extends: relaxed, rules: {line-length: disable}}' "$dep/windows/winget"
  else skipt "winget manifests" "yamllint not installed"; fi
  check "winget versions agree" test "$(grep -h '^PackageVersion:' "$dep"/windows/winget/*.yaml | sort -u | wc -l | tr -d ' ')" = 1
}

v_launchd() {
  echo "launchd"
  local f="$dep/launchd/com.github.jrand0m.corsarr.plist"
  if has plutil; then check "plutil -lint" plutil -lint "$f"
  elif has xmllint; then check "plist xmllint" xmllint --noout "$f"
  else skipt launchd "plutil/xmllint not installed"; fi
}

v_go_install() {
  echo "go install"
  need go-install go || return 0
  check "go install module path" sh -c "GOBIN='$work/gobin' CGO_ENABLED=0 go install ./cmd/corsarr && '$work/gobin/corsarr' version"
}

v_unraid() {
  echo "unraid"
  need unraid xmllint || return 0
  check "unraid template xml" xmllint --noout "$dep/unraid/corsarr.xml"
}

v_nas() {
  echo "nas"
  need nas docker || return 0
  docker compose version >/dev/null 2>&1 || { skipt nas "docker compose plugin not installed"; return; }
  check "nas compose" compose_config "$dep/nas/compose.yml"
}

v_swarm() {
  echo "swarm"
  need swarm docker || return 0
  check "swarm stack config" env "${sample_env[@]}" sh -c "cd '$dep/swarm' && docker stack config -c stack.yml >/dev/null"
}

v_nomad() {
  echo "nomad"
  need nomad nomad || return 0
  check "nomad fmt" nomad fmt -check "$dep/nomad/corsarr.nomad.hcl"
}

sha256() { if has sha256sum; then sha256sum "$1"; else shasum -a 256 "$1"; fi | cut -d' ' -f1; }

aur_hashes() { # the PKGBUILD's local-file hashes match deploy/systemd (fix: updpkgsums)
  local want got
  want="$(sed -n '/^sha256sums=(/,/)/p' "$dep/aur/PKGBUILD" | grep -o '[0-9a-f]\{64\}')"
  got="$(for f in corsarr.service corsarr.sysusers corsarr.yml; do sha256 "$dep/systemd/$f"; done)"
  [ "$want" = "$got" ] || { echo "deploy/aur/PKGBUILD sha256sums are stale for deploy/systemd files; expected:"; echo "$got"; return 1; }
}

v_aur() {
  echo "aur"
  check "aur local-file hashes" aur_hashes
  need aur namcap || return 0
  check "aur namcap" namcap "$dep/aur/PKGBUILD"
}

# ── smoke ────────────────────────────────────────────────────────────────────

wait_http() { # wait_http <base-url>: health ok and the first-run page served
  local u="$1"
  for _ in $(seq 1 60); do
    if curl -fsS "$u/api/v1/health" >/dev/null 2>&1; then
      curl -fsS "$u/api/v1/firstrun" >/dev/null && curl -fsS "$u/" | grep -qi '<html'
      return
    fi
    sleep 2
  done
  return 1
}

prep_image() {
  local ref; ref="$(pinned_image)"
  if [ -n "${SMOKE_IMAGE:-}" ]; then docker tag "$SMOKE_IMAGE" "$ref"
  else retry docker pull -q "$ref" >/dev/null; fi
  echo "$ref"
}

s_docker() {
  prep_image >/dev/null
  local media="$work/media"; mkdir -p "$media"; chmod 0777 "$media"
  NAME=corsarr-smoke VOLUME=corsarr-smoke-data PORT=18090 MEDIA_DIR="$media" "$dep/docker/run.sh"
  trap 'docker rm -f corsarr-smoke >/dev/null 2>&1; docker volume rm corsarr-smoke-data >/dev/null 2>&1; rm -rf "$work"' EXIT
  check "smoke docker" wait_http http://127.0.0.1:18090
  for _ in $(seq 1 30); do [ "$(docker inspect -f '{{.State.Health.Status}}' corsarr-smoke)" = healthy ] && break; sleep 2; done
  check "smoke docker healthcheck" test "$(docker inspect -f '{{.State.Health.Status}}' corsarr-smoke)" = healthy
}

s_compose() {
  prep_image >/dev/null
  local media="$work/media"; mkdir -p "$media"; chmod 0777 "$media"
  # run a copy, so the secret file and .env never land in the checkout
  cp -R "$dep/compose" "$work/compose"
  echo placeholder > "$work/compose/secrets/qbit_password.txt"
  printf 'MEDIA_DIR=%s\nTMDB_API_KEY=placeholder\nINDEXER_API_KEY=placeholder\n' "$media" > "$work/compose/.env"
  compose_smoke=(docker compose -p corsarr-smoke --project-directory "$work/compose" -f "$work/compose/compose.yml")
  trap '"${compose_smoke[@]}" down -v >/dev/null 2>&1; rm -rf "$work"' EXIT
  "${compose_smoke[@]}" up -d --wait --wait-timeout 120
  check "smoke compose" wait_http http://127.0.0.1:8090
}

kind_cluster() {
  need kind kind kubectl || return 1
  kind get clusters 2>/dev/null | grep -qx corsarr-smoke || kind create cluster --name corsarr-smoke --wait 120s
  kind load docker-image --name corsarr-smoke "$(prep_image)"
}

port_forward_check() { # <namespace> <service> <name>
  kubectl -n "$1" port-forward "svc/$2" 18091:8090 >/dev/null 2>&1 & local pf=$!
  sleep 3
  check "$3" wait_http http://127.0.0.1:18091
  kill "$pf" 2>/dev/null || true
}

s_helm() {
  need helm helm || return 0
  kind_cluster || return 0
  trap 'kind delete cluster --name corsarr-smoke >/dev/null 2>&1; rm -rf "$work"' EXIT
  kubectl create namespace corsarr-helm
  kubectl label namespace corsarr-helm pod-security.kubernetes.io/enforce=restricted
  check "smoke helm install" helm install corsarr "$dep/helm/corsarr" -n corsarr-helm --wait --timeout 180s
  check "smoke helm test" helm test corsarr -n corsarr-helm --timeout 120s
  port_forward_check corsarr-helm corsarr "smoke helm"
}

s_kubernetes() {
  kind_cluster || return 0
  trap 'kind delete cluster --name corsarr-smoke >/dev/null 2>&1; rm -rf "$work"' EXIT
  kubectl apply -k "$dep/kubernetes/base"
  check "smoke kubernetes rollout" kubectl -n corsarr rollout status deploy/corsarr --timeout 180s
  port_forward_check corsarr corsarr "smoke kubernetes"
}

s_podman() {
  need podman podman systemctl || return 0
  # installs into the real user units and volume: CI runners only
  [ "${CI:-}" = true ] || { skipt podman "runs only with CI=true (writes ~/.config/containers/systemd)"; return 0; }
  local ref; ref="$(pinned_image)"
  if [ -n "${SMOKE_IMAGE:-}" ]; then docker save "$SMOKE_IMAGE" | podman load && podman tag "$SMOKE_IMAGE" "$ref"
  else retry podman pull -q "$ref" >/dev/null; fi
  local units="$HOME/.config/containers/systemd"
  mkdir -p "$units" "$HOME/.config/corsarr" "$HOME/media"
  cp "$dep/podman/corsarr.container" "$dep/podman/corsarr-data.volume" "$units/"
  cp "$dep/podman/corsarr.yml" "$HOME/.config/corsarr/"
  trap 'systemctl --user stop corsarr >/dev/null 2>&1; rm -f "$units/corsarr.container" "$units/corsarr-data.volume"; systemctl --user daemon-reload; podman volume rm -f corsarr-data >/dev/null 2>&1; rm -rf "$work"' EXIT
  systemctl --user daemon-reload
  systemctl --user start corsarr
  check "smoke podman" wait_http http://127.0.0.1:8090
}

s_packages() {
  need packages nfpm docker || return 0
  local bin; bin="$(corsarr_bin)"
  mkdir -p "$work/pkg"
  for p in deb rpm apk; do
    nfpm_build "$p" "$bin" "$work/pkg" >/dev/null
  done
  local probe='corsarr version && id corsarr && test -f /etc/corsarr/corsarr.yml && corsarr config validate --configFile=/etc/corsarr/corsarr.yml'
  check "smoke deb (debian)" docker run --rm -v "$work/pkg:/pkg:ro" debian:stable-slim sh -c "apt-get install -y -q /pkg/*.deb >/dev/null && $probe"
  check "smoke rpm (fedora)" docker run --rm -v "$work/pkg:/pkg:ro" fedora:latest sh -c "dnf install -y -q /pkg/*.rpm >/dev/null && $probe"
  check "smoke apk (alpine)" docker run --rm -v "$work/pkg:/pkg:ro" alpine:latest sh -c "apk add -q --allow-untrusted /pkg/*.apk && $probe"
}

# ── main ─────────────────────────────────────────────────────────────────────

summary() {
  echo
  echo "passed: ${#pass[@]}  skipped: ${#skip[@]}  failed: ${#fail[@]}"
  for s in "${skip[@]+"${skip[@]}"}"; do echo "  skipped: $s"; done
  for f in "${fail[@]+"${fail[@]}"}"; do echo "  failed:  $f"; done
  [ "${#fail[@]}" -eq 0 ] || exit 1
  if [ "${STRICT:-0}" = 1 ] && [ "${#skip[@]}" -gt 0 ]; then echo "STRICT=1: skips are failures"; exit 1; fi
}

action="${1:-validate}" target="${2:-all}"
case "$action" in
  validate)
    if [ "$target" = all ]; then set -- "${validate_targets[@]}"; else set -- "$target"; fi
    for t in "$@"; do
      [[ " ${validate_targets[*]} " == *" $t "* ]] || { echo "unknown target $t (all ${validate_targets[*]})"; exit 2; }
      "v_${t//-/_}"
    done ;;
  smoke)
    [[ " ${smoke_targets[*]} " == *" $target "* ]] || { echo "unknown smoke target $target (${smoke_targets[*]})"; exit 2; }
    need "$target" docker curl || { summary; exit 0; }
    "s_$target" ;;
  *) echo "unknown action $action (validate|smoke)"; exit 2 ;;
esac
summary
