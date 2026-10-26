#!/usr/bin/env bash
# Install KitBag through one distribution channel on a throwaway host (CI
# container or runner) and prove the installed command and its install-source
# stamp. Never run this on a workstation: it installs system packages.
# Use only Bash 3 features so this also runs on macOS.
set -euo pipefail

usage() {
  echo 'usage: package-check.sh deb|rpm|aur|homebrew|install-script DIR' >&2
  echo '       package-check.sh homebrew-tap VERSION' >&2
  echo '       package-check.sh download DIR TAG PATTERN' >&2
  exit 2
}
[[ $# -ge 2 ]] || usage
channel=$1
root=$(cd "$(dirname "$0")/.." && pwd)
repository=TomaszJanusz/kitbag

metadata() {
  tr -d ' \t\r\n' <"$1/kitbag-update.json" | sed -n "s/.*\"$2\":\"\\([^\"]*\\)\".*/\\1/p"
}

# expect_command PATH VERSION GUIDANCE: run the installed command and check the
# owner-specific self-update guidance, which proves where the stamp was read.
expect_command() {
  local command=$1 version=$2 guidance=$3 reported advice
  reported=$("$command" version --bare)
  [[ "$reported" == "kitbag $version" ]] || {
    echo "package-check: $command reports '$reported', expected 'kitbag $version'" >&2
    exit 1
  }
  "$command" version
  if [[ -n "$guidance" ]]; then
    advice=$("$command" self-update)
    [[ "$advice" == "$guidance" ]] || {
      echo "package-check: self-update printed '$advice', expected '$guidance'" >&2
      exit 1
    }
  fi
  echo "package-check: $channel installed kitbag $version at $command"
}

case $channel in
deb)
  # apt-get reads a relative "dir/file.deb" as package/release; use a path.
  dir=$(cd "$2" && pwd)
  version=$(metadata "$dir" version)
  package=$(echo "$dir"/kitbag_*_"$(dpkg --print-architecture)".deb)
  dpkg-deb --info "$package"
  dpkg-deb --contents "$package"
  [[ "$(dpkg-deb --field "$package" Recommends)" == git ]]
  # A local package needs no repositories; skip the recommended Git download.
  DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends "$package"
  [[ "$(readlink /usr/bin/kitbag)" == /usr/lib/kitbag/kitbag ]]
  [[ "$(cat /usr/lib/kitbag/kitbag.install-source)" == deb ]]
  dpkg -S /usr/lib/kitbag/kitbag.install-source
  expect_command kitbag "$version" 'sudo apt-get update && sudo apt-get install --only-upgrade kitbag'
  apt-get remove -y kitbag
  [[ ! -e /usr/lib/kitbag/kitbag.install-source && ! -e /usr/bin/kitbag ]]
  ;;
rpm)
  dir=$(cd "$2" && pwd)
  version=$(metadata "$dir" version)
  package=$(echo "$dir"/kitbag-*."$(uname -m)".rpm)
  rpm -qpi "$package"
  rpm -qplv "$package"
  rpm -qp --recommends "$package" | grep -Fx git
  dnf install -y --disablerepo='*' --setopt=install_weak_deps=False "$package"
  [[ "$(readlink /usr/bin/kitbag)" == /usr/lib/kitbag/kitbag ]]
  [[ "$(cat /usr/lib/kitbag/kitbag.install-source)" == rpm ]]
  rpm -qf /usr/lib/kitbag/kitbag.install-source
  expect_command kitbag "$version" 'sudo dnf upgrade kitbag'
  dnf remove -y kitbag
  [[ ! -e /usr/lib/kitbag/kitbag.install-source && ! -e /usr/bin/kitbag ]]
  ;;
aur)
  # Build GoReleaser's PKGBUILD as an unprivileged user, with its release URLs
  # redirected to the local payload; makepkg still verifies sha256sums.
  dir=$(cd "$2" && pwd)
  version=$(metadata "$dir" version)
  id builder >/dev/null 2>&1 || useradd --create-home builder
  build=$(mktemp -d /tmp/kitbag-aur.XXXXXX)
  sed "s|https://github.com/$repository/releases/download/[^/]*/|file://$dir/|" \
    "$dir/aur/kitbag-bin.pkgbuild" >"$build/PKGBUILD"
  grep -F "file://$dir/" "$build/PKGBUILD"
  chown -R builder "$build"
  (cd "$build" && runuser -u builder -- makepkg --noconfirm)
  pacman -U --noconfirm "$build"/kitbag-bin-*.pkg.tar.zst
  [[ "$(readlink /usr/bin/kitbag)" == /usr/lib/kitbag/kitbag ]]
  [[ "$(cat /usr/lib/kitbag/kitbag.install-source)" == aur ]]
  pacman -Qo /usr/lib/kitbag/kitbag.install-source
  expect_command kitbag "$version" 'update kitbag with your AUR helper (for example: yay -Syu kitbag)'
  pacman -R --noconfirm kitbag-bin
  [[ ! -e /usr/lib/kitbag/kitbag.install-source && ! -e /usr/bin/kitbag ]]
  rm -rf "$build"
  ;;
homebrew)
  # A local tap with the rendered formula pointing at the local payload.
  dir=$(cd "$2" && pwd)
  version=$(metadata "$dir" version)
  export HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_INSTALL_CLEANUP=1 HOMEBREW_NO_ANALYTICS=1
  brew tap-new --no-git kitbag-ci/local
  tap=$(brew --repository kitbag-ci/local)
  bash "$root/scripts/homebrew-formula.sh" "$dir" "file://$dir" >"$tap/Formula/kitbag.rb"
  # Caveats print on every install and upgrade; the runner has Git, so none.
  out=$(brew install --formula kitbag-ci/local/kitbag 2>&1)
  printf '%s\n' "$out"
  if grep -Eq 'Caveats|kitbag init|service install' <<<"$out"; then
    echo "package-check: unexpected Homebrew caveats" >&2
    exit 1
  fi
  brew test kitbag-ci/local/kitbag
  prefix=$(brew --prefix)
  [[ -L "$prefix/bin/kitbag" ]]
  [[ "$(cat "$(brew --prefix kitbag-ci/local/kitbag)/libexec/kitbag.install-source")" == homebrew ]]
  expect_command "$prefix/bin/kitbag" "$version" 'brew upgrade kitbag'
  brew uninstall --formula kitbag-ci/local/kitbag
  brew untap kitbag-ci/local
  ;;
homebrew-tap)
  version=${2#v}
  export HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_INSTALL_CLEANUP=1 HOMEBREW_NO_ANALYTICS=1
  brew install --formula TomaszJanusz/tap/kitbag
  brew test TomaszJanusz/tap/kitbag
  expect_command "$(brew --prefix)/bin/kitbag" "$version" 'brew upgrade kitbag'
  ;;
install-script)
  # Without KITBAG_RELEASE_BASE_URL, serve the local payload (a snapshot is a
  # nightly) from file:// URLs. Stable releases are installed by exact version.
  dir=$(cd "$2" && pwd)
  version=$(metadata "$dir" version)
  home=$(mktemp -d /tmp/kitbag-install-home.XXXXXX)
  if [[ -z "${KITBAG_RELEASE_BASE_URL:-}" ]]; then
    site=$home/site
    mkdir -p "$site/download"
    ln -s "$dir" "$site/download/nightly"
    export KITBAG_RELEASE_BASE_URL="file://$site"
  fi
  if [[ "$(metadata "$dir" channel)" == nightly ]]; then
    args=(--channel nightly)
  else
    args=(--version "$version")
  fi
  HOME=$home sh "$root/install.sh" "${args[@]}" | tee "$home/output"
  if [[ -n "${KITBAG_REQUIRE_SIGNATURE:-}" ]]; then
    grep -F 'verified the keyless signature' "$home/output"
  fi
  [[ "$(cat "$home/.local/bin/kitbag.install-source")" == install-script ]]
  expect_command "$home/.local/bin/kitbag" "$version" ''
  # A second run upgrades its own installation in place.
  HOME=$home sh "$root/install.sh" "${args[@]}" | grep -F 'upgrading the existing kitbag'
  rm -rf "$home"
  ;;
download)
  # Fetch published assets whose names match the extended regular expression
  # PATTERN, plus discovery metadata, and check them against the checksums.
  [[ $# -eq 4 ]] || usage
  dir=$2 tag=$3 pattern=$4
  base=https://github.com/$repository/releases/download/$tag
  mkdir -p "$dir"
  fetch() {
    curl --fail --silent --show-error --location --proto '=https' --retry 2 \
      --output "$dir/$1" "$base/$1"
  }
  fetch checksums.txt
  files=$(awk '{ print $2 }' "$dir/checksums.txt" | grep -E -- "$pattern")
  for file in kitbag-update.json $files; do
    fetch "$file"
  done
  (cd "$dir" && sha256sum --check --ignore-missing checksums.txt)
  ;;
*) usage ;;
esac
