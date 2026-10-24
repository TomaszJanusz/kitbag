# 6. Distribute v1 through macOS and Linux channels

- Status: Accepted
- Date: 2026-10-24

## Context and Problem Statement

A public tool needs installation paths that fit its target users without creating
an unbounded set of packaging systems to maintain. The revised v1 scope is macOS
and Linux; Windows distribution follows its compatibility layer.

## Decision Drivers

- Easy installation of prebuilt binaries.
- Verifiable artifacts and consistent release production.
- Respect package manager ownership during updates.
- Keep v1 packaging effort focused on supported platforms.

## Considered Options

- GitHub Releases plus established macOS/Linux package channels.
- Add Windows package managers to v1.
- Add an npm binary wrapper immediately.
- Support every available package manager.

## Decision Outcome

Use GoReleaser to produce GitHub Releases with archives, checksums, cosign
signatures, SBOMs, and SLSA provenance. The v1 channels are a Homebrew tap
(formula for CLI and cask for tray), a checksum-verifying `install.sh`, deb/rpm
packages, and AUR. Direct release downloads and `go install` remain available
paths. These are planned channels, not a claim that artifacts already exist.

Defer Windows installation and winget to M6. Exclude Scoop. Park the optional npm
wrapper until after v1 and a separate decision: downloading a prebuilt binary
through npm may aid discovery, but adds account, pipeline, and supply-chain
maintenance. Community Nix packaging or an apt repository can follow demand;
they are not v1 commitments.

Package-installed binaries update through their package manager. Direct release
or installer installations can use verified, atomic self-update. Do not silently
replace a package-owned binary.

Following the maintainer's §0 revision, Apple Developer and Azure signing accounts
are deferred. The macOS tray and future Windows binaries initially ship without
platform code signing, with documented first-run caveats. This is distinct from
release artifact signing with cosign; platform signing is not a release blocker.

### Consequences

- Release automation and installation verification are follow-up implementation,
  not part of this documentation change.
- The initial package matrix follows actual platform support.
- Unsigned desktop binaries need clear user-facing installation guidance.
- Additional channels require explicit maintenance ownership.

## Links

- Planning source: plan §0, §5, §14, and §19.2 item 9; see the
  [source and revision notes](README.md#planning-sources-and-revisions).
