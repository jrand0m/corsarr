#!/usr/bin/env bash
# Moves every deployment sample in deploy/ to a release: image tags, chart
# version and appVersion, kustomize newTag, download URLs, package names,
# installer and app-store versions, and the checksums Homebrew, Scoop, winget,
# the AUR PKGBUILD, the Nix flake and the Ansible role carry (a checksum is
# matched to the archive named on one of the three lines before it).
#
#   scripts/pins.sh <vX.Y.Z> [SHA256SUMS]
#
# Without a SHA256SUMS file it is downloaded from the GitHub release. The
# current version is read from the Helm chart. Review the result with git diff.
set -euo pipefail

new=${1:-}
[[ "$new" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$ ]] || { echo "usage: pins.sh <vX.Y.Z> [SHA256SUMS]" >&2; exit 2; }
top="$(cd "$(dirname "$0")/.." && pwd)"
cd "$top"
repo="${REPO:-jrand0m/corsarr}"

sums=${2:-}
if [ -z "$sums" ]; then
  sums=$(mktemp)
  trap 'rm -f "$sums"' EXIT
  for i in 1 2 3 4 5; do
    curl -fsSL -o "$sums" "https://github.com/$repo/releases/download/$new/SHA256SUMS" && break
    [ "$i" = 5 ] && { echo "could not download SHA256SUMS for $new" >&2; exit 1; }
    sleep $((i * 5))
  done
fi

old=$(sed -n 's/^appVersion: *"\{0,1\}\(v[^"]*\)"\{0,1\}$/\1/p' deploy/helm/corsarr/Chart.yaml)
[ -n "$old" ] || { echo "no appVersion in deploy/helm/corsarr/Chart.yaml" >&2; exit 1; }
echo "deploy samples: $old -> $new"

# generated defaults and placeholder test values are not pins
files=$(grep -rlIE "${old#v}|[0-9a-fA-F]{64}" deploy | grep -vE '^deploy/corsarr\.example\.yml$|/ci/' || true)

sha() { if command -v sha256sum >/dev/null; then sha256sum "$1"; else shasum -a 256 "$1"; fi | cut -d' ' -f1; }
local_sums=""
for f in corsarr.service corsarr.sysusers corsarr.yml; do local_sums+="$(sha "deploy/systemd/$f") "; done

for f in $files; do
  OLD="${old#v}" NEW="${new#v}" SUMS="$sums" LOCAL="$local_sums" perl -0pi -e '
    BEGIN {
      open my $fh, "<", $ENV{SUMS} or die "$ENV{SUMS}: $!";
      local $/ = "\n";
      while (<$fh>) { my ($h, $n) = split; $n =~ s/^\*//; $sum{$n} = $h if $n; }
      @local = split " ", $ENV{LOCAL};
    }
    my ($o, $n) = (quotemeta $ENV{OLD}, $ENV{NEW});
    # version, with or without the leading v, never inside a longer version
    s/(?<![\d.])(v?)$o(?!\d|\.\d)/$1$n/g;
    # a checksum within three lines after the URL of the file it belongs to
    for my $name (keys %sum) {
      my $h = $sum{$name};
      s{(/\Q$name\E\b[^\n]*\n(?:[^\n]*\n){0,2}?[^\n]*?)\b([0-9a-fA-F]{64})\b}{
        my ($pre, $was) = ($1, $2);
        $pre . ($was =~ /[A-F]/ ? uc $h : $h)
      }ge;
    }
    # AUR PKGBUILD: per-architecture archives and the files published beside it
    s/(sha256sums_x86_64=\(\x27)[0-9a-f]{64}/$1$sum{"corsarr-linux-amd64.tar.gz"}/ if $sum{"corsarr-linux-amd64.tar.gz"};
    s/(sha256sums_aarch64=\(\x27)[0-9a-f]{64}/$1$sum{"corsarr-linux-arm64.tar.gz"}/ if $sum{"corsarr-linux-arm64.tar.gz"};
    if (/^pkgname=/m) {
      my $i = 0;
      s{(sha256sums=\(\x27[0-9a-f]{64}\x27(?:\s+\x27[0-9a-f]{64}\x27){0,2})}{
        my $block = $1; $block =~ s/[0-9a-f]{64}/$local[$i++]/ge; $block
      }e;
    }
  ' "$f"
done

git -c color.ui=never diff --stat -- deploy
