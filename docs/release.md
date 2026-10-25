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
