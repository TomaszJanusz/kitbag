#!/usr/bin/env bash
# Exercise install.sh against fake releases served from file:// URLs.
# Use only Bash 3 features so this also runs on macOS.
set -eo pipefail

cd "$(git rev-parse --show-toplevel)"
installer=$PWD/install.sh
scratch=$(mktemp -d /tmp/kitbag-install-test.XXXXXX)
trap 'rm -rf "$scratch"' EXIT

sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d ' ' -f 1
  else
    shasum -a 256 "$1" | cut -d ' ' -f 1
  fi
}

# Host detection is stubbed so every case is deterministic on any runner.
stubs=$scratch/stubs
mkdir -p "$stubs"
cat >"$stubs/uname" <<'SH'
#!/bin/sh
case $1 in
-s) echo "${FAKE_OS:-Linux}" ;;
-m) echo "${FAKE_ARCH:-x86_64}" ;;
*) exit 2 ;;
esac
SH
cat >"$stubs/sysctl" <<'SH'
#!/bin/sh
echo "${FAKE_TRANSLATED:-0}"
SH
cat >"$stubs/fake-cosign" <<'SH'
#!/bin/sh
printf '%s\n' "$*" >>"$COSIGN_LOG"
case $* in
*"$FAKE_COSIGN_ACCEPT"*) exit 0 ;;
*) exit 1 ;;
esac
SH
chmod +x "$stubs/uname" "$stubs/sysctl" "$stubs/fake-cosign"

# release DIR VERSION: archives for every platform plus their checksums.
release() {
  local dir=$1 version=$2 os arch work
  mkdir -p "$dir"
  for os in darwin linux; do
    for arch in amd64 arm64; do
      work=$scratch/build-$os-$arch
      mkdir -p "$work"
      printf '#!/bin/sh\n# %s/%s\necho "kitbag %s"\n' "$os" "$arch" "$version" >"$work/kitbag"
      chmod +x "$work/kitbag"
      printf 'release-archive\n' >"$work/kitbag.install-source"
      tar -czf "$dir/kitbag_${version}_${os}_${arch}.tar.gz" -C "$work" kitbag kitbag.install-source
      rm -r "$work"
    done
  done
  (cd "$dir" && for file in kitbag_*.tar.gz; do printf '%s  %s\n' "$(sha256 "$file")" "$file"; done) >"$dir/checksums.txt"
  printf 'bundle\n' >"$dir/checksums.txt.sigstore.json"
}

site=$scratch/site
release "$site/download/v1.2.3" 1.2.3
release "$site/download/v1.2.4" 1.2.4
release "$site/download/nightly" 1.2.5-nightly.20261025.1.1
mkdir -p "$site/latest/download"
printf '{\n  "version": "1.2.4",\n  "channel": "stable"\n}\n' >"$site/latest/download/kitbag-update.json"
printf '{"version":"1.2.5-nightly.20261025.1.1","channel":"nightly"}\n' >"$site/download/nightly/kitbag-update.json"

failures=0
case_number=0
output=$scratch/output

# run NAME EXPECT [ARGS...]: EXPECT is ok or fail. Fresh HOME per case unless
# KEEP_HOME is set; the installer sees only the stubbed host tools first.
run() {
  local name=$1 expect=$2 status=0
  shift 2
  case_number=$((case_number + 1))
  if [[ -z "$KEEP_HOME" ]]; then
    home=$scratch/home-$case_number
    mkdir -p "$home"
  fi
  HOME=$home PATH="$stubs:$PATH" KITBAG_RELEASE_BASE_URL="${BASE:-file://$site}" \
    KITBAG_COSIGN="${COSIGN:-kitbag-test-no-cosign}" COSIGN_LOG=$scratch/cosign.log \
    sh "$installer" "$@" >"$output" 2>&1 || status=$?
  if [[ "$expect" == ok && "$status" != 0 || "$expect" == fail && "$status" == 0 ]]; then
    printf 'FAIL %s: expected %s, exit %s\n' "$name" "$expect" "$status"
    sed 's/^/    /' "$output"
    failures=$((failures + 1))
    return 0
  fi
  printf 'ok   %s\n' "$name"
}

# check NAME CONDITION...: evaluate an assertion about the previous case.
check() {
  local name=$1
  shift
  if ! "$@"; then
    printf 'FAIL %s\n' "$name"
    sed 's/^/    /' "$output"
    failures=$((failures + 1))
  fi
}

says() { grep -Fq -- "$1" "$output"; }
installed() { [[ "$("$home/.local/bin/kitbag" version --bare)" == "kitbag $1" ]]; }
stamp_is() { [[ "$(cat "$home/.local/bin/kitbag.install-source")" == "$1" ]]; }
platform_is() { grep -Fqx "# $1" "$home/.local/bin/kitbag"; }
untouched() { [[ ! -e "$home/.local/bin/kitbag" && ! -e "$home/.local/bin/kitbag.install-source" ]]; }
no_staging() { [[ -z "$(find "$home/.local/bin" -name '.kitbag.*' 2>/dev/null)" ]]; }
sh_exact_stamp() { [[ "$(od -An -c "$home/.local/bin/kitbag.install-source" | tr -s ' ')" == " i n s t a l l - s c r i p t \n" ]]; }

run "fresh stable install" ok
check "installs latest stable" installed 1.2.4
check "writes the install-script stamp" sh_exact_stamp
check "linux amd64 archive" platform_is linux/amd64
check "no staging files remain" no_staging
check "prints PATH advice" says 'is not on your PATH'
check "prints next steps" says 'kitbag service install'
check "notes skipped signature" says 'cosign not found'

KEEP_HOME=1 run "upgrade an install-script install" ok --version 1.2.3
check "explicit version wins" installed 1.2.3
KEEP_HOME=1 run "upgrade to latest" ok
check "upgraded to latest" installed 1.2.4

run "explicit v-prefixed version" ok --version=v1.2.3
check "installs requested version" installed 1.2.3
KITBAG_VERSION=1.2.3 run "KITBAG_VERSION" ok
check "installs KITBAG_VERSION" installed 1.2.3

run "nightly channel" ok --channel nightly
check "installs nightly" installed 1.2.5-nightly.20261025.1.1
run "nightly with version" fail --channel nightly --version 1.2.3
run "unknown channel" fail --channel beta

FAKE_ARCH=aarch64 run "linux aarch64" ok
check "maps aarch64 to arm64" platform_is linux/arm64
FAKE_OS=Darwin FAKE_ARCH=arm64 run "darwin arm64" ok
check "maps Darwin arm64" platform_is darwin/arm64
FAKE_OS=Darwin FAKE_ARCH=x86_64 run "darwin amd64" ok
check "maps Darwin x86_64" platform_is darwin/amd64
FAKE_OS=Darwin FAKE_ARCH=x86_64 FAKE_TRANSLATED=1 run "rosetta" ok
check "prefers native arm64 under Rosetta" platform_is darwin/arm64
FAKE_ARCH=riscv64 run "unsupported architecture" fail
check "names the architecture" says 'unsupported architecture: riscv64'
check "nothing installed" untouched
FAKE_OS=FreeBSD run "unsupported OS" fail
check "names the OS" says 'unsupported operating system: FreeBSD'

# Tamper with a copy so the shared site stays valid for later cases.
cp -R "$site" "$scratch/tampered"
# A valid, runnable archive with the right version: only the checksum differs.
cp "$site/download/v1.2.4/kitbag_1.2.4_linux_arm64.tar.gz" "$scratch/tampered/download/v1.2.4/kitbag_1.2.4_linux_amd64.tar.gz"
BASE=file://$scratch/tampered run "tampered archive" fail
check "reports checksum mismatch" says 'checksum mismatch'
check "refuses before installing" untouched
mkdir -p "$home/.local/bin"
printf '#!/bin/sh\necho "kitbag 1.2.3"\n' >"$home/.local/bin/kitbag"
chmod +x "$home/.local/bin/kitbag"
printf 'install-script\n' >"$home/.local/bin/kitbag.install-source"
KEEP_HOME=1 BASE=file://$scratch/tampered run "tampered upgrade" fail
check "keeps the previous binary" installed 1.2.3

cp -R "$site" "$scratch/missing"
grep -v linux_amd64 "$site/download/v1.2.4/checksums.txt" >"$scratch/missing/download/v1.2.4/checksums.txt"
BASE=file://$scratch/missing run "missing checksum" fail
check "reports missing entry" says 'no SHA-256 entry'
check "nothing installed without checksum" untouched

cp -R "$site" "$scratch/duplicate"
grep linux_amd64 "$site/download/v1.2.4/checksums.txt" >>"$scratch/duplicate/download/v1.2.4/checksums.txt"
BASE=file://$scratch/duplicate run "duplicate checksum" fail
check "reports duplicate entry" says 'more than once'

cp -R "$site" "$scratch/mislabelled"
printf '{"version":"1.2.5-nightly.1","channel":"nightly"}\n' >"$scratch/mislabelled/latest/download/kitbag-update.json"
BASE=file://$scratch/mislabelled run "stable metadata from another channel" fail
check "reports channel mismatch" says "channel 'nightly', not 'stable'"
printf '{"version":"../../evil","channel":"stable"}\n' >"$scratch/mislabelled/latest/download/kitbag-update.json"
BASE=file://$scratch/mislabelled run "invalid metadata version" fail
check "rejects path-like versions" says 'invalid stable version'

cp -R "$site" "$scratch/wrong-binary"
work=$scratch/wrong-binary-work
mkdir -p "$work"
printf '#!/bin/sh\necho "kitbag 9.9.9"\n' >"$work/kitbag"
chmod +x "$work/kitbag"
archive=$scratch/wrong-binary/download/v1.2.4/kitbag_1.2.4_linux_amd64.tar.gz
tar -czf "$archive" -C "$work" kitbag
(cd "$scratch/wrong-binary/download/v1.2.4" && for file in kitbag_*.tar.gz; do printf '%s  %s\n' "$(sha256 "$file")" "$file"; done) >"$scratch/wrong-binary/download/v1.2.4/checksums.txt"
BASE=file://$scratch/wrong-binary run "binary reports another version" fail
check "reports version mismatch" says "reports 'kitbag 9.9.9'"
check "nothing installed for wrong version" untouched
ln -s kitbag-real "$work/link"
mv "$work/link" "$work/kitbag"
tar -czf "$archive" -C "$work" kitbag
(cd "$scratch/wrong-binary/download/v1.2.4" && for file in kitbag_*.tar.gz; do printf '%s  %s\n' "$(sha256 "$file")" "$file"; done) >"$scratch/wrong-binary/download/v1.2.4/checksums.txt"
BASE=file://$scratch/wrong-binary run "symlink executable entry" fail
check "rejects linked executables" says 'not a regular file'

BASE=http://example.com/releases run "plain http" fail
check "requires https" says 'must use https'
BASE=http://127.0.0.1:9/releases run "loopback http is allowed" fail
check "loopback http passes the URL policy" says 'download failed'

# Existing installs: package-owned and unknown executables are protected.
for source in homebrew deb rpm aur go-install; do
  home=$scratch/owned-$source
  mkdir -p "$home/.local/bin"
  printf 'owned\n' >"$home/.local/bin/kitbag"
  printf '%s\n' "$source" >"$home/.local/bin/kitbag.install-source"
  KEEP_HOME=1 run "refuse $source install" fail
  check "names $source" says "installed by $source"
  check "keeps $source binary" grep -Fqx owned "$home/.local/bin/kitbag"
done
home=$scratch/unknown
mkdir -p "$home/.local/bin"
printf 'owned\n' >"$home/.local/bin/kitbag"
KEEP_HOME=1 run "refuse unstamped install" fail
check "suggests --force" says '--force'
KEEP_HOME=1 run "force replaces unstamped install" ok --force
check "forced install" installed 1.2.4
check "forced stamp" stamp_is install-script
home=$scratch/linked
mkdir -p "$home/.local/bin"
ln -s /bin/sh "$home/.local/bin/kitbag"
KEEP_HOME=1 run "refuse symlinked target" fail --force
check "explains symlink" says 'symbolic link'

prefix=$scratch/custom
run "custom prefix" ok --prefix "$prefix"
check "installs into prefix" [ -x "$prefix/kitbag" ]
check "stamps prefix" [ "$(cat "$prefix/kitbag.install-source")" = install-script ]
run "unknown option" fail --sudo

# Signature verification runs when cosign exists and pins the workflow identity.
: >"$scratch/cosign.log"
COSIGN=$stubs/fake-cosign FAKE_COSIGN_ACCEPT=release.yml@refs/heads/main run "cosign accepts main identity" ok
check "reports verified signature" says 'verified the keyless signature'
check "pins issuer" grep -Fq -- '--certificate-oidc-issuer https://token.actions.githubusercontent.com' "$scratch/cosign.log"
check "pins main identity" grep -Fq -- '--certificate-identity https://github.com/TomaszJanusz/kitbag/.github/workflows/release.yml@refs/heads/main' "$scratch/cosign.log"
: >"$scratch/cosign.log"
COSIGN=$stubs/fake-cosign FAKE_COSIGN_ACCEPT=release.yml@refs/tags/v1.2.4 run "cosign accepts exact tag identity" ok
check "tries main then tag" [ "$(wc -l <"$scratch/cosign.log" | tr -d ' ')" = 2 ]
COSIGN=$stubs/fake-cosign FAKE_COSIGN_ACCEPT=release.yml@refs/tags/v1.2.3 run "cosign rejects another tag" fail
check "refuses unverified checksums" says 'cosign could not verify'
check "nothing installed without signature" untouched
COSIGN=$stubs/fake-cosign FAKE_COSIGN_ACCEPT=release.yml@refs/tags/v1.2.5-nightly.20261025.1.1 run "nightly accepts only main" fail --channel nightly
rm "$scratch/missing/download/v1.2.4/checksums.txt.sigstore.json"
cp "$site/download/v1.2.4/checksums.txt" "$scratch/missing/download/v1.2.4/checksums.txt"
COSIGN=$stubs/fake-cosign FAKE_COSIGN_ACCEPT=release.yml BASE=file://$scratch/missing run "missing signature bundle" fail
check "reports missing bundle" says 'checksums.txt.sigstore.json'

if ((failures)); then
  printf '%s install.sh check(s) failed\n' "$failures"
  exit 1
fi
printf 'install.sh: all %s cases passed\n' "$case_number"
