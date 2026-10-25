# Minimum versions and verified updates

`min_tool_version` is a semantic version. A local manifest requiring a newer
KitBag blocks sync, plan, status, adopt, unmanage, revert (including `--list`),
and doctor with exit 4 before creating state directories, taking locks,
recovering a journal, fetching, moving targets or saving state. Doctor also
suppresses `--report` writes on this failure. Development builds compare as
`0.0.0-dev`. Revert remains available with a missing or damaged manifest, but
cannot bypass a readable manifest's minimum version.

Status prints `tool too old`, the required version and the running version;
the text is red when colour is enabled and honours `NO_COLOR`. Status JSON adds
`tool_too_old` with `type: "tool-too-old"`, `required`, `current`, and
`repository`. Core workflow and doctor accept an optional `Emit` callback for
the same typed `compatibility.Event`. This transient event can feed a tray or
API without changing the host. There is no event file or API dependency.

The local check runs before any writes. If the minimum first arrives through
a clone or pull, the newly acquired manifest is checked again before recovery
or apply. That source acquisition has necessarily changed Git's working copy
and metadata; sync retains its existing explicit last-sync failure record
(exit 4, zero applied changes). Existing target/state contents are never
recovered or applied from that incompatible tree. A manifest already known to
be incompatible does not even update the last-sync record. The testscript
`min_tool_version.txtar` compares complete HOME tree snapshots, including
contents, modes, links and file modification times, with both absent and
existing state. `sync.txtar` covers a minimum introduced by a pull.

## Install-source stamp

Installers write a regular UTF-8 file named `kitbag.install-source` next to
the actual executable, with exactly one source value and an optional trailing
newline:

```text
homebrew
```

Values: `homebrew`, `deb`, `rpm`, `aur`, `install-script`, `release-archive`,
`go-install`. A missing or unrecognized stamp means `unknown`; an unreadable
stamp is an error. Stamp reading uses `platform.FS`. Executable discovery
resolves installer symlinks before selecting the sibling stamp. KIT-39/40 own
stamp writers and packaging; this change defines the reader and format.
Never add a direct-install stamp to a package-owned executable to bypass its
package manager.

`kitbag self-update` prints the owning tool's command and does no network or
filesystem writes for package sources:

| Source | Guidance |
| --- | --- |
| homebrew | `brew upgrade kitbag` |
| deb | `sudo apt-get update && sudo apt-get install --only-upgrade kitbag` |
| rpm | `sudo dnf upgrade kitbag` |
| aur | Use the AUR helper, for example `yay -Syu kitbag` |
| go-install | `go install github.com/TomaszJanusz/kitbag/cmd/kitbag@latest` |
| unknown | Reinstall through the original installer |

## Direct installation protocol

Only `install-script` and `release-archive` permit replacement. `--check` and
`--dry-run` verify the entire available release without writing or executing
it. `self-update` works without machine configuration or Git. It never requests
privilege escalation; an unwritable binary directory fails normally.

The default base URL is
`https://github.com/TomaszJanusz/kitbag/releases`.
`KITBAG_RELEASE_BASE_URL` selects a mirror or local test server, without
changing signature trust. HTTPS is required except for loopback HTTP. Redirects
follow the same rule. Requests have a two-minute timeout, context cancellation,
a ten-redirect limit and response size limits.

`--channel stable` (default) reads `<base>/latest/download/kitbag-update.json`.
`--channel nightly` reads `<base>/download/nightly/kitbag-update.json`.
Metadata is a small JSON object, for example:

```json
{"version":"1.2.3","channel":"stable"}
```

Stable rejects prereleases; nightly requires a `nightly.` prerelease such as
`1.3.0-nightly.20261025`. Other channels fail. The selected download directory
must also contain:

- `kitbag_<version>_<GOOS>_<GOARCH>.tar.gz`, with one regular root entry `kitbag`.
  Other entries are skipped, never extracted to disk; links cannot supply the
  executable. Expanded executable size is limited to 128 MiB, download to 64 MiB.
- `checksums.txt`, SHA-256 lines of the form `<hex>  <archive filename>`.
  The selected archive must have exactly one valid checksum line.
- `checksums.txt.sig`, the base64 detached cosign signature over the exact
  checksum file bytes.

The archive name binds the version, platform and channel-specific prerelease
to signed checksums. Untrusted discovery metadata cannot select an unsigned
archive or rename one across channels. An older advertised version is an
error; no channel switch silently downgrades. Matching versions are verified
but not installed. Replay of a still-newer signed release is possible; this
protocol authenticates releases, without asserting transparency-log freshness.

## Signature trust and release integration

`internal/releasesig` implements minimal offline verification using Go's
standard-library ECDSA P-256 / SHA-256 / ASN.1 verifier and a pinned PEM
SubjectPublicKeyInfo public key. This is the detached key-based cosign blob
format, not keyless certificate verification. See the upstream
[cosign sign-blob reference](https://github.com/sigstore/cosign/blob/main/doc/cosign_sign-blob.md)
and [self-managed keys](https://docs.sigstore.dev/cosign/key_management/overview/).
The release signer must produce the detached signature (`--output-signature`)
over `checksums.txt`, using its managed P-256 key and SHA-256. No cosign
subprocess, downloaded public key, certificate, online trust service or new Go
module is needed for verification. Go and cosign tooling remain pinned in
`mise.toml`. Depguard confines production HTTP to `internal/releasehttp` and
ECDSA verification to `internal/releasesig`.

**Production trust is not provisioned in this development repository.**
KIT-39 must pin the maintainer's public key in `cmd/kitbag/self_update.go`
(`releasePublicKey`) before publishing an updater-enabled release, and emit
the metadata and detached artifacts above. Its current empty value fails
closed. No runtime flag/environment variable can replace this trust anchor.
This task creates no production key, release or signature. Tests generate
fixture-only keys in memory. Key rotation needs a reviewed release containing
the replacement public key, distributed through the existing trusted path.

## Replacement and recovery

Core policy injects HTTP, signature verification, version probing and re-exec.
It uses `platform.FS.WriteAtomic` to stage, chmod, fsync, rename over the running
Unix binary, and sync the parent directory. The live executable name is never
removed first. An update lock serializes writers; its reusable `.update.lock`
file remains next to the binary. Provenance and the currently installed executable version are rechecked under
that lock, so a concurrent newer installation cannot be downgraded.

The original is preserved as `<binary>.old` before replacement. The new binary
must successfully print exactly `kitbag <advertised version>` through
`version --bare` before `.old` is removed. `--bare` avoids loading local config
or invoking Git during confirmation. A failed confirmation rolls back with
another atomic replacement. If rollback fails, `.old` remains for manual
recovery; the error says so. A pre-existing `.old` causes refusal, preserving
interrupted-update evidence. After confirmation the process re-execs
`kitbag version`. A re-exec failure leaves the confirmed new binary installed
and returns an error. Windows runtime support remains deferred.

Tests use in-memory platform faults, signed fixture releases on `httptest`,
and a real temp-directory replacement of a copied test executable, including
execution of that replacement before backup cleanup. Re-exec is injected so
tests never replace the test runner process. Production signing, packaging,
public release compatibility, and actual process replacement are not exercised.
