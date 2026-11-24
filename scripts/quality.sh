#!/usr/bin/env bash
# Quality gate: coverage × complexity (CRAP) per function for Go packages.
# CRAP(f) = comp(f)^2 * (1 - cov(f))^3 + comp(f); fail when any function >= 10.
set -euo pipefail
# Use the pinned toolchain through mise when it is installed, plain go otherwise (CI).
run() { if command -v mise >/dev/null 2>&1; then mise exec -- "$@"; else "$@"; fi; }
run go test -short -coverprofile=coverage.out ./cmd/... ./internal/... >/dev/null
run go run ./scripts/crap -profile coverage.out -max 10
