# Release automation

The release pipeline builds the CLI for darwin/linux × amd64/arm64 using the
versions pinned in [mise.toml](../mise.toml), plus Linux deb/rpm packages and the
install script. Stable releases then update the Homebrew tap and AUR package when
their maintainer-provided secrets exist ([channels](#distribution-channels);
user guide: [install.md](install.md)). Windows belongs to a later milestone. No
package repository or long-lived signing credential is used here.

## Approval and triggers

A maintainer manually runs **Release Please** on `main` to propose or refresh a
version and `CHANGELOG.md` from Conventional Commits. The initial `0.0.0`
manifest is a bootstrap baseline, not a published release. Release Please uses
its simple strategy and `.release-please-version`; executable stamping always
comes from GoReleaser's version, never that file at runtime.

**First release pin.** The one-time `"release-as": "0.1.0"` pin was removed from
`release-please-config.json` after v0.1.0 was published, so later proposals
follow the commit history again.

`bump-minor-pre-major` and `bump-patch-for-minor-pre-major` keep versions below
`1.0.0` on the usual pre-1.0 rules: breaking changes bump the minor and features
bump the patch, so only a deliberate `release-as` reaches `1.0.0`.

**Merging a release PR requires the maintainer's explicit approval for that exact
version.** Never enable automerge on release PRs. Review the proposed version,
changelog and successful checks before merging. Main pushes only finalize
already merged release PRs; they do not open new proposals. Merely merging the
pipeline does not create a release proposal, tag or release.

The merge's main push lets Release Please create the tag and GitHub release,
then directly calls `release.yml` with its tag and commit outputs. This avoids
GitHub's suppression of events created by `GITHUB_TOKEN`: a `release: published`
trigger would not solve that problem. A maintainer-pushed `vMAJOR.MINOR.PATCH`
tag also runs the same pipeline. No other tag syntax publishes stable assets.
Do not both push a version tag and merge a release PR for the same version.

The default token also suppresses CI events for bot-created proposal PRs.
Close/reopen that PR manually (or push a reviewed change to its branch) to
trigger the required checks. Repository Actions must already permit Actions to
create PRs; enabling that setting is a maintainer task, not done by this change.
No PAT is introduced.

## Upgrade rehearsal

**Release gate.** After a package upgrade the user must not need any manual step:
no `kitbag service install`, no restart, no `init`. Before asking the maintainer
to approve a release PR, the release manager runs the upgrade rehearsal on the
Linux QA host and attaches its summary to the request. A case that misses its
expectation blocks the release until the change is fixed; a README note or a
package caveat is not a fix.

```sh
scripts/upgrade-rehearsal.sh                      # latest release -> origin/main
scripts/upgrade-rehearsal.sh --from-version 0.1.2 --from-version 0.1.1
scripts/upgrade-rehearsal.sh --candidate ./kitbag --variant rename --timeout 300
```

The script runs every step as the restricted `testing` account through
`sudo -n -iu testing`, whose login environment is clean, so it never touches the
invoking user's HOME or agent directories. That account has a real
`systemd --user` manager with `Linger=yes`, and `sudo -n testing-reset` wipes it.
Fixtures are created by `testing` itself, because Git refuses repositories owned
by another user (`safe.directory`). The script resets the account before each
case and on exit (`--keep` leaves the last case running), refuses to start while
the account runs anything besides its user manager, and serializes runs with
`$TMPDIR/kitbag-upgrade-rehearsal.lock`.

The default candidate is freshly fetched `origin/main`, exported with
`git archive` (local edits never reach it) and built with the pinned Go exactly
like a release,
`CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=<version>"`,
stamped `<next patch>-rehearsal.<commit>`. `--ref REF` builds another commit,
normally the one you are about to release; `--candidate PATH` uses an existing
executable instead. The default previous version is the latest GitHub release
tag.

Each case (`--from-version` × `--variant`; the default runs all three variants):

1. Installs the previous release with that release's published `install.sh`,
   verifying SHA-256 and the keyless cosign signature with the cosign pinned in
   `mise.toml`. It creates a local bare configuration remote with a tiny
   `kitbag.toml` and `global/AGENTS.md`, runs `kitbag init --yes`, sets `host`,
   `sync.poll_interval = 5s` and `daemon.update_check = false`, and runs
   `kitbag service install`, so the old daemon runs under systemd.
2. Replaces the executable like a package manager. `rename` copies the candidate
   next to `~/.local/bin/kitbag` and renames it over the file (install.sh, dpkg
   and rpm). `stale` does the same after changing `RestartSec` in the installed
   unit and reloading systemd. It models a definition written by an older
   generator, so the new daemon must repair its own unit at startup (rewrite,
   `daemon-reload`, exit 75) and the script asserts the restored directive and the
   repair marker. Plain `rename` from 0.1.2 does not test this: the 0.1.2 unit
   already matches the current generator. `symlink` models Homebrew: the old
   version lives in `Cellar/kitbag/<version>/libexec`, the user runs
   `<prefix>/bin/kitbag`, and the upgrade pours a new keg, swaps the
   `opt/kitbag` and `bin/kitbag` links with symlink plus `mv -T`, then deletes
   the old keg like `brew cleanup`.
3. Runs no command that could restart anything and waits up to `--timeout`
   seconds (default 300) until the unit is active, its `MainPID` changed, the
   status API's `daemon_version` equals the candidate, and `kitbag service
   status` reports neither a stale definition nor a missing executable, stable
   for one more poll. It then asserts that `Last check` is newer than that moment
   and that `kitbag doctor` has no FAIL and reports `daemon-version` and
   `service` as OK. Symlink cases also assert that the unit now names the stable
   `opt` link.
4. Asserts that `kitbag service install` says "already installed and up to date"
   without changing `MainPID`, and that `kitbag service refresh --dry-run --json`
   reports no change, rewrite or restart. Install persists the invoking shell's
   PATH, so every session in the script uses the same PATH; a different PATH is
   a changed definition and legitimately restarts the daemon.

The script reads release tags to decide which cases must be hands-off: the old
daemon has to restart itself when its executable changes (exit 75, first in
0.1.2), and for `symlink` its Linux unit must already name the stable Homebrew
link (not in any release up to 0.1.2). Cases older than that are observed for
`--observe` seconds (default 130) to confirm they do not self-heal. The script
then asserts that one `kitbag service install` with the new binary prints
`Restarted the running daemon (<old> -> <candidate>)` and converges, and runs
steps 3 and 4. Such a case passes, but its summary says `MANUAL STEP`: users of
that version and channel have one manual step
([service.md](service.md#refresh-after-upgrades)), which the release notes must
name. The rule forbids new manual steps: a case expected to be hands-off that
needs one blocks the release. The summary lists every case, its
expectation, its result and its timings. The exit status is nonzero when any
case misses its expectation.

The old daemon checks its executable once a minute and needs two matching
observations, so a hands-off case takes about two minutes; one run of all three
variants for two versions takes about 16 minutes.

**What it cannot prove.** It covers Linux with systemd only. launchd on macOS
(`KeepAlive`, bootout/bootstrap during startup repair, the detached refresh
helper, Homebrew's real `opt` link under `/opt/homebrew`) still needs a manual
rehearsal on a Mac, and so do real `brew`, dpkg and rpm transactions: the script
reproduces their file operations, not the tools. It does not cover timer mode,
`--no-start` installations, linger disabled, logout, reboot or SSH remotes. One
host and one run cannot rule out timing races.

## Artifacts and trust

GoReleaser creates `kitbag_<version>_<GOOS>_<GOARCH>.tar.gz`, each containing
root entries `kitbag`, `LICENSE`, `README.md`, and `kitbag.install-source` with
`release-archive` plus a newline. Cobra completion is disabled in this CLI;
there are no completion files to package. Archive names, root executable and
stamp follow [the direct-install protocol](self-update.md#direct-installation-protocol).

nfpm (through GoReleaser) builds `kitbag_<version>_<arch>.deb` and
`kitbag-<version>-1.<arch>.rpm` for amd64/arm64 (prerelease versions use `~`).
They install `/usr/lib/kitbag/kitbag`, a `deb` or `rpm` stamp beside it from
`packaging/`, the license, and a `/usr/bin/kitbag` link, and recommend `git`.
`install.sh` is copied unchanged from the tagged commit.

Syft produces an SPDX JSON SBOM for each archive. `checksums.txt` uses SHA-256
with two spaces before the filename and covers archives, packages, SBOMs,
`install.sh` and `kitbag-update.json` discovery metadata. Cosign keylessly signs that manifest
with GitHub OIDC; `checksums.txt.sigstore.json` carries the certificate,
signature and transparency-log proof. Authenticating the manifest authenticates
each listed artifact. SLSA's isolated generic generator binds all of these files
(including the manifest and bundle) to `kitbag.intoto.jsonl` provenance. The
publishing job verifies every subject using the pinned `slsa-verifier` first.

**Self-update compatibility is blocked.** `internal/releasesig` currently
accepts a base64 detached ECDSA P-256 signature in `checksums.txt.sig`, checked
against the empty, fail-closed `releasePublicKey` constant. A keyless bundle is
not that format. Publishing a pretend `.sig` or pinning an ephemeral Fulcio key
would not make this secure. This pipeline emits the required keyless bundle and
leaves the verifier and trust anchor unchanged; `kitbag self-update` cannot
verify these releases yet. Verify downloads manually as below. Do not advertise
an updater-enabled release until the follow-up trust design is implemented.

The smallest compatible direction is to extend `internal/releasesig` with an
exactly pinned `sigstore-go`, confine it with depguard, load the bundle with
bounded HTTP, and pin the expected GitHub workflow identity and issuer in the
binary. This requires reviewed TUF trust-root/bootstrap and offline/freshness
policy, certificate and log-proof validation, channel/version binding and
negative tests for wrong identities, issuers, subjects and tampering. It is
larger than a pipeline change and remains an open maintainer decision. A
long-lived project signing key in a GitHub secret is explicitly excluded.

## Verify a download

Download the selected release's four archives, SBOMs, metadata, `checksums.txt`,
`checksums.txt.sigstore.json`, and `kitbag.intoto.jsonl` into an empty directory.
Use the pinned tools from a trusted checkout with `mise exec --`:

```sh
cosign verify-blob --bundle checksums.txt.sigstore.json \
  --certificate-identity 'https://github.com/TomaszJanusz/kitbag/.github/workflows/release.yml@refs/heads/main' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com checksums.txt
sha256sum --check checksums.txt
slsa-verifier verify-artifact kitbag_0.1.0_linux_amd64.tar.gz \
  --provenance-path kitbag.intoto.jsonl \
  --source-uri github.com/TomaszJanusz/kitbag \
  --source-branch main --print-provenance > verified-provenance.json
jq -e --arg sha RELEASE_COMMIT_SHA \
  '.predicate.invocation.configSource.digest.sha1 == $sha' verified-provenance.json
```

Replace `0.1.0` and `RELEASE_COMMIT_SHA` with the reviewed release version and
full tag commit. For a maintainer-pushed tag run, use certificate identity
`https://github.com/TomaszJanusz/kitbag/.github/workflows/release.yml@refs/tags/v0.1.0`
and `--source-tag v0.1.0` instead of `--source-branch main`. The main identity
is for Release Please and manual nightly callers. Verify both issuer and exact
identity; never accept arbitrary workflows using a broad identity regex. On
macOS, `shasum -a 256 --check checksums.txt` replaces `sha256sum`.
Run the provenance command for each file you intend to consume. A fork must use
its own repository and workflow identities and can only prove that fork's build.

## Distribution channels

After `publish` succeeds for a stable tag, the `channels` job runs
`scripts/release-channels.sh publish`. It first re-validates GoReleaser's AUR
PKGBUILD/.SRCINFO against the verified checksums and the package-owned `aur`
stamp, and renders the Homebrew formula with `scripts/homebrew-formula.sh`. It
then pushes `Formula/kitbag.rb` to `TomaszJanusz/homebrew-tap` with
`HOMEBREW_TAP_TOKEN`, and `PKGBUILD`/`.SRCINFO` to `kitbag-bin` on the AUR over
SSH with `AUR_SSH_PRIVATE_KEY` and a pinned host key. A missing secret skips that
channel with a job-summary line instead of failing the release. Release Please
passes these two optional secrets explicitly; nightly runs pass none and skip
the job. The tap, token, AUR account and key are maintainer setup
([install.md](install.md#maintainer-setup)).

The `verify-linux`, `verify-fedora` and `verify-macos` jobs then install the
published release through install.sh (with cosign on macOS), the `.deb`, the
`.rpm` and, when it was updated, the tap. They cannot run before the first real
release; the Release snapshot workflow runs the same `scripts/package-check.sh`
checks against local snapshot payloads on every relevant PR.

## Nightly channel

**Nightly prerelease** is `workflow_dispatch` only and refuses any ref except
`main`. Its version is the next patch after the release manifest, with
`-nightly.<UTC timestamp>.<run number>.<attempt>`. The `nightly` release/tag is a
mutable channel alias; only this tag may move. Its metadata always says
`nightly`, publication explicitly sets `--prerelease --latest=false`, and an
existing non-prerelease with that name causes failure. It cannot write to a tap
or run a package publisher. Stable tags and releases are never modified by it.

Nightly archives, metadata and provenance replace assets on the alias release.
Treat that set as mutable: concurrent downloads during replacement may fail
verification and should be downloaded again as a complete set. The build's
exact source commit is recorded in provenance; the alias is not a trust anchor.
Old versioned archives may remain attached; only the current signed checksum
manifest describes the current nightly. Self-update's future nightly discovery
path is `/download/nightly/kitbag-update.json`; stable uses
`/latest/download/kitbag-update.json`.

To enable a schedule, the maintainer must approve a separate change adding, for
example, `schedule: [{cron: '17 3 * * *'}]` under `on` in
`release-nightly.yml`. Preserve its main-ref guard and prerelease-only path.
No schedule is enabled by this implementation.

## Hardening, rotation and reproducibility

Top-level permissions are empty. Build/sign gets contents-read and
id-token-write; the provenance call gets actions-read, contents-write and
id-token-write; the final publisher gets contents-write. Checkouts never
persist credentials. PR snapshots have contents-read only, no signing or
publication. Jobs have timeouts and release runs serialize without cancellation.
All ordinary third-party action pins were resolved to full commits via `gh api`.

GitHub [does not allow permission elevation in nested reusable workflows](https://docs.github.com/en/actions/reference/workflows-and-actions/reusing-workflow-configurations#access-and-permissions-for-nested-workflows).
Startup validation includes jobs that will be skipped. The SLSA generator's
[`v2.1.0` workflow](https://github.com/slsa-framework/slsa-github-generator/blob/v2.1.0/.github/workflows/generator_generic_slsa3.yml)
declares these permissions for every job (unspecified permissions are none):

- `detect-env`: id-token-write.
- `generator`: actions-read, contents-read, id-token-write.
- `upload-assets`: contents-write, even though `upload-assets: false` skips it.
- `final`: contents-read, inherited from the upstream workflow default.

It makes no further reusable workflow calls. Its minimum caller grant is
therefore actions-read, contents-write and id-token-write. The `provenance` call
must grant that union even though KitBag uploads provenance in its own `publish`
job. The Release Please `release` and Nightly prerelease `nightly` calls already
grant the same union: it covers `release.yml`'s build, provenance and publish
jobs and the nested generator. Neither caller needs issues or pull-requests
permissions; those remain confined to Release Please's separate `propose` job.

To validate GitHub startup without publishing, dispatch only **Release Please**
on a reviewed non-main branch. Its `propose` main-ref guard skips that job;
`release` depends on `propose` and requires its `created` output to be `true`, so
it is skipped too. The run must complete with skipped jobs rather than
`startup_failure`. This proves startup validation, not signing or publication.

One explicit exception is the SLSA generic generator at `v2.1.0`, resolved to
`f7dd8c54c2067bafc12ca7a55595d5ee9b75204a`. Its
[upstream contract](https://github.com/slsa-framework/slsa-github-generator/blob/v2.1.0/internal/builders/generic/README.md#referencing-the-slsa-generator)
requires an exact version tag; SHA references fail generator/verifier identity
checks. A single-line zizmor suppression documents this constraint. Local
workflow calls also retain `./` syntax because the pinned actionlint rejects
zizmor's newer `$/` suggestion; their narrowly scoped suppressions explain why.
The generator is no longer actively maintained upstream; migrating to GitHub
attestations would change the required verifier and needs a separate decision.

Keyless signing has no project private key to rotate. Changing repository,
workflow path or ref changes certificate identity: update verification guidance
and the future binary's trust policy through the existing trusted distribution
path. Review cosign/Sigstore trust-root and generator/verifier updates together;
never download a replacement trust anchor from the artifact being verified.

`CGO_ENABLED=0`, `-trimpath`, fixed version ldflags and commit `mod_timestamp`
make executable builds repeatable. Extra archive entries have normalized modes,
owner/group and commit timestamps. `mise run release:snapshot` checks the config,
builds and structurally verifies all archives and SBOMs, runs the Linux amd64
binary's bare version command, builds all four executables again and compares
SHA-256 hashes. SBOM generation timestamps are intentionally not compared.
This proves repeatability on the same runner/toolchain, not independent-host or
bit-for-bit SBOM reproducibility.

The snapshot check runs on main and release-related PR paths. Its follow-up
jobs install the snapshot's `.deb` and install.sh in Ubuntu containers, its
`.rpm` in Fedora, its PKGBUILD with makepkg in Arch Linux, and install.sh plus
the formula from a local tap on macOS ([details](install.md#how-channels-are-tested)). Locally use
`mise exec -- goreleaser release --snapshot --clean --parallelism 2` for the bare
GoReleaser dry run, or `mise run release:snapshot` for the complete check. Neither
signs nor publishes. `scripts/release-build.sh` adds discovery metadata and its
checksum; `scripts/release-verify.sh` validates the upload payload.

Keyless OIDC signing, GitHub release uploads, and SLSA verification require an
actual maintainer-authorized run. They cannot be claimed from a local snapshot.
The local throwaway-key cosign test proves blob signing and tamper rejection;
it does not prove Fulcio identity, Rekor or GitHub OIDC integration.
