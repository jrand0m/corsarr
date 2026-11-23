#!/usr/bin/env bash
# Footprint budget gate: binary size and idle memory. Usage: scripts/budget.sh [binary]
set -euo pipefail
bin="${1:-$(ls -t bin/corsarr-* 2>/dev/null | head -1)}"
max_bin_mib=30
size=$(wc -c < "$bin")
mib=$(( size / 1048576 ))
echo "binary $bin: ${mib} MiB (budget ${max_bin_mib} MiB)"
[ "$mib" -le "$max_bin_mib" ] || { echo "FAIL: binary over budget"; exit 1; }
# Idle RSS check (linux only; skipped elsewhere).
if [ "$(uname -s)" = Linux ] && [ -x "$bin" ]; then
  d=$(mktemp -d); "$bin" --data.dir="$d" --server.address=127.0.0.1:0 & pid=$!
  sleep 3; rss_kb=$(awk '/VmRSS/{print $2}' /proc/$pid/status); kill $pid; wait $pid 2>/dev/null || true
  echo "idle RSS: $((rss_kb/1024)) MiB (budget 40 MiB)"
  [ "$rss_kb" -le $((40*1024)) ] || { echo "FAIL: idle RSS over budget"; exit 1; }
fi
