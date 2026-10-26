#!/bin/sh
# Install KitBag from GitHub Releases without sudo. See docs/install.md.
#
#   curl -fsSL https://github.com/TomaszJanusz/kitbag/releases/latest/download/install.sh | sh
#
# Options: --prefix DIR (default ~/.local/bin), --version X.Y.Z, --channel
# stable|nightly, --force (replace an install this script did not create).
# Environment: KITBAG_VERSION, KITBAG_RELEASE_BASE_URL (mirror or local test
# server), KITBAG_COSIGN (cosign command; verification is skipped only when it
# cannot be found).
set -eu

repository=TomaszJanusz/kitbag
issuer=https://token.actions.githubusercontent.com
workflow="https://github.com/$repository/.github/workflows/release.yml"

say() { printf 'kitbag-install: %s\n' "$*"; }
fail() {
  printf 'kitbag-install: error: %s\n' "$*" >&2
  exit 1
}

usage() {
  cat <<'EOF'
Usage: install.sh [--prefix DIR] [--version X.Y.Z] [--channel stable|nightly] [--force]

Downloads a KitBag release archive, verifies its SHA-256 checksum (and the
keyless cosign signature of checksums.txt when cosign is installed), and
installs the kitbag executable into DIR (default: ~/.local/bin) without sudo.
EOF
}

prefix=${HOME:+$HOME/.local/bin}
version=${KITBAG_VERSION:-}
channel=stable
force=false
while [ $# -gt 0 ]; do
  case $1 in
  --prefix)
    [ $# -ge 2 ] || fail "--prefix needs a directory"
    prefix=$2
    shift 2
    ;;
  --prefix=*)
    prefix=${1#--prefix=}
    shift
    ;;
  --version)
    [ $# -ge 2 ] || fail "--version needs a value"
    version=$2
    shift 2
    ;;
  --version=*)
    version=${1#--version=}
    shift
    ;;
  --channel)
    [ $# -ge 2 ] || fail "--channel needs a value"
    channel=$2
    shift 2
    ;;
  --channel=*)
    channel=${1#--channel=}
    shift
    ;;
  --force)
    force=true
    shift
    ;;
  -h | --help)
    usage
    exit 0
    ;;
  *) fail "unknown option: $1 (see --help)" ;;
  esac
done

[ -n "$prefix" ] || fail "HOME is not set; pass --prefix"
case $prefix in
/*) ;;
*) prefix=$(pwd)/$prefix ;;
esac
case $channel in
stable | nightly) ;;
*) fail "channel must be stable or nightly" ;;
esac
version=${version#v}
if [ -n "$version" ] && [ "$channel" = nightly ]; then
  fail "--version and --channel nightly are mutually exclusive"
fi

# Map the host to a published archive. Rosetta reports x86_64 on Apple
# silicon; prefer the native build there.
case $(uname -s) in
Darwin) os=darwin ;;
Linux) os=linux ;;
*) fail "unsupported operating system: $(uname -s) (macOS and Linux only)" ;;
esac
case $(uname -m) in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*) fail "unsupported architecture: $(uname -m) (amd64 and arm64 only)" ;;
esac
if [ "$os$arch" = darwinamd64 ] && [ "$(sysctl -n sysctl.proc_translated 2>/dev/null || :)" = 1 ]; then
  arch=arm64
fi

base=${KITBAG_RELEASE_BASE_URL:-https://github.com/$repository/releases}
base=${base%/}
# Plain HTTP is accepted only from this machine, like kitbag self-update.
case $base in
https://* | file:///*) ;;
http://127.0.0.1 | http://127.0.0.1[:/]* | http://localhost | http://localhost[:/]* | http://\[::1\]*) ;;
*) fail "release base URL must use https (plain http only for loopback): $base" ;;
esac

command -v curl >/dev/null 2>&1 || fail "curl is required"
fetch() {
  curl --fail --silent --show-error --location --proto '=https,http,file' \
    --proto-redir '=https' --retry 2 --output "$2" "$1" ||
    fail "download failed: $1"
}

sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d ' ' -f 1
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | cut -d ' ' -f 1
  else
    fail "sha256sum or shasum is required to verify the download"
  fi
}

tmp=$(mktemp -d "${TMPDIR:-/tmp}/kitbag-install.XXXXXX") || fail "cannot create a temporary directory"
staged_binary=
staged_stamp=
trap 'rm -rf "$tmp"; [ -z "$staged_binary" ] || rm -f "$staged_binary" "$staged_stamp"' EXIT
trap 'exit 1' HUP INT TERM

# Resolve the version through the same discovery metadata as self-update.
if [ -z "$version" ]; then
  if [ "$channel" = nightly ]; then
    metadata=$base/download/nightly/kitbag-update.json
  else
    metadata=$base/latest/download/kitbag-update.json
  fi
  fetch "$metadata" "$tmp/kitbag-update.json"
  compact=$(tr -d ' \t\r\n' <"$tmp/kitbag-update.json")
  version=$(printf '%s\n' "$compact" | sed -n 's/.*"version":"\([^"]*\)".*/\1/p')
  advertised=$(printf '%s\n' "$compact" | sed -n 's/.*"channel":"\([^"]*\)".*/\1/p')
  [ "$advertised" = "$channel" ] || fail "release metadata is for channel '$advertised', not '$channel'"
fi
if [ "$channel" = nightly ]; then
  printf '%s\n' "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+-nightly\.[0-9A-Za-z.]+$' ||
    fail "invalid nightly version: $version"
  download=$base/download/nightly
else
  printf '%s\n' "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$' ||
    fail "invalid stable version: $version (expected X.Y.Z)"
  download=$base/download/v$version
fi

archive=kitbag_${version}_${os}_${arch}.tar.gz
say "downloading kitbag $version for $os/$arch"
fetch "$download/checksums.txt" "$tmp/checksums.txt"
fetch "$download/$archive" "$tmp/$archive"

# Authenticate checksums.txt first: it binds every archive to this release.
cosign=${KITBAG_COSIGN:-cosign}
if command -v "$cosign" >/dev/null 2>&1; then
  fetch "$download/checksums.txt.sigstore.json" "$tmp/checksums.txt.sigstore.json"
  verified=false
  # Release Please runs release.yml from main; a maintainer tag push runs it
  # from that exact tag. Nightly builds are always from main.
  for ref in refs/heads/main "refs/tags/v$version"; do
    [ "$channel" = nightly ] && [ "$ref" != refs/heads/main ] && continue
    if "$cosign" verify-blob --bundle "$tmp/checksums.txt.sigstore.json" \
      --certificate-identity "$workflow@$ref" --certificate-oidc-issuer "$issuer" \
      "$tmp/checksums.txt" >/dev/null 2>&1; then
      verified=true
      break
    fi
  done
  $verified || fail "cosign could not verify checksums.txt for $repository's release workflow; refusing to install"
  say "verified the keyless signature of checksums.txt"
else
  say "cosign not found; skipping signature verification (SHA-256 is still checked)"
fi

expected=$(awk -v name="$archive" '$2 == name && $1 ~ /^[0-9a-f]+$/ && length($1) == 64 { print $1 }' "$tmp/checksums.txt")
[ -n "$expected" ] || fail "checksums.txt has no SHA-256 entry for $archive; refusing to install"
[ "$(printf '%s\n' "$expected" | wc -l | tr -d ' ')" = 1 ] || fail "checksums.txt lists $archive more than once; refusing to install"
actual=$(sha256 "$tmp/$archive")
[ "$actual" = "$expected" ] || fail "checksum mismatch for $archive (expected $expected, got $actual); refusing to install"
say "verified the SHA-256 checksum"

mkdir "$tmp/extract"
(cd "$tmp/extract" && tar -xzf "../$archive" kitbag) || fail "$archive does not contain a kitbag executable"
binary=$tmp/extract/kitbag
if [ -L "$binary" ] || [ ! -f "$binary" ]; then
  fail "the kitbag entry in $archive is not a regular file"
fi
chmod 0755 "$binary"
reported=$("$binary" version --bare 2>/dev/null) || fail "the downloaded kitbag does not run on this machine"
[ "$reported" = "kitbag $version" ] || fail "the downloaded executable reports '$reported', expected 'kitbag $version'"

# Only replace installs this script or a release archive created. Package
# managers own their binaries; never overwrite them behind their back.
target=$prefix/kitbag
stamp=$prefix/kitbag.install-source
if [ -L "$target" ]; then
  fail "$target is a symbolic link (another installer owns it); remove it or choose --prefix"
fi
if [ -e "$target" ]; then
  previous=unknown
  if [ -f "$stamp" ]; then
    previous=$(tr -d ' \t\r\n' <"$stamp")
  fi
  case $previous in
  install-script | release-archive) say "upgrading the existing kitbag in $prefix" ;;
  homebrew | deb | rpm | aur | go-install)
    fail "$target was installed by $previous; upgrade it with that tool instead"
    ;;
  *)
    $force || fail "$target exists but was not installed by this script; rerun with --force to replace it"
    say "replacing $target (--force)"
    ;;
  esac
fi

# Stage next to the target so each rename is atomic on the same filesystem.
mkdir -p "$prefix" || fail "cannot create $prefix"
staged_binary=$prefix/.kitbag.install.$$
staged_stamp=$prefix/.kitbag.install-source.$$
cp "$binary" "$staged_binary" || fail "cannot write to $prefix"
chmod 0755 "$staged_binary" || fail "cannot make $staged_binary executable"
printf 'install-script\n' >"$staged_stamp" || fail "cannot write to $prefix"
mv -f "$staged_binary" "$target" || fail "cannot install $target"
mv -f "$staged_stamp" "$stamp" || fail "cannot write $stamp"
say "installed kitbag $version to $target"

case :${PATH:-}: in
*:"$prefix":*) ;;
*)
  say "$prefix is not on your PATH; add it to your shell profile, for example:"
  # The literal $PATH is meant for the user's shell profile.
  # shellcheck disable=SC2016
  printf '\n  export PATH="%s:$PATH"\n\n' "$prefix"
  ;;
esac
cat <<'EOF'
Next steps:
  kitbag init              # create or connect your configuration repository
  kitbag service install   # background sync; init offers it, or run it later
KitBag uses Git; install it if it is not already available.
EOF
