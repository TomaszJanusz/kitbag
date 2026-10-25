#!/usr/bin/env bash
# Read-only structural and checksum validation of the release payload.
set -euo pipefail

cd "${1:?usage: release-verify.sh DIST}"
sha256sum --check checksums.txt
version=$(jq -er '.version | select(type == "string" and length > 0)' kitbag-update.json)
for os in darwin linux; do
  for arch in amd64 arm64; do
    archive="kitbag_${version}_${os}_${arch}.tar.gz"
    test -f "$archive"
    test "$(tar -tzf "$archive" | LC_ALL=C sort)" = "$(printf '%s\n' LICENSE README.md kitbag kitbag.install-source | LC_ALL=C sort)"
    test "$(tar -xOzf "$archive" kitbag.install-source)" = release-archive
    jq -e '.spdxVersion and (.packages | length > 0)' "$archive.sbom.json" >/dev/null
  done
done
