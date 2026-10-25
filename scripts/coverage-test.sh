#!/usr/bin/env bash
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
scratch=$(mktemp -d /tmp/kitbag-coverage-test.XXXXXX)
trap 'rm -rf "$scratch"' EXIT

# Unequal block sizes catch averaging percentages; duplicate blocks catch
# -coverpkg denominator inflation and lost hits from other test binaries.
cat >"$scratch/pass.out" <<'PROFILE'
mode: atomic
github.com/TomaszJanusz/kitbag/internal/core/example/a.go:1.1,2.1 85 1
github.com/TomaszJanusz/kitbag/internal/core/example/a.go:3.1,4.1 15 0
github.com/TomaszJanusz/kitbag/internal/adapters/example/a.go:1.1,2.1 90 0
github.com/TomaszJanusz/kitbag/internal/adapters/example/a.go:3.1,4.1 10 0
github.com/TomaszJanusz/kitbag/internal/adapters/example/a.go:1.1,2.1 90 1
github.com/TomaszJanusz/kitbag/internal/vcs/example/a.go:1.1,2.1 80 1
github.com/TomaszJanusz/kitbag/internal/vcs/example/a.go:3.1,4.1 20 0
github.com/TomaszJanusz/kitbag/cmd/kitbag/a.go:1.1,2.1 78 1
github.com/TomaszJanusz/kitbag/cmd/kitbag/a.go:3.1,4.1 22 0
github.com/TomaszJanusz/kitbag/pkg/manifest/a.go:1.1,2.1 88 1
github.com/TomaszJanusz/kitbag/pkg/manifest/a.go:3.1,4.1 12 0
PROFILE

report() {
  LC_ALL=C awk -f scripts/coverage-report.awk "$1" >"$scratch/report"
}

report "$scratch/pass.out"
grep -Eq 'PASS group adapters +90.00% .*90/100 statements' "$scratch/report"
grep -Eq 'internal/adapters/example +90.00% +90/100' "$scratch/report"

# Every group independently rejects a loss of hits, including untested code.
for group in internal/core internal/adapters internal/vcs cmd/kitbag pkg/manifest; do
  awk -v group="/$group/" 'index($1, group) { $3 = 0 } { print }' \
    "$scratch/pass.out" >"$scratch/fail.out"
  if report "$scratch/fail.out"; then
    printf 'Expected coverage failure for %s\n' "$group" >&2
    exit 1
  fi
  grep -Eq "FAIL group ${group#internal/} +0.00%" "$scratch/report"
done

# Compare unrounded values: 84.999% must fail even though it displays as 85%.
sed 's/85 1/84999 1/; s/15 0/15001 0/' "$scratch/pass.out" >"$scratch/round.out"
if report "$scratch/round.out"; then
  printf '%s\n' 'Expected failure below the unrounded core floor' >&2
  exit 1
fi
grep -Eq 'FAIL group core +85.00%' "$scratch/report"

# A missing group cannot silently disable a gate.
sed '/\/pkg\/manifest\//d' "$scratch/pass.out" >"$scratch/missing.out"
if report "$scratch/missing.out"; then
  printf '%s\n' 'Expected failure for missing manifest coverage' >&2
  exit 1
fi
grep -Eq 'FAIL group pkg/manifest: no executable statements' "$scratch/report"
printf '%s\n' 'Coverage report controls passed.'
