#!/usr/bin/env bash
# Build locally or in CI. This script never signs, tags or publishes.
set -euo pipefail

channel=${1:?usage: release-build.sh stable|nightly|snapshot}
case "$channel" in
stable)
  goreleaser release --clean --parallelism 2 --skip=publish,sign
  ;;
nightly)
  base=$(jq -r '.["."]' .release-please-manifest.json)
  IFS=. read -r major minor patch <<<"$base"
  base="$major.$minor.$((patch + 1))"
  export KITBAG_SNAPSHOT_VERSION="${base}-nightly.$(date -u +%Y%m%d%H%M%S).${GITHUB_RUN_NUMBER:?}.${GITHUB_RUN_ATTEMPT:?}"
  goreleaser release --snapshot --clean --parallelism 2
  ;;
snapshot)
  goreleaser release --snapshot --clean --parallelism 2
  channel=nightly
  ;;
*) exit 2 ;;
esac

version=$(jq -r .version dist/metadata.json)
jq -n --arg version "$version" --arg channel "$channel" \
  '{version: $version, channel: $channel}' >dist/kitbag-update.json
# GoReleaser hashed install.sh (checksum.extra_files) but publishes nothing;
# ship the exact signed copy beside the archives.
cp install.sh dist/install.sh
# Include discovery metadata in the authenticated manifest, along with every
# archive, package and SBOM. GoReleaser has already hashed its artifacts.
(cd dist && sha256sum kitbag-update.json >>checksums.txt && sha256sum --check checksums.txt)
bash scripts/release-verify.sh dist
bash scripts/release-channels.sh verify dist
