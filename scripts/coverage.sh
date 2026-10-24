#!/usr/bin/env bash
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

scratch=$(mktemp -d /tmp/kitbag-coverage.XXXXXX)
trap 'rm -rf "$scratch"' EXIT
profile="$scratch/core.out"

# Include packages without tests so new, untested core code cannot escape the gate.
go test -count=1 -covermode=atomic -coverpkg=./internal/core/... \
  -coverprofile="$profile" ./internal/core/...

threshold=85
if ! awk 'NR > 1 { statements += $2 } END { exit !(statements > 0) }' "$profile"; then
  printf '%s\n' 'Core has no executable statements; coverage reports only (future threshold: 85%).'
  threshold=0
fi

go-test-coverage --profile "$profile" --threshold-total "$threshold"
