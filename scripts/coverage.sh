#!/usr/bin/env bash
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

scratch=$(mktemp -d /tmp/kitbag-coverage.XXXXXX)
trap 'rm -rf "$scratch"' EXIT
# Include packages without tests so new, untested core code cannot escape the gate.
# Keep core independent: CLI/integration tests must not inflate its existing floor.
go test -count=1 -covermode=atomic -coverpkg=./internal/core/... \
  -coverprofile="$scratch/core.out" ./internal/core/...

# testscript.Main runs the instrumented test binary as the CLI and forwards
# GOCOVERDIR, so go test includes subprocess coverage without a second build.
# Test-only internal/e2e, internal/schemacheck and internal/plancheck are excluded.
go test -count=1 -covermode=atomic \
  -coverpkg=./internal/adapters/...,./internal/vcs/...,./cmd/kitbag,./pkg/manifest \
  -coverprofile="$scratch/groups.out" \
  ./internal/adapters/... ./internal/vcs/... ./cmd/kitbag ./pkg/manifest

LC_ALL=C awk -f scripts/coverage-report.awk "$scratch/core.out" "$scratch/groups.out"
# Retain the existing core tool as a cross-check of the statement aggregation.
go-test-coverage --profile "$scratch/core.out" --threshold-total 85
