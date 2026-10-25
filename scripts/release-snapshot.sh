#!/usr/bin/env bash
# Exercise the real packaging path and compare binaries from independent builds.
set -euo pipefail

goreleaser check
first=$(mktemp /tmp/kitbag-release-hashes.XXXXXX)
trap 'rm -f "$first"' EXIT
# Reproducibility covers the four executables; SBOM document timestamps and
# snapshot metadata intentionally vary. Keep paths relative to this checkout.
# Build the executables alone first so the complete payload stays in dist for
# package checks and CI artifacts.
goreleaser build --snapshot --clean --parallelism 2
find dist -type f -name kitbag -print0 | sort -z | xargs -0 sha256sum >"$first"
test "$(wc -l <"$first")" -eq 4
bash scripts/release-build.sh snapshot
sha256sum --check "$first"
./dist/kitbag_linux_amd64_v1/kitbag version --bare | grep -Fx "kitbag $(jq -r .version dist/metadata.json)"
