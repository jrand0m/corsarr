#!/usr/bin/env bash
# Quality gate: coverage × complexity (CRAP) per function for Go packages.
# CRAP(f) = comp(f)^2 * (1 - cov(f))^3 + comp(f); fail when any function >= 10.
set -euo pipefail
mise exec -- go test -short -coverprofile=coverage.out ./cmd/... ./internal/... >/dev/null
mise exec -- go run ./scripts/crap -profile coverage.out -max 10
