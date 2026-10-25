# Prior-art review

Research date: **2026-10-24**. KIT-2; plan §9 and §19. This is a review of
upstream documentation and source, not runtime compatibility testing. “Take”
means a design recommendation for planned work, not an implemented feature or
an added dependency. Agent-specific contracts remain in [the agent inventory](agents.md).

## Evidence and planning precedence

The source snapshots below were resolved through GitHub's API and the selected
files fetched at those exact commits; no third-party scripts were executed.
Package versions are repository metadata, not proof of a tested release.

- APM: `2920043258932f0f67e6f01df7b9047eb42f358c` (2026-10-23; [package 0.35.0][apm-version]).
- agentsync: `3bead1e5f0f2c769dab9e7429aae09aef13e2159` (2026-10-24).
- dotagents: `7193a940e9b1ab01ab256f3465a0c96830e12b25` (2026-02-04).
- agentfiles: `b2627e84b4c512430759f55c8d7ac5b54bc368b3` (2026-08-31).
- skills: `a80e4c9d12d607631249d4d7d059f835d4b2f13a` (2026-10-19; [package 1.7.4][skills-version]).
- chezmoi: `2871e65d104188da02da227b68f52a0e9a3b25e6` (2026-10-23; `master` snapshot).

[Plan §9][plan] favors the skills CLI lock; §19.2.6 later favors APM-compatible
sources and a compatible or literal APM lock. Neither settles an exact schema:
[KIT-2][kit2] explicitly commissions that recommendation, and
[ADR 2](adr/0002-git-source-of-truth.md) leaves it open. Treat those earlier
examples as provisional inputs. The new [ADR 9](adr/0009-skill-lock-interchange.md)
is **Accepted**, supersedes no Accepted ADR, and does not silently accept either
plan paragraph as a complete implementation contract.

## 1. microsoft/apm

APM manages agent packages, resolution, deployment and audit. Its current
manifest reference covers `apm.yml` metadata, `dependencies.apm`,
`dependencies.mcp`, `dependencies.lsp`, target selection, compilation, security
and policy. Dependencies can be transitive. This is broader than installing
individual skills. See [manifest reference][apm-manifest] and [parser][apm-parser].

**Source references.** GitHub shorthand is
`owner/repo[/subpath][#ref]`; an optional `host[:port]/` selects another host.
`#ref` denotes a branch, tag or commit, not a content checksum. The parser also
accepts HTTPS/HTTP clone URLs, SCP-style `git@host:path`, `ssh://` URLs, and
local paths (`./`, `../`, absolute and home-relative paths, including Windows
forms). Explicit clone URLs with embedded virtual subpaths are rejected; use
an object with `git`, `path`, and `ref`. That object also resolves ambiguity
between nested GitLab groups and package subpaths. Object `ref` supports
semver ranges; resolution records the chosen tag and commit. HTTP has separate
insecure-host opt-ins. This is not one universal URL grammar that KitBag can
claim to support merely by accepting `owner/repo/path#ref`.

**Lock.** The current name is `apm.lock.yaml`; `apm.lock` is legacy. The reader
prefers the new file and can read the old name; mutation migrates the old name
when the new one is absent. YAML contains `lockfile_version` as a string and a
`dependencies` list. Entries include `repo_url`, `resolved_commit`, optional
`resolved_ref`, `virtual_path`, parent/depth, package kind and selected skills
or targets. Deployment paths, per-file hashes, MCP baselines and canonical
`deployments` ownership rows give it responsibilities beyond resolution.
[Lock reference][apm-lock-doc], [reader/writer][apm-lock-code].

Ordinary version `"1"` covers Git-only graphs; `"2"` adds registry or Git semver
evidence. The source accepts `"3"` for a root-source-replacement candidate; the
docs explicitly call it unratified with no first supported release assigned.
Do not conflate CLI version 0.35.0, OpenAPM schema revision 0.1.46, and
`lockfile_version`. The [0.1.46 schema][apm-schema] permits `"1"`/`"2"`, whereas
the working-draft CLI has additional candidate behavior. Unknown explicit
versions and malformed locks fail closed; pre-versioned legacy files can be
read as v1. Additive fields can appear without a version bump, so a version
number alone does not define an interoperable writer.

**Integrity.** `resolved_commit` pins Git history; `resolved_hash` verifies a
registry download. `content_hash` fingerprints a package tree: SHA-256 over
sorted POSIX relative paths followed by raw file bytes, prefixed `sha256:`.
It excludes `.git`, `__pycache__`, root `.apm-pin`, and symlinks; it does not
include permissions. Per-file hashes omit the path and normalize CRLF to LF
only for UTF-8 text without NULs, preserving bare CR. Native bundle hashing
has a separate contract including executable bits. These hashes are not
interchangeable, and no hash proves that instructions are safe.
[Hash implementations][apm-hash].

**Security and audit.** The scanner marks Unicode tag characters, bidi
embeddings/overrides/isolates and supplementary variation selectors critical;
zero-width characters generally warn, with context-sensitive exceptions such
as emoji ZWJ and a leading BOM. This is targeted content inspection, not a
complete prompt-injection or executable-safety detector. Audit scans recognized
primitive surfaces, including untracked prompt content; hash checks need a
recorded baseline. Whole-project drift replays installation in scratch space.
Bare audit is cache-only and can skip replay on a cache miss; drift is advisory
unless configured to fail. `audit --ci` hydrates a lock-pinned scratch replay
and fails on checks/drift; missing gitignored outputs still limit deployed-byte
coverage. `--file` and package-scoped scans skip whole-project drift.
[Scanner][apm-scanner], [scope implementation][apm-file-scan], [audit contract][apm-audit].

**Take:** explicit source coordinates, immutable resolution evidence, distinct
source/deployed hashes, coverage-aware Unicode reports, and scratch replay as
an audit model. Preserve the distinction between ownership and matching bytes.

**Do not take:** APM's entire resolver, packaging/marketplace policy system or
its project deployment ledger as KitBag's portable skill lock. APM now documents
global audit/install too; positioning must not claim it is exclusively
project-scoped or lacks all global configuration support. KitBag's intended
focus remains one user's Git-backed configuration across machines, publication
review and a shared daemon/UI, rather than package-management breadth.

## 2. dallay/agentsync

The default configuration is `.agents/agentsync.toml`. `source_dir = "."` is
relative to that configuration; `[agents.<name>]` enables agents and
`[agents.<name>.targets.<primitive>]` specifies `source`, `destination`, and
`type`. Whole-directory `symlink` and per-child `symlink-contents` have distinct
ownership/container behavior. `default_agents` is a filter, not a new source
layer. A user config can supply an initialization template; it replaces the
built-in template rather than merging with it. [Configuration][sync-config],
[configuration model][sync-config-code].

`[mcp]` enables rendering with `merge` (default) or `overwrite`;
`[mcp_servers.<name>]` defines command/args/env or URL/headers/transport.
Only configured, enabled and selected agents receive output. Merge preserves
unmentioned server names, with TOML winning same-name conflicts. Formatters
handle Claude/Cursor JSON, VS Code's shared Copilot destination, Codex TOML
(`headers` → `http_headers`), Gemini JSON (`trust: true`), and OpenCode's own
shape, among others. These are per-client conversions, not generic JSON fan-out.
[MCP guide][sync-mcp], [formatters][sync-mcp-code].

Revert distinguishes symlink `.bak` backups from MCP's machine-local ownership
journal. MCP apply snapshots the original whole file and hashes the written
state. Revert restores/removes only when that state still matches and all owners
of a shared destination are selected; subsequent edits, symlinks or invalid
journal evidence preserve the file and journal. Legacy installs without a
journal cannot reconstruct the original. Rebase is explicit and retains the
pre-first-apply snapshot, so later revert can discard intervening edits.
Snapshots may contain credentials and need private local storage; they do not
belong in a committed dependency lock. The documented external-editor race at
the final compare/publish boundary also prevents an absolute atomicity claim.
[MCP recovery][sync-mcp], [journal][sync-journal], [symlink revert][sync-revert].

[Windows guidance][sync-windows] distinguishes native Developer Mode or elevated
symlink permission from WSL, recommends one execution context per checkout and
WSL-local storage for WSL. WSL is optional, not proof that native Windows works.

**Take:** typed targets, explicit renderer selection, preservation of unrelated
settings, backup-aware revert, protected local journals and concrete Windows
diagnostics. Validate per-client semantics against [KIT-1](agents.md).

**Do not take:** mandatory `.agents/` repository layout, automatic project
`.gitignore` management or checkout hooks as KitBag's global sync model; do not
copy Gemini trust elevation as a universal default. agentsync also writes global
Claude Desktop MCP and now has repository-owned plugin materialization, so
“only symlinks” or “never writes outside a project” would be inaccurate.

## 3. iannuttall/dotagents

Global `.agents/CLAUDE.md` overrides `.agents/AGENTS.md` for Claude's destination;
`.agents/GEMINI.md` similarly overrides the common file for Gemini. This selects
one source, not concatenation or runtime agent instruction precedence.
Factory/Codex use `AGENTS.md`. Changing override presence requires rerunning
apply/repair. Project scope maps commands/hooks/skills only and leaves root
instruction files alone. [README][dot-readme], [mapping implementation][dot-map].

Commands link to Claude/Factory/Cursor/Gemini command folders, Codex `prompts`,
and OpenCode `commands` in this snapshot. Hooks link only to Claude and Factory
hook directories. Path mapping does not translate command frontmatter, create
settings hook registrations or establish agent support. In particular, use
KIT-1's current client contracts rather than treating this older mapping as a
compatibility matrix. Backups live under `.agents/backup/<timestamp>`; undo loads
the latest timestamped backup manifest, backs up existing destinations and restores the
recorded entries. [Backup][dot-backup], [undo][dot-undo].

**Take:** explicit per-agent override then common fallback, repair after source
selection changes, and inspectable undo history.

**Do not take:** blind directory equivalence across command formats, hook
activation by file placement, or a claim that all agents share instruction
precedence. KitBag still needs rendering/registration and host additions defined
by KIT-4; these are not supplied by dotagents' symlink mapping.

## 4. Railly/agentfiles

Agentfiles is an Obsidian file browser/editor with optional analytics. Its
[README path table][files-readme] and [tool registry][files-paths] distinguish
skills, commands, rules/memories and subagents. Examples include Claude
`~/.claude/{skills,commands,agents}`, Codex skills/prompts/memories/agents,
Windsurf's `.codeium/windsurf` paths plus `.windsurf/rules`, and XDG-based
OpenCode/Amp/Goose paths. Cline, Claude Desktop and Aider include detection-only
entries; the shared `.agents/skills` root is another registry entry. The table
is a discovery seed, not evidence that every listed primitive is loaded by the
latest agent on every OS. KIT-1's source-backed inventory is authoritative for
KitBag adapter planning.

The [marketplace implementation][files-market] resolves a repository default
branch to one commit, fetches that commit's recursive tree and rejects truncated
trees. Skill selection must resolve to exactly one directory. It allows at most
**200 blob entries** and **10 × 1024 × 1024 bytes (10 MiB)** per skill, checks
both declared total sizes and actual downloaded bytes, and resolves each output
path inside a temporary root. Raw downloads use the selected commit. These are
not archive extraction limits: this path downloads individual blobs. The byte
limit is checked after each response is buffered, so it is not a strict bound
on peak network/memory consumption. Blob selection does not establish a blanket
“rejects Git symlinks” guarantee; do not promote path containment into one.

The [dashboard][files-dashboard] consumes Skillkit context data with separate
always-loaded instruction, skill metadata and memory token totals, plus session
cost/cache estimates. This snapshot does not establish a tokenizer contract or
prove what each agent actually loaded. Context tax is an estimate of recurring
input burden, not all skill-body tokens or measured model billing.

**Take:** primitive-specific inventory, exact source pinning, bounded imports,
containment checks and clearly labeled context estimates per agent/host.

**Do not take:** Obsidian dependence, conversation-history browsing, usage billing
analytics, discovery-as-installation proof, or treating marketplace popularity
and these download checks as a security certification.

## 5. chezmoi

The [concept model][chez-concepts] separates source state (possibly templates),
computed target state and actual destination state. The three-way
[`merge`][chez-merge] uses those states; its source is not simply KitBag's last
applied bytes. [`apply`][chez-apply] detects destination edits since the last
write and prompts before overwriting. The [design explanation][chez-design]
describes generating regular destination files rather than requiring links back
to the source, while still supporting symlinks explicitly.

**Take:** separate desired and observed state, preview/diff, explicit handling
of local changes and copy materialization. KitBag's accepted
[ADR 3](adr/0003-materialization-and-platform.md) requires comparing source,
last-applied state and current destination: safe unchanged destinations can be
updated, locally changed destinations report drift, explicit force can discard
it, and adoption can bring it back to the repository. Hashes detect divergence;
actual three-way text merging would additionally need baseline content. Do not
claim hashes alone implement merging.

**Do not take:** chezmoi's source filename encoding, general template/script
system or its state database as an agent-aware manifest. Its source/target/
destination merge is inspiration, not byte-for-byte semantics for KitBag copy
mode. Agent settings merges and hook ownership still need separate contracts.

## 6. Vercel skills CLI and skills.sh

The CLI's [source parser][skills-parser] accepts local paths, GitHub
`owner/repo[/subpath]`, `owner/repo@skill`, GitHub/GitLab prefixes and tree URLs,
Git clone URLs, Azure Repos URLs, well-known providers and hosted downloads.
Git-like inputs support `#ref` and `#ref@skill`; fragments are not interpreted
as Git refs for arbitrary provider URLs. Selection by skill name and selection
by repository path are different operations. This overlaps APM shorthand but
does not make their complete parsers equivalent. Installation and search are
skill-focused; they do not supply KitBag's full configuration sync policy.
[CLI overview][skills-readme].

**Two different locks.** The committed project file is `skills-lock.json`,
`version: 1`, with `skills` keyed by skill name. Required entry fields in the
TypeScript interface are `source`, `sourceType`, and `computedHash`; optional
fields include `sourceUrl`, `ref`, `skillPath`, npm `version`/`via`, subagent
placement and `wellKnownDigest`. `computedHash` is bare hex SHA-256: recursively
collect regular files, exclude `.git` and `node_modules` directories, normalize
path separators to `/`, sort with JavaScript `localeCompare`, then hash each
path followed by raw content. Symlink entries are not regular files in this
walk; executable bits and timestamps are absent. Writer sorts skill names and
omits timestamps. There is no dedicated resolved-commit field and a ref can
move. [Local lock implementation][skills-local].

The global file is `.skill-lock.json` under `$XDG_STATE_HOME/skills` when set,
otherwise `~/.agents`; its current version is **3**. `skillFolderHash` is a
GitHub folder tree SHA fetched through the Trees API, not the local SHA-256;
entries also carry source URL/path/ref and install/update timestamps, with
prompt dismissals and last-selected agents at top level. Do not confuse either
file with the other or commit machine UI state as a portable skill lock.
[Global lock implementation][skills-global].

Both readers return an empty lock for malformed input or versions below their
current version, and do not explicitly reject a higher numeric version. This
is not a fail-closed version negotiation contract for KitBag. Extra data surviving
one read/write is not a guarantee that all future CLI update paths preserve it.
Name-keyed entries also cannot independently identify two same-named skills
from different sources without a collision policy.

**Index API.** The pinned [CLI search client][skills-find] calls
`GET https://skills.sh/api/search?q=...&limit=20` (optional `owner`) and expects
`skills[]` with `id`, `name`, `source`, `installs`. A public read on the research
date with `q=react&limit=2` returned that shape plus query/count metadata.
However, the current [official API docs][skills-api] describe authenticated
`/api/v1/skills/search` returning `data[]`, with Vercel OIDC and a 600/minute
team/project limit. The docs also cover catalog pagination and skill detail/audit
endpoints. This mismatch needs a client boundary; the legacy endpoint is not a
promised unauthenticated public contract. No authenticated API call was tested.
Neither search shape pins the bytes to install. A detail-response `hash` is
documented for change detection, without enough canonicalization detail here
to equate it to either CLI lock hash. The [site's security guidance][skills-docs]
recommends reviewing skills and does not guarantee every listing's security.

**Take:** skills.sh discovery and a deliberately scoped `skills-lock.json` v1
exchange path, with named hash semantics, source review and independent pins.

**Do not take:** global UI lock state, permissive error recovery, assumed API
stability, or popularity/audit labels as installation authorization. Do not
assume either lock proves frozen, identical cross-platform replay.

## Recommendation and remaining work

Prefer **skills-lock.json v1 compatibility for skill exchange**, not adoption of
APM's lock as KitBag's lock. Keep immutable source provenance under KitBag's
own separately versioned contract and machine application state local. This is
a proposed direction, not a third format already specified or compatibility
already delivered. See [ADR 9](adr/0009-skill-lock-interchange.md) for boundaries,
alternatives and acceptance conditions.

The important hash differences are substantive: APM excludes `__pycache__` and
root `.apm-pin`, skills excludes `node_modules`; their ordering contracts differ;
APM prefixes digests; APM deployed-file hashes normalize some newlines; Git tree
SHAs encode Git objects rather than either custom stream. Conversion must
recompute the destination algorithm or retain explicitly labeled evidence. A
renamed field is not conversion.

Before implementation: KIT-4 should distinguish source intent, source resolution,
and machine ownership; define collisions and supported reference subsets; and
avoid promising arbitrary APM package/transitive semantics. Follow-up skill work
must settle exact provenance schema, migration/version policy, cross-platform
hash vectors, CLI update round trips, immutable-ref restoration, authenticated
index availability for a local CLI, and unavailable-source/offline behavior.
None was exercised at runtime here. No installed agent configs were inspected
or changed. No parser/schema/lock implementation is included.

[plan]: https://app.notion.com/p/40319f2ad838818587aad3205e6928d3
[kit2]: https://linear.app/privacybrand/issue/KIT-2/spike-prior-art-review-apm-dallayagentsync-dotagents-agentfiles
[apm-manifest]: https://github.com/microsoft/apm/blob/2920043258932f0f67e6f01df7b9047eb42f358c/docs/src/content/docs/reference/manifest-schema.md
[apm-parser]: https://github.com/microsoft/apm/blob/2920043258932f0f67e6f01df7b9047eb42f358c/src/apm_cli/models/dependency/reference.py
[apm-lock-doc]: https://github.com/microsoft/apm/blob/2920043258932f0f67e6f01df7b9047eb42f358c/docs/src/content/docs/reference/lockfile-spec.md
[apm-lock-code]: https://github.com/microsoft/apm/blob/2920043258932f0f67e6f01df7b9047eb42f358c/src/apm_cli/deps/lockfile.py
[apm-schema]: https://github.com/microsoft/apm/blob/2920043258932f0f67e6f01df7b9047eb42f358c/docs/public/specs/schemas/lockfile-v0.1.46.schema.json
[apm-hash]: https://github.com/microsoft/apm/blob/2920043258932f0f67e6f01df7b9047eb42f358c/src/apm_cli/utils/content_hash.py
[apm-scanner]: https://github.com/microsoft/apm/blob/2920043258932f0f67e6f01df7b9047eb42f358c/src/apm_cli/security/content_scanner.py
[apm-file-scan]: https://github.com/microsoft/apm/blob/2920043258932f0f67e6f01df7b9047eb42f358c/src/apm_cli/security/file_scanner.py
[apm-audit]: https://github.com/microsoft/apm/blob/2920043258932f0f67e6f01df7b9047eb42f358c/docs/src/content/docs/reference/cli/audit.md
[sync-config]: https://github.com/dallay/agentsync/blob/3bead1e5f0f2c769dab9e7429aae09aef13e2159/website/docs/src/content/docs/reference/configuration.mdx
[sync-config-code]: https://github.com/dallay/agentsync/blob/3bead1e5f0f2c769dab9e7429aae09aef13e2159/src/config.rs
[sync-mcp]: https://github.com/dallay/agentsync/blob/3bead1e5f0f2c769dab9e7429aae09aef13e2159/website/docs/src/content/docs/guides/mcp.mdx
[sync-mcp-code]: https://github.com/dallay/agentsync/blob/3bead1e5f0f2c769dab9e7429aae09aef13e2159/src/mcp.rs
[sync-journal]: https://github.com/dallay/agentsync/blob/3bead1e5f0f2c769dab9e7429aae09aef13e2159/src/mcp_ownership.rs
[sync-revert]: https://github.com/dallay/agentsync/blob/3bead1e5f0f2c769dab9e7429aae09aef13e2159/src/linker/revert.rs
[sync-windows]: https://github.com/dallay/agentsync/blob/3bead1e5f0f2c769dab9e7429aae09aef13e2159/website/docs/src/content/docs/guides/windows-symlink-setup.mdx
[dot-readme]: https://github.com/iannuttall/dotagents/blob/7193a940e9b1ab01ab256f3465a0c96830e12b25/README.md
[dot-map]: https://github.com/iannuttall/dotagents/blob/7193a940e9b1ab01ab256f3465a0c96830e12b25/src/core/mappings.ts
[dot-backup]: https://github.com/iannuttall/dotagents/blob/7193a940e9b1ab01ab256f3465a0c96830e12b25/src/core/backup.ts
[dot-undo]: https://github.com/iannuttall/dotagents/blob/7193a940e9b1ab01ab256f3465a0c96830e12b25/src/core/undo.ts
[files-readme]: https://github.com/Railly/agentfiles/blob/b2627e84b4c512430759f55c8d7ac5b54bc368b3/README.md
[files-paths]: https://github.com/Railly/agentfiles/blob/b2627e84b4c512430759f55c8d7ac5b54bc368b3/src/tool-configs.ts
[files-market]: https://github.com/Railly/agentfiles/blob/b2627e84b4c512430759f55c8d7ac5b54bc368b3/src/marketplace.ts
[files-dashboard]: https://github.com/Railly/agentfiles/blob/b2627e84b4c512430759f55c8d7ac5b54bc368b3/src/views/dashboard.ts
[chez-concepts]: https://github.com/twpayne/chezmoi/blob/2871e65d104188da02da227b68f52a0e9a3b25e6/assets/chezmoi.io/docs/reference/concepts.md
[chez-merge]: https://github.com/twpayne/chezmoi/blob/2871e65d104188da02da227b68f52a0e9a3b25e6/assets/chezmoi.io/docs/reference/commands/merge.md
[chez-apply]: https://github.com/twpayne/chezmoi/blob/2871e65d104188da02da227b68f52a0e9a3b25e6/assets/chezmoi.io/docs/reference/commands/apply.md
[chez-design]: https://github.com/twpayne/chezmoi/blob/2871e65d104188da02da227b68f52a0e9a3b25e6/assets/chezmoi.io/docs/user-guide/frequently-asked-questions/design.md
[skills-parser]: https://github.com/vercel-labs/skills/blob/a80e4c9d12d607631249d4d7d059f835d4b2f13a/src/source-parser.ts
[skills-readme]: https://github.com/vercel-labs/skills/blob/a80e4c9d12d607631249d4d7d059f835d4b2f13a/README.md
[skills-local]: https://github.com/vercel-labs/skills/blob/a80e4c9d12d607631249d4d7d059f835d4b2f13a/src/local-lock.ts
[skills-global]: https://github.com/vercel-labs/skills/blob/a80e4c9d12d607631249d4d7d059f835d4b2f13a/src/skill-lock.ts
[skills-find]: https://github.com/vercel-labs/skills/blob/a80e4c9d12d607631249d4d7d059f835d4b2f13a/src/find.ts
[skills-api]: https://skills.sh/docs/api
[skills-docs]: https://skills.sh/docs
[apm-version]: https://github.com/microsoft/apm/blob/2920043258932f0f67e6f01df7b9047eb42f358c/pyproject.toml
[skills-version]: https://github.com/vercel-labs/skills/blob/a80e4c9d12d607631249d4d7d059f835d4b2f13a/package.json
