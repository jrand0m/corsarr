#!/usr/bin/env bash
# Refuses raw-HTML sinks in the web UI source: every text the UI shows is bound as text, so
# `{@html}`, innerHTML/outerHTML, insertAdjacentHTML and DOMParser have no place in it.
# Usage: scripts/lint-ui-html.sh [dir]   (default: ui/src)
set -euo pipefail
dir="${1:-ui/src}"
pattern='\{@html|innerHTML|outerHTML|insertAdjacentHTML|DOMParser'
if hits="$(grep -rnE "$pattern" "$dir" --include='*.svelte' --include='*.ts' --include='*.js' --include='*.mjs' --include='*.html')"; then
  echo "$hits"
  echo "FAIL: raw HTML sink in $dir; bind values as text instead"
  exit 1
fi
echo "ok: no raw HTML sinks in $dir"
