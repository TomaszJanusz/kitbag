#!/usr/bin/env bash
# Exercise the real packaging path and compare binaries from independent builds.
set -euo pipefail

goreleaser check
bash scripts/release-build.sh snapshot
first=$(mktemp /tmp/kitbag-release-hashes.XXXXXX)
trap 'rm -f "$first"' EXIT
# Reproducibility covers the four executables; SBOM document timestamps and
# snapshot metadata intentionally vary. Keep paths relative to this checkout.
find dist -type f -name kitbag -print0 | sort -z | xargs -0 sha256sum >"$first"
test "$(wc -l <"$first")" -eq 4
./dist/kitbag_linux_amd64_v1/kitbag version --bare | grep -Fx "kitbag $(jq -r .version dist/metadata.json)"
goreleaser build --snapshot --clean --parallelism 2
sha256sum --check "$first"
