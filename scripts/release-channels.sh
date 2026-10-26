#!/usr/bin/env bash
# Validate (verify) or publish (publish) the Homebrew formula and AUR package
# for a verified release payload. Publishing runs only in the trusted release
# workflow after the GitHub release exists; each channel is skipped, not
# failed, while its maintainer-provided secret is absent.
set -euo pipefail

mode=${1:?usage: release-channels.sh verify|publish DIST}
dist=$(cd "${2:?usage: release-channels.sh verify|publish DIST}" && pwd)
root=$(cd "$(dirname "$0")/.." && pwd)
version=$(jq -er '.version | select(type == "string" and length > 0)' "$dist/kitbag-update.json")
tap=TomaszJanusz/homebrew-tap
# Matches the Ed25519 fingerprint published at https://aur.archlinux.org.
aur_host_key='aur.archlinux.org ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIEuBKrPzbawxA/k2g6NcyV5jmqwJ2s+zpgZGZ7tpLIcN'

checksum() {
  awk -v name="$1" '$2 == name { print $1 }' "$dist/checksums.txt"
}

verify() {
  local pkgbuild=$dist/aur/kitbag-bin.pkgbuild srcinfo=$dist/aur/kitbag-bin.srcinfo
  local pkgver sources arch goarch archive hash
  test -f "$pkgbuild"
  test -f "$srcinfo"
  # Stable PKGBUILDs spell the version in source URLs as ${pkgver}.
  pkgver=$(sed -n 's/^pkgver=//p' "$pkgbuild")
  sources=$(grep '^source_' "$pkgbuild" | sed "s/\${pkgver}/$pkgver/g")
  for arch in x86_64 aarch64; do
    case $arch in
    x86_64) goarch=amd64 ;;
    *) goarch=arm64 ;;
    esac
    archive="kitbag_${version}_linux_${goarch}.tar.gz"
    hash=$(checksum "$archive")
    [[ "$hash" =~ ^[0-9a-f]{64}$ ]]
    grep -Fqx "sha256sums_$arch=('$hash')" "$pkgbuild"
    grep -Fq "/$archive\")" <<<"$sources"
    grep -Fqx "	sha256sums_$arch = $hash" "$srcinfo"
  done
  # Package-owned stamp beside the real executable, never the archive's stamp.
  grep -Fqx "  printf 'aur\\n' >kitbag.install-source" "$pkgbuild"
  # shellcheck disable=SC2016 # literal PKGBUILD lines
  {
    grep -Fqx '  install -Dm644 kitbag.install-source "${pkgdir}/usr/lib/kitbag/kitbag.install-source"' "$pkgbuild"
    grep -Fqx '  ln -s /usr/lib/kitbag/kitbag "${pkgdir}/usr/bin/kitbag"' "$pkgbuild"
  }
  bash "$root/scripts/homebrew-formula.sh" "$dist" >"$dist/kitbag.rb.check"
  grep -Fq '(libexec/"kitbag.install-source").write "homebrew\n"' "$dist/kitbag.rb.check"
  # Caveats print on every install and upgrade: only a missing Git qualifies,
  # never first-run steps (kitbag prints those itself).
  grep -Fq '    return if which("git")' "$dist/kitbag.rb.check"
  if grep -Eq 'kitbag (init|service)' "$dist/kitbag.rb.check"; then
    echo "release-channels: formula caveats carry first-run instructions" >&2
    return 1
  fi
  if command -v ruby >/dev/null 2>&1; then
    ruby -c "$dist/kitbag.rb.check" >/dev/null
  fi
  rm "$dist/kitbag.rb.check"
}

summary() {
  echo "$1"
  if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
    echo "- $1" >>"$GITHUB_STEP_SUMMARY"
  fi
}

output() {
  if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
    echo "$1" >>"$GITHUB_OUTPUT"
  fi
}

# commit_push DIR REF MESSAGE: commit changes, if any, and push them with the
# git command in the git_cmd array.
commit_push() {
  local dir=$1 ref=$2 message=$3
  "${git_cmd[@]}" -C "$dir" add -A
  if "${git_cmd[@]}" -C "$dir" diff --cached --quiet; then
    return 1
  fi
  "${git_cmd[@]}" -C "$dir" -c user.name='github-actions[bot]' \
    -c user.email='41898282+github-actions[bot]@users.noreply.github.com' commit -q -m "$message"
  "${git_cmd[@]}" -C "$dir" push -q origin "HEAD:$ref"
}

publish_homebrew() {
  local work auth
  if [[ -z "${HOMEBREW_TAP_TOKEN:-}" ]]; then
    summary "Homebrew: skipped; the HOMEBREW_TAP_TOKEN secret is not configured."
    output homebrew=skipped
    return
  fi
  work=$(mktemp -d /tmp/kitbag-tap.XXXXXX)
  # Keep the token out of the remote URL and the clone's configuration.
  auth="AUTHORIZATION: basic $(printf 'x-access-token:%s' "$HOMEBREW_TAP_TOKEN" | base64 | tr -d '\n')"
  git_cmd=(git -c "http.https://github.com/.extraheader=$auth")
  "${git_cmd[@]}" clone -q --depth 1 "https://github.com/$tap.git" "$work"
  mkdir -p "$work/Formula"
  bash "$root/scripts/homebrew-formula.sh" "$dist" >"$work/Formula/kitbag.rb"
  if commit_push "$work" "$(git -C "$work" symbolic-ref --short HEAD)" "kitbag $version"; then
    summary "Homebrew: published kitbag $version to $tap."
  else
    summary "Homebrew: $tap already has kitbag $version."
  fi
  output homebrew=published
  rm -rf "$work"
}

publish_aur() {
  local work ssh
  if [[ -z "${AUR_SSH_PRIVATE_KEY:-}" ]]; then
    summary "AUR: skipped; the AUR_SSH_PRIVATE_KEY secret is not configured."
    output aur=skipped
    return
  fi
  ssh=$(mktemp -d /tmp/kitbag-aur-ssh.XXXXXX)
  work=$(mktemp -d /tmp/kitbag-aur.XXXXXX)
  (umask 077 && printf '%s\n' "$AUR_SSH_PRIVATE_KEY" >"$ssh/key")
  printf '%s\n' "$aur_host_key" >"$ssh/known_hosts"
  export GIT_SSH_COMMAND="ssh -i $ssh/key -o IdentitiesOnly=yes -o UserKnownHostsFile=$ssh/known_hosts -o StrictHostKeyChecking=yes"
  git_cmd=(git)
  git clone -q ssh://aur@aur.archlinux.org/kitbag-bin.git "$work"
  cp "$dist/aur/kitbag-bin.pkgbuild" "$work/PKGBUILD"
  cp "$dist/aur/kitbag-bin.srcinfo" "$work/.SRCINFO"
  if commit_push "$work" master "Update to $version"; then
    summary "AUR: published kitbag-bin $version."
  else
    summary "AUR: kitbag-bin already has $version."
  fi
  output aur=published
  rm -rf "$work" "$ssh"
}

case $mode in
verify) verify ;;
publish)
  verify
  [[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || {
    echo "release-channels: only stable releases are published to channels, not $version" >&2
    exit 1
  }
  publish_homebrew
  publish_aur
  ;;
*) exit 2 ;;
esac
