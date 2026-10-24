# 9. Preserve skills lock interchange and separate immutable provenance

- Status: Proposed
- Date: 2026-10-24
- Origin: KIT-2

## Context and Problem Statement

KitBag plans to synchronize global agent configuration from Git. Imported skills
need reviewable source identities, immutable pins and content verification;
applying those skills also needs machine-specific ownership and drift state.
These are different lifecycles.

Plan §2.4/§9 use `skills-lock.json`; §19.2.6 later suggests an APM-compatible or
literal `apm.lock`. KIT-2 explicitly asks which direction to recommend. The
[accepted ADR index](README.md#planning-sources-and-revisions) and
[ADR 2](0002-git-source-of-truth.md) leave the lock schema undecided. Treat the
plan examples as provisional, not acceptance of exact bytes or semantics. This
proposal does not supersede an Accepted ADR or authorize implementation.

## Decision Drivers

- Reuse the existing skill ecosystem where the semantics actually match.
- Pin imported bytes independently of moving branches, tags or search results.
- Keep host paths, ownership, settings snapshots and secrets out of portable locks.
- Avoid implementing APM's package/deployment system as a prerequisite for sync.
- Fail visibly on unsupported versions or incomplete resolution evidence.

## Considered Options

1. Adopt APM's `apm.lock.yaml` (legacy `apm.lock`) as KitBag's primary lock.
2. Treat `skills-lock.json` v1 alone as a complete frozen-install contract.
3. Preserve a limited `skills-lock.json` v1 interchange surface, with separately
   versioned KitBag provenance and machine-local application state.

## Proposed Outcome

Recommend **option 3**. Retain compatibility with the project-scoped
`skills-lock.json` **version 1** for exchanging individual skill records. Do not
adopt or relabel APM's lock as KitBag's lock. Do not use Vercel's global
`.skill-lock.json` version 3 as the portable file.

Compatibility here is a future read/export goal for an explicitly supported
subset, not a current implementation, byte-identical serialization, lossless
APM conversion or proven CLI round trip. The exact schema/file name for KitBag
provenance remains follow-up design, not a new public format specified here.

The boundary should be:

- **Intent:** `kitbag.toml` declares sources and target policy; a lock does not
  silently change what is requested. KIT-4 defines this manifest contract.
- **Interchange:** a skills v1 record retains the upstream meaning of `source`,
  `sourceType`, optional URL/ref/path and `computedHash`. Export only records
  representable without loss; report unsupported source kinds or collisions.
  Recompute the exact upstream folder hash when exporting. Do not put an APM
  digest or Git tree SHA into `computedHash`.
- **Resolution:** a separate committed KitBag provenance contract must bind
  canonical source, selected subpath, requested selector, exact resolved commit
  or immutable artifact identity, and a named content-hash algorithm/version.
  Link that evidence unambiguously to the exchanged skill and detect stale or
  conflicting evidence. Until it exists, a skills v1 record alone must not be
  presented as sufficient for frozen installation. Do not hide required pins in
  ad hoc extra skills fields whose preservation by other writers is unproven.
- **Application:** keep destination paths, last-applied hashes, ownership and
  backup/revert journals in local state, following
  [ADR 3](0003-materialization-and-platform.md). Source integrity is not authority
  to overwrite an edited destination. Never export local settings snapshots or
  credentials with a portable skill lock.

APM-compatible source notation may be supported as a separate, explicitly
bounded input adapter. Consuming an APM package or its locked transitive graph
requires additional resolution/interpretation work; the familiar shorthand is
not that implementation. Optional delegation to an installed APM remains a
separate decision, with no dependency introduced by this ADR.

## Evidence and Tradeoffs

The [prior-art review](../prior-art.md) pins the inspected source and documents
these differences:

- APM 0.35.0's YAML lock holds dependency graphs, exact commits, content and
  deployment hashes, MCP baselines and deployment ownership. Its ordinary
  versions are strings `"1"`/`"2"`, with a candidate `"3"` for replacements.
  Adopting it entails a larger evolving contract, including scope and ownership
  semantics that do not fit a portable skill exchange file automatically.
- skills 1.7.4's project JSON v1 maps skill names to source records and folder
  `computedHash`. It has no dedicated resolved-commit field. Its hash excludes
  `.git`/`node_modules`, uses normalized relative paths sorted by JavaScript
  `localeCompare`, and raw content bytes. APM's exclusions, ordering and digest
  envelope differ; its deployed-file newline normalization differs again.
- The skills global v3 folder hash is a GitHub tree SHA, with local timestamps
  and UI state. It is neither the project v1 hash nor an APM content hash.
- APM rejects unknown versions/malformed locks; the skills readers can return
  empty locks for malformed/old data and accept higher numeric versions without
  explicit negotiation. KitBag must define its own fail-closed reader policy.

This adds a consistency burden between interchange and provenance, but keeps
upstream skill exchange possible without pretending that its current lock
solves immutable replay. A single KitBag-only format would be simpler to own
but would discard the plan's interoperability objective. APM's lock is stronger
for APM package replay; that is a reason to preserve an integration boundary,
not to claim it is an inadequate package-manager lock.

## Conditions Before Acceptance and Implementation

1. Define the supported skills v1 fields/source kinds and same-name collision
   policy, plus explicit loss/error reporting for imports and exports.
2. Specify KitBag provenance identity, consistency checks, immutable resolution,
   offline/unavailable-source behavior and validation before any deployment.
3. Define separate schema versions and migrations for manifest, provenance and
   local state. KIT-4's manifest N/N−1 rule does not automatically apply to an
   upstream lock. Reject unsupported versions without empty-lock fallback or
   destructive rewrite; preserve original files during explicit migrations.
4. Verify fixture-based read/export and actual skills CLI update round trips at
   a pinned release, including optional/unknown fields. Test hash vectors for
   CRLF, Unicode names/order, ignored directories and platform differences.
   Do not promise cross-platform digest equality before those tests.
5. Settle the local CLI's supported skills.sh API/authentication path and keep
   discovery failures independent of already pinned, locally available content.

No parser, schema, dependency adoption or runtime experiment is included in
KIT-2. This proposal remains open for architecture review.

## Links

- [KIT-2 prior-art spike](https://linear.app/privacybrand/issue/KIT-2/spike-prior-art-review-apm-dallayagentsync-dotagents-agentfiles)
- [KIT-4 manifest contract](https://linear.app/privacybrand/issue/KIT-4/manifest-schema-v1-kitbagtoml-and-json-schema)
- [Prior art and pinned upstream evidence](../prior-art.md)
- [Planning source](https://app.notion.com/p/40319f2ad838818587aad3205e6928d3), §2.4, §9, §19.2.6
