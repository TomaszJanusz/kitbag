#!/usr/bin/env bash
# Rehearse a package upgrade of a running KitBag service, hands-off, before a
# release is approved (release gate, see docs/release.md#upgrade-rehearsal).
#
# Each case installs a previous release as the restricted `testing` account,
# starts its systemd user service, replaces the executable with the candidate
# the way a package manager does, and checks that the service comes back on the
# candidate without any user command. Nothing runs as, or writes to, the
# invoking user's HOME.
#
# Linux-only: the run needs sudo rules for `testing` (`sudo -n -iu testing` and
# `sudo -n testing-reset`), a systemd --user manager with Linger=yes, GNU
# `mv -T`, curl, jq and the pinned cosign/go from mise.toml. Option parsing and
# --help use only Bash 3.2 features so they also work on macOS; launchd is not
# rehearsed (docs/release.md lists what this cannot prove).
#
#   scripts/upgrade-rehearsal.sh                         # latest release -> HEAD
#   scripts/upgrade-rehearsal.sh --from-version 0.1.2 --from-version 0.1.1
#   scripts/upgrade-rehearsal.sh --candidate ./kitbag --variant rename
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: scripts/upgrade-rehearsal.sh [options]

  --from-version X.Y.Z  previous release to upgrade from (repeatable; default:
                        the latest GitHub release)
  --candidate PATH      candidate kitbag executable (default: build the current
                        checkout with -X main.version=<candidate version>)
  --candidate-version V version stamped into the default build (default:
                        <next patch>-rehearsal.<commit>)
  --variant NAME        rename (atomic overwrite of the installed file, as
                        install.sh/dpkg/rpm do), stale (rename after changing
                        a generated unit directive, so the new daemon must
                        repair its definition at startup), symlink
                        (Homebrew-style keg plus opt/bin link swap and
                        cleanup) or all (default)
  --timeout SECONDS     hands-off wait per case (default 300)
  --observe SECONDS     how long a case that is not expected to self-heal is
                        watched before the explicit fallback (default 130)
  --account NAME        throwaway account (default testing)
  --keep                leave the last case installed and running
  --no-fetch            do not fetch release tags from origin
  -h, --help            show this help

Exit status: 0 when every case met its expectation, 1 otherwise, 2 on usage or
environment errors.
EOF
}

die() {
  printf 'upgrade-rehearsal: error: %s\n' "$*" >&2
  exit 2
}

from_versions=
candidate=
candidate_version=
variants=all
timeout=300
observe=130
account=testing
keep=false
fetch=true
while [ $# -gt 0 ]; do
  case $1 in
  --from-version)
    [ $# -ge 2 ] || die "--from-version needs a value"
    from_versions="$from_versions ${2#v}"
    shift 2
    ;;
  --candidate)
    [ $# -ge 2 ] || die "--candidate needs a path"
    candidate=$2
    shift 2
    ;;
  --candidate-version)
    [ $# -ge 2 ] || die "--candidate-version needs a value"
    candidate_version=$2
    shift 2
    ;;
  --variant)
    [ $# -ge 2 ] || die "--variant needs a value"
    variants=$2
    shift 2
    ;;
  --timeout)
    [ $# -ge 2 ] || die "--timeout needs seconds"
    timeout=$2
    shift 2
    ;;
  --observe)
    [ $# -ge 2 ] || die "--observe needs seconds"
    observe=$2
    shift 2
    ;;
  --account)
    [ $# -ge 2 ] || die "--account needs a name"
    account=$2
    shift 2
    ;;
  --keep)
    keep=true
    shift
    ;;
  --no-fetch)
    fetch=false
    shift
    ;;
  -h | --help)
    usage
    exit 0
    ;;
  *) die "unknown option: $1 (see --help)" ;;
  esac
done

case $variants in
all) variants="rename stale symlink" ;;
rename | stale | symlink) ;;
*) die "--variant must be rename, stale, symlink or all" ;;
esac
for value in "$timeout" "$observe"; do
  case $value in
  '' | *[!0-9]*) die "durations are whole seconds: $value" ;;
  esac
done
for v in $from_versions; do
  printf '%s\n' "$v" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$' || die "invalid release version: $v"
done

[ "$(uname -s)" = Linux ] || die "the rehearsal needs Linux with systemd --user (launchd is not rehearsed)"
for tool in git curl jq sudo mise; do
  command -v "$tool" >/dev/null 2>&1 || die "$tool is required"
done

cd "$(git rev-parse --show-toplevel)"
poll=5

lock=${TMPDIR:-/tmp}/kitbag-upgrade-rehearsal.lock
mkdir "$lock" 2>/dev/null || die "another rehearsal holds $lock (remove it if that run is gone)"
scratch=$(mktemp -d "${TMPDIR:-/tmp}/kitbag-upgrade-rehearsal.XXXXXX")

# as_account runs the Bash script on stdin as the throwaway account, in a clean
# login environment (sudo -i resets HOME, PATH and every agent variable).
as_account() {
  sudo -n -iu "$account" bash -s -- "$@"
}

# kb runs the installed kitbag (whatever the channel's PATH finds now) as the
# account, with stderr merged.
kb() {
  as_account "$@" <<'EOF'
. "$HOME/.kitbag-rehearsal/env"
kitbag "$@" 2>&1
EOF
}

main_pid() {
  as_account <<'EOF'
systemctl --user show dev.kitbag.agent.service -p MainPID --value
EOF
}

# push copies a file into the account's rehearsal directory as an executable.
push() {
  # The invoking user reads the source; only the write happens as the account.
  # shellcheck disable=SC2024
  sudo -n -u "$account" -H sh -c 'umask 022 && mkdir -p "$(dirname "$HOME/.kitbag-rehearsal/$1")" && cat >"$HOME/.kitbag-rehearsal/$1" && chmod 0755 "$HOME/.kitbag-rehearsal/$1"' sh "$2" <"$1"
}

reset_account() {
  sudo -n testing-reset >/dev/null
  local waited=0
  until [ "$(as_account <<<'systemctl --user is-system-running 2>/dev/null' || :)" = running ]; do
    waited=$((waited + 1))
    [ "$waited" -lt 30 ] || die "the $account systemd --user manager did not come back after reset"
    sleep 1
  done
}

cleanup() {
  status=$?
  if ! $keep; then
    sudo -n testing-reset >/dev/null 2>&1 || printf 'upgrade-rehearsal: warning: testing-reset failed; clean %s by hand\n' "$account" >&2
  fi
  rm -rf "$scratch"
  rmdir "$lock" 2>/dev/null || :
  exit "$status"
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM

# Refuse to wipe an account that someone else is using right now: only the user
# manager and its helpers may run before the first reset.
busy=$(pgrep -l -u "$account" | awk '{ print $2 }' | grep -Ev '^(systemd|\(sd-pam\)|dbus-daemon|kitbag)$' || :)
[ -z "$busy" ] || die "$account is running other processes ($(printf '%s' "$busy" | tr '\n' ' ')); it may be in use"
sudo -n -iu "$account" true 2>/dev/null || die "cannot run commands as $account with sudo -n -iu"
sudo -n -l testing-reset >/dev/null 2>&1 || die "sudo -n testing-reset is not permitted"

if $fetch; then
  git fetch -q origin 'refs/tags/v*:refs/tags/v*'
fi
release_tags=$(git tag -l 'v[0-9]*.[0-9]*.[0-9]*' --sort=v:refname)

# Feature detection reads the released source, so expectations follow history
# rather than a hand-maintained table.
has_self_restart() { git cat-file -e "$1:internal/daemonhost/executable.go" 2>/dev/null; }
has_service_refresh() { git cat-file -e "$1:cmd/kitbag/service_refresh.go" 2>/dev/null; }
has_linux_stable_link() { git show "$1:internal/platform/unix/service_linux.go" 2>/dev/null | grep -q stableExecutable; }
first_release() {
  local tag
  for tag in $release_tags; do
    if "$1" "$tag"; then
      printf '%s' "${tag#v}"
      return
    fi
  done
  printf 'unreleased'
}

latest=$(printf '%s\n' "$release_tags" | tail -n 1)
[ -n "$latest" ] || die "no release tags found"
[ -n "$from_versions" ] || from_versions=${latest#v}
for v in $from_versions; do
  git rev-parse -q --verify "refs/tags/v$v" >/dev/null || die "v$v is not a release tag"
done

cosign=$(mise which cosign 2>/dev/null) || die "the pinned cosign is not installed (mise install)"

started=$(date +%s)
if [ -z "$candidate" ]; then
  head=$(git rev-parse --short HEAD)
  if [ -z "$candidate_version" ]; then
    IFS=. read -r major minor patch <<<"${latest#v}"
    candidate_version=$major.$minor.$((patch + 1))-rehearsal.$head
    if [ -n "$(git status --porcelain -- '*.go' go.mod go.sum)" ]; then
      candidate_version=$candidate_version.dirty
    fi
  fi
  candidate=$scratch/kitbag
  printf 'Building candidate %s from %s (origin/main is %s)\n' "$candidate_version" "$head" "$(git rev-parse --short origin/main 2>/dev/null || echo unknown)"
  build_started=$(date +%s)
  CGO_ENABLED=0 mise exec -- go build -trimpath -ldflags "-s -w -X main.version=$candidate_version" -o "$candidate" ./cmd/kitbag
  printf 'Built in %ss\n' "$(($(date +%s) - build_started))"
  candidate_source="checkout $(git rev-parse HEAD)"
else
  [ -x "$candidate" ] || die "candidate is not an executable: $candidate"
  candidate_source="executable $candidate"
fi
reported=$("$candidate" version --bare) || die "the candidate does not run"
case $reported in
"kitbag "*) ;;
*) die "unexpected candidate version output: $reported" ;;
esac
if [ -n "$candidate_version" ] && [ "$reported" != "kitbag $candidate_version" ]; then
  die "candidate reports '$reported', expected 'kitbag $candidate_version'"
fi
candidate_version=${reported#kitbag }

printf '\n== Upgrade rehearsal\n'
printf 'Candidate: %s (%s)\n' "$candidate_version" "$candidate_source"
printf 'Account: %s, Linux systemd --user, sync.poll_interval %ss\n' "$account" "$poll"
printf 'First release with executable self-restart (exit 75): %s\n' "$(first_release has_self_restart)"
printf 'First release with service refresh and daemon startup repair: %s\n' "$(first_release has_service_refresh)"
printf 'First release whose Linux unit names the stable Homebrew link: %s\n' "$(first_release has_linux_stable_link)"
printf 'Expected hands-off upgrades to this candidate (Linux):\n'
for tag in $release_tags; do
  rename=no
  symlink=no
  if has_self_restart "$tag"; then
    rename=yes
    if has_linux_stable_link "$tag"; then
      symlink=yes
    fi
  fi
  printf '  from %-8s rename/stale: %-3s symlink (Homebrew keg): %s\n' "${tag#v}" "$rename" "$symlink"
done
printf 'Older daemons need one kitbag service install or service refresh with the new binary.\n'

failures=0
summary=
case_failed=false

check() {
  # check NAME CONDITION-EXIT-STATUS DETAIL
  if [ "$2" = 0 ]; then
    printf '  PASS %s%s\n' "$1" "${3:+: $3}"
  else
    printf '  FAIL %s%s\n' "$1" "${3:+: $3}"
    case_failed=true
  fi
}

since() { printf '%s' "$(($(date +%s) - $1))"; }

# The snippets below run as the account. $R holds every fixture; $R/env selects
# the bin directory a user of that installation channel would have on PATH.
fixture() {
  as_account "$1" "$2" <<'EOF'
set -eu
from=$1 variant=$2
R=$HOME/.kitbag-rehearsal
mkdir -p "$R/tools" "$R/candidate"
G="git -c user.name=Rehearsal -c user.email=rehearsal@example.invalid -c init.defaultBranch=main"
$G init -q --bare "$R/remote.git"
$G init -q "$R/seed"
cd "$R/seed"
printf 'schema = 1\n\n[instructions]\nsource = "global/AGENTS.md"\n\n[targets.claude]\nagent = "claude"\n' >kitbag.toml
mkdir global
printf '# Upgrade rehearsal\n\nThrowaway instructions.\n' >global/AGENTS.md
$G add -A
$G commit -qm 'chore: seed rehearsal configuration'
$G push -q "$R/remote.git" HEAD:main
cd "$HOME"
curl -fsSL "https://github.com/TomaszJanusz/kitbag/releases/download/v$from/install.sh" -o "$R/install.sh"
# Prepend only when missing: ~/.profile adds ~/.local/bin once it exists, and
# service install persists PATH, so a duplicate would change the definition.
if [ "$variant" != symlink ]; then
  prefix=$HOME/.local/bin
  bindir=$prefix
else
  prefix=$R/stage
  bindir=$R/brew/bin
fi
printf 'case ":$PATH:" in *":%s:"*) ;; *) export PATH="%s:$PATH" ;; esac\n' "$bindir" "$bindir" >"$R/env"
KITBAG_COSIGN=$R/tools/cosign sh "$R/install.sh" --version "$from" --prefix "$prefix" >"$R/install.log" 2>&1 || {
  cat "$R/install.log"
  exit 1
}
grep -q 'verified the keyless signature' "$R/install.log" || {
  cat "$R/install.log"
  echo "install.sh did not verify the cosign signature" >&2
  exit 1
}
if [ "$variant" = symlink ]; then
  # Homebrew layout: Cellar/<formula>/<version>/libexec/kitbag with the
  # formula's bin link, plus prefix opt/ and bin/ links.
  keg=$R/brew/Cellar/kitbag/$from
  mkdir -p "$keg/libexec" "$keg/bin" "$R/brew/opt" "$R/brew/bin"
  mv "$prefix/kitbag" "$keg/libexec/kitbag"
  ln -s ../libexec/kitbag "$keg/bin/kitbag"
  ln -s "../Cellar/kitbag/$from" "$R/brew/opt/kitbag"
  ln -s "../Cellar/kitbag/$from/bin/kitbag" "$R/brew/bin/kitbag"
fi
. "$R/env"
grep -E 'verified|installed' "$R/install.log" | sed 's/^/    /'
kitbag init "$R/remote.git" --yes >"$R/init.log" 2>&1 || {
  cat "$R/init.log"
  exit 1
}
kitbag config set host kitbag-rehearsal
kitbag config set sync.poll_interval 5s
kitbag config set daemon.update_check false
kitbag service install 2>&1 | sed 's/^/    /'
if [ "$variant" = stale ]; then
  # Model a definition from an older generator: a changed directive, which
  # comparison treats as significant, loaded by the supervisor.
  unit=$HOME/.config/systemd/user/dev.kitbag.agent.service
  sed 's/^RestartSec=5s$/RestartSec=4s/' "$unit" >"$unit.tmp"
  grep -q '^RestartSec=4s$' "$unit.tmp"
  mv "$unit.tmp" "$unit"
  systemctl --user daemon-reload
  echo "    changed RestartSec=5s to 4s in the installed unit and reloaded systemd"
fi
EOF
}

# probe prints one line: active pid daemon_version stale binary_missing exec.
# daemon_version is read through the candidate copy because older status output
# lacks it. Staleness depends on the executable that asks, so service status
# runs the installed kitbag the channel's PATH finds. Neither command restarts.
probe() {
  as_account <<'EOF'
. "$HOME/.kitbag-rehearsal/env"
active=$(systemctl --user is-active dev.kitbag.agent.service 2>/dev/null || :)
pid=$(systemctl --user show dev.kitbag.agent.service -p MainPID --value 2>/dev/null || :)
exec=$(systemctl --user show dev.kitbag.agent.service -p ExecStart --value 2>/dev/null | sed -n 's/.*argv\[\]=\([^ ;]*\).*/\1/p')
dv=$("$HOME/.kitbag-rehearsal/candidate/kitbag" status --json 2>/dev/null | jq -r '.daemon_version // "none"' 2>/dev/null || echo none)
svc=$(kitbag service status --json 2>/dev/null | jq -r '"\(.stale) \(.binary_missing)"' 2>/dev/null || echo 'unknown unknown')
printf '%s %s %s %s %s\n' "${active:-unknown}" "${pid:-0}" "${dv:-none}" "$svc" "${exec:-none}"
EOF
}

replace_rename() {
  as_account <<'EOF'
set -eu
R=$HOME/.kitbag-rehearsal
bin=$HOME/.local/bin
# Same directory, then rename(2) over the old file, like dpkg/rpm/install.sh.
cp "$R/candidate/kitbag" "$bin/.kitbag.upgrade.$$"
chmod 0755 "$bin/.kitbag.upgrade.$$"
mv -f "$bin/.kitbag.upgrade.$$" "$bin/kitbag"
EOF
}

replace_symlink() {
  as_account "$1" "$2" <<'EOF'
set -eu
from=$1 to=$2
B=$HOME/.kitbag-rehearsal/brew
# brew upgrade: pour the new keg, repoint opt/ and bin/ atomically
# (symlink + rename), then cleanup deletes the old keg.
mkdir -p "$B/Cellar/kitbag/$to/libexec" "$B/Cellar/kitbag/$to/bin"
cp "$HOME/.kitbag-rehearsal/candidate/kitbag" "$B/Cellar/kitbag/$to/libexec/kitbag"
chmod 0755 "$B/Cellar/kitbag/$to/libexec/kitbag"
ln -s ../libexec/kitbag "$B/Cellar/kitbag/$to/bin/kitbag"
ln -s "../Cellar/kitbag/$to" "$B/opt/.kitbag.new"
mv -T "$B/opt/.kitbag.new" "$B/opt/kitbag"
ln -s "../Cellar/kitbag/$to/bin/kitbag" "$B/bin/.kitbag.new"
mv -T "$B/bin/.kitbag.new" "$B/bin/kitbag"
rm -rf "$B/Cellar/kitbag/$from"
EOF
}

# wait_converged SECONDS OLD_PID: waits until the service runs the candidate
# with a current definition. Sets t_pid, t_version, t_done (seconds since
# $t0) and pids (every MainPID seen).
wait_converged() {
  local limit=$1 old=$2 line active pid dv stale missing
  t_pid='' t_version='' t_done=''
  pids=$old
  while [ "$(since "$t0")" -le "$limit" ]; do
    line=$(probe)
    read -r active pid dv stale missing _ <<<"$line"
    case " $pids " in
    *" $pid "*) ;;
    *) [ "$pid" = 0 ] || pids="$pids $pid" ;;
    esac
    if [ -z "$t_pid" ] && [ "$pid" != "$old" ] && [ "$pid" != 0 ]; then t_pid=$(since "$t0"); fi
    if [ -z "$t_version" ] && [ "$dv" = "$candidate_version" ]; then t_version=$(since "$t0"); fi
    if [ "$active" = active ] && [ "$pid" != "$old" ] && [ "$dv" = "$candidate_version" ] &&
      [ "$stale" = false ] && [ "$missing" = false ]; then
      # Stable for one poll interval: no restart loop.
      sleep "$poll"
      if [ "$(probe)" = "$line" ]; then
        t_done=$(since "$t0")
        return 0
      fi
      continue
    fi
    sleep 3
  done
  last_probe=$line
  return 1
}

post_checks() {
  local variant=$1 converged_at=$2 path=$3 out pid before after
  # Last check must be newer than convergence: wait up to three polls.
  local fresh=1 at=0 waited=0
  while [ "$waited" -le $((poll * 3 + 5)) ]; do
    at=$(kb status --json |
      jq -r '.last_check.at // "" | sub("\\.[0-9]+"; "") | if . == "" then 0 else fromdateiso8601 end')
    if [ "$at" -ge "$converged_at" ]; then
      fresh=0
      break
    fi
    sleep 2
    waited=$((waited + 2))
  done
  check "Last check is fresh" "$fresh" "$(kb status | grep -E '^(Versions|Last check):' | tr '\n' ';' | sed 's/;$//')"

  out=$(kb doctor --json || :)
  local fails mismatch service
  fails=$(printf '%s' "$out" | jq -r '[.checks[] | select(.status == "FAIL") | .id] | join(",")')
  mismatch=$(printf '%s' "$out" | jq -r '[.checks[] | select(.id == "daemon-version")][0] | "\(.status): \(.message)"')
  service=$(printf '%s' "$out" | jq -r '[.checks[] | select(.id == "service")][0] | "\(.status): \(.message)"')
  check "doctor has no FAIL" "$([ -z "$fails" ] && echo 0 || echo 1)" "${fails:-none}"
  check "doctor daemon-version is OK" "$(case $mismatch in OK:*) echo 0 ;; *) echo 1 ;; esac)" "$mismatch"
  check "doctor service is OK" "$(case $service in OK:*) echo 0 ;; *) echo 1 ;; esac)" "$service"

  if [ "$variant" = stale ]; then
    out=$(
      as_account <<'EOF'
grep -c '^RestartSec=5s$' "$HOME/.config/systemd/user/dev.kitbag.agent.service" || :
cat "$HOME/.local/state/kitbag/service-refresh.json" 2>/dev/null || echo 'no service-refresh.json'
EOF
    )
    if [ "$path" = hands-off ]; then
      check "daemon repaired the unit at startup" "$(case $out in 1*'"attempts"'*) echo 0 ;; *) echo 1 ;; esac)" "RestartSec=5s restored, marker: $(printf '%s' "$out" | tail -n 1)"
    else
      check "service install repaired the unit" "$(case $out in 1*) echo 0 ;; *) echo 1 ;; esac)" "RestartSec=5s restored"
    fi
  fi
  if [ "$variant" = symlink ]; then
    read -r _ _ _ _ _ exec <<<"$(probe)"
    check "unit names the stable opt link" "$(case $exec in */brew/opt/kitbag/*) echo 0 ;; *) echo 1 ;; esac)" "$exec"
  fi

  before=$(main_pid)
  out=$(kb service install || :)
  sleep "$poll"
  after=$(main_pid)
  check "service install says up to date" "$(printf '%s' "$out" | grep -q 'already installed and up to date' && echo 0 || echo 1)" "$(printf '%s' "$out" | head -n 1)"
  check "service install did not restart" "$([ "$before" = "$after" ] && echo 0 || echo 1)" "MainPID $before -> $after"

  out=$(kb service refresh --dry-run --json || :)
  check "service refresh --dry-run is a no-op" \
    "$(printf '%s' "$out" | jq -e '.installed and .dry_run and (.changed | not) and (.definition_changed | not) and (.restart_needed | not)' >/dev/null && echo 0 || echo 1)" "$out"
  pid=$(main_pid)
  check "dry run did not restart" "$([ "$pid" = "$after" ] && echo 0 || echo 1)" "MainPID $pid"
}

run_case() {
  local from=$1 variant=$2 expect=no line old_pid active dv out case_started t_replace
  case_failed=false
  if has_self_restart "v$from"; then
    if [ "$variant" != symlink ] || has_linux_stable_link "v$from"; then expect=yes; fi
  fi
  case_started=$(date +%s)
  printf '\n== Case %s -> %s, variant %s (hands-off expected: %s)\n' "$from" "$candidate_version" "$variant" "$expect"
  reset_account
  push "$cosign" tools/cosign
  push "$candidate" candidate/kitbag
  printf '  Installing %s with install.sh (SHA-256 + cosign) and starting the service\n' "$from"
  if ! fixture "$from" "$variant"; then
    check "install previous release" 1 "fixture setup failed"
    finish_case "$from" "$variant" "$expect" "setup failed" "$case_started"
    return
  fi
  sleep 3
  line=$(probe)
  read -r active old_pid dv _ <<<"$line"
  check "old service is active on $from" "$([ "$active" = active ] && [ "$dv" = "$from" ] && echo 0 || echo 1)" "active=$active MainPID=$old_pid daemon=$dv"

  printf '  Replacing the executable (%s) at %s; no kitbag command that restarts anything runs now\n' "$variant" "$(date '+%H:%M:%S')"
  t0=$(date +%s)
  t_replace=$t0
  if [ "$variant" = symlink ]; then replace_symlink "$from" "$candidate_version"; else replace_rename; fi

  local outcome
  if [ "$expect" = yes ]; then
    if wait_converged "$timeout" "$old_pid"; then
      check "hands-off restart onto $candidate_version" 0 "first MainPID change +${t_pid}s, candidate version +${t_version}s, converged +${t_done}s; MainPIDs: $pids"
      outcome="hands-off in ${t_done}s"
      post_checks "$variant" "$((t0 + t_done))" hands-off
    else
      check "hands-off restart onto $candidate_version" 1 "not converged within ${timeout}s; last probe: $last_probe; MainPIDs: $pids"
      outcome="NOT hands-off within ${timeout}s"
    fi
  else
    if wait_converged "$observe" "$old_pid"; then
      printf '  NOTE unexpectedly self-healed in %ss\n' "$t_done"
      outcome="hands-off in ${t_done}s (unexpected)"
      post_checks "$variant" "$((t0 + t_done))" hands-off
    else
      printf '  INFO not self-healed after %ss, as expected; last probe: %s\n' "$observe" "$last_probe"
      t0=$(date +%s)
      out=$(kb service install || :)
      printf '%s\n' "$out" | sed 's/^/    | /'
      check "service install restarts the old daemon" \
        "$(printf '%s' "$out" | grep -Eq "Restarted the running daemon \(($from|older) -> $candidate_version\)" && echo 0 || echo 1)" \
        "$(printf '%s' "$out" | grep 'Restarted the running daemon' || echo 'no Restarted line')"
      if wait_converged "$timeout" "$old_pid"; then
        check "service runs $candidate_version after one service install" 0 "converged +${t_done}s"
        outcome="needs one service install (expected); converged ${t_done}s after it"
        post_checks "$variant" "$((t0 + t_done))" fallback
      else
        check "service runs $candidate_version after one service install" 1 "last probe: $last_probe"
        outcome="service install did not converge"
      fi
    fi
  fi
  printf '  Journal since the replacement:\n'
  as_account "$t_replace" <<'EOF' | sed 's/^/    | /'
journalctl --user -u dev.kitbag.agent.service -o short-iso --no-pager --since "@$1" 2>/dev/null | tail -n 12
EOF
  finish_case "$from" "$variant" "$expect" "$outcome" "$case_started"
}

finish_case() {
  local result=ok
  if $case_failed; then
    result=FAILED
    failures=$((failures + 1))
  fi
  summary="$summary$(printf '%-7s %-8s %-8s expected:%-4s %-6s %ss  %s' "$1" "$2" "$candidate_version" "$3" "$result" "$(since "$5")" "$4")
"
}

for from in $from_versions; do
  for variant in $variants; do
    run_case "$from" "$variant"
  done
done

printf '\n== Summary (%ss total)\n%s' "$(since "$started")" "$summary"
if $keep; then
  printf 'Kept the last case running as %s (clean with: sudo -n testing-reset)\n' "$account"
fi
if [ "$failures" -gt 0 ]; then
  printf '%s case(s) did not meet expectations\n' "$failures"
  exit 1
fi
printf 'All cases met expectations\n'
