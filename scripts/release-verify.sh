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

# Linux packages: Debian and RPM spell prereleases with "~".
package_version=${version/-/\~}
packages=(
  "kitbag_${package_version}_amd64.deb" "kitbag_${package_version}_arm64.deb"
  "kitbag-${package_version}-1.x86_64.rpm" "kitbag-${package_version}-1.aarch64.rpm"
)
for file in "${packages[@]}"; do
  test -f "$file"
done
test "$(find . -maxdepth 1 \( -name '*.deb' -o -name '*.rpm' \) | wc -l | tr -d ' ')" = "${#packages[@]}"
cmp install.sh "$(git rev-parse --show-toplevel)/install.sh"
# Every published file must be covered by the signed manifest.
for file in ./*.tar.gz ./*.sbom.json ./*.deb ./*.rpm install.sh kitbag-update.json; do
  awk -v name="${file#./}" '$2 == name { found = 1 } END { exit !found }' checksums.txt
done

# Inspect package metadata and the package-owned stamp where dpkg-deb exists
# (Ubuntu CI). RPM contents are installed and checked in a Fedora container.
if command -v dpkg-deb >/dev/null 2>&1; then
  for arch in amd64 arm64; do
    deb="kitbag_${package_version}_${arch}.deb"
    test "$(dpkg-deb --field "$deb" Package Version Architecture Recommends)" = "$(printf 'Package: kitbag\nVersion: %s\nArchitecture: %s\nRecommends: git' "$package_version" "$arch")"
    contents=$(dpkg-deb --contents "$deb")
    grep -E '^l.* \./usr/bin/kitbag -> /usr/lib/kitbag/kitbag$' <<<"$contents" >/dev/null
    grep -E '^-rwxr-xr-x .* \./usr/lib/kitbag/kitbag$' <<<"$contents" >/dev/null
    test "$(dpkg-deb --fsys-tarfile "$deb" | tar -xOf - ./usr/lib/kitbag/kitbag.install-source)" = deb
  done
else
  echo 'release-verify: dpkg-deb not found; skipped Debian package inspection' >&2
fi
