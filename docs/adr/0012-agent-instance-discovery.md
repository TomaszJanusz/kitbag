# 12. Discover agent instances and add them through the manifest

- Status: Proposed
- Date: 2026-10-26
- Revised: 2026-10-26, after the maintainer decided storage (manifest schema 2
  with a per-instance `hosts` filter), publication (fully automatic, no manual
  commits or messages) and discovery breadth (`--probe` on explicit request).

## Context and Problem Statement

After using `kitbag tui` v0.1.0, the maintainer found that the detected agents
are not visible anywhere and that there is no way to add an agent, or a second
account of the same agent (for example Claude Code in `~/.claude_2`), other than
hand-editing `kitbag.toml` ([KIT-80](https://linear.app/privacybrand/issue/KIT-80/tui-clickable-tabs-detected-agents-view-add-agentsecond-account-wizard)).
The maintainer calls this foundational. This record proposes how instances are
discovered, how the manifest expresses where an instance applies, how a change
reaches the repository without the user committing anything, and the CLI, API,
view-model and terminal surfaces for it. It changes no code; implementation
follows approval.

Facts the design must respect, all from the current tree:

- An agent instance is a configuration directory, declared as
  `[targets.<t>.instances.<n>] dir = "~/..."`; its identity is (agent kind,
  `dir`). A target without listed instances uses the adapter's single default
  directory, resolved from a relocation variable (`CLAUDE_CONFIG_DIR`,
  `CODEX_HOME`, the parent `GEMINI_CLI_HOME`, `XDG_CONFIG_HOME` for OpenCode).
  **Listing any instance replaces that implicit default**
  ([manifest.md](../manifest.md#targetsname)).
- A listed instance whose directory is missing produces a warning, is never
  created, and makes `sync` exit 1 ([apply.md](../apply.md)). The maintainer's
  real manifest declares `second = "~/.claude_2"` for every host, so any host
  without that directory warns on every sync.
- `[hosts.<h>.targets.<t>.instances]` **replaces** the target's whole set on
  that host; there is no partial merge.
- Unknown manifest keys are errors and "a new key arrives only with a new
  schema version"; a tool that knows schema N reads N and N−1
  ([manifest.md](../manifest.md#versioning-and-reading-rules)).
- A v0.1.x tool rejects `schema = 2` during validation with "value must be 1
  (unsupported schema; update kitbag or run an explicit migration)". That is a
  positioned validation error (exit 2), not a `*manifest.VersionError`, so it
  neither exits 4 nor emits the `tool-too-old` event; `min_tool_version` is
  never reached because validation fails first. Nothing is applied.
- `kitbag init` detection checks only whether the Claude and Codex default
  directories are real directories; it never lists HOME, reads agent files or
  runs an executable ([init.md](../init.md#detection-and-existing-repositories)).
  `kitbag doctor` resolves default homes separately and knows only Claude and
  Codex, so a Gemini or OpenCode target fails its instance check.
- Git is the source of truth ([ADR 2](0002-git-source-of-truth.md)); ADR 2
  already rejected pure file synchronization and a hosted service. The mapping
  from the old script decided that instance overrides live in the manifest.
- A dirty working copy makes `sync` skip the pull; a local commit that is not
  pushed diverges as soon as upstream moves, and divergence stops `sync`
  before apply ([vcs.md](../vcs.md#pull-policy)). `vcs.Backend.Commit` commits
  the whole working tree, and nothing in the interface merges, rebases, forces
  or discards. Publication is not implemented: KIT-53 (collect, check, commit,
  direct push) and KIT-54 (pull requests) are in the backlog, and
  `[publish] mode = "direct" | "pr"` already exists in schema 1.
- `config set` refuses comment-bearing files instead of re-encoding them; no
  comment-preserving manifest writer exists.
- UI behavior, validation and text live in `internal/ui/model`; renderers only
  draw ([ADR 5](0005-ui-layers.md)). The local API is versioned per request and
  authenticated ([ADR 10](0010-local-api.md)).

## Decision Drivers

- Never read, list or log credentials, sessions or any agent file content.
- One authoritative, reviewable declaration per repository (ADR 2).
- No surprise on other machines: an instance added on one host must not make
  sync warn on hosts that do not have that directory.
- **No manual Git.** The user never commits, writes a commit message or
  pushes for a change KitBag made; publication is automatic and observable.
- Never force-push, rewrite published history or resolve text conflicts.
- Preview before every write: the manifest diff and exactly what the next sync
  will change on this machine.
- Preserve the maintainer's hand-written manifest: comments, order, layout.
- Older tools fail closed and say why; no machine applies a half-understood
  manifest.
- One implementation shared by CLI, TUI and the later window; scriptable JSON.
- Small, independently reviewable PRs.

## 1. Discovery

### Considered options

- **D1. Resolved candidates only:** for each native adapter, its documented
  default directory and its relocation variable from the caller's environment,
  plus every instance declared in the manifest (for this host and others).
  Presence is a single `Lstat` of the directory itself.
- **D2. D1 plus an opt-in name probe:** one `ReadDir` of HOME (names only,
  never descending or following links) matching each adapter's documented
  directory name with a suffix (`.claude_2`, `.claude-work`, `.codex.alt`),
  then `Lstat` of each match.
- **D3. Marker or executable checks:** look for files such as `settings.json`
  or run `claude --version`.

### Decision

D1 by default; D2 only on explicit request, through `kitbag agent list --probe`
or the wizard's **Look for more directories** action (decided by the
maintainer). D3 is rejected for discovery: file markers sit next to
credentials (`.credentials.json`, `auth.json`) and gain little over a
directory check, and executable probes already live in `kitbag doctor`.

Every candidate reports a **source** (`default`, `environment` naming the
variable, `declared`, `probe`) and a **presence**:

| Presence | Meaning |
| --- | --- |
| `present` | A real directory. |
| `missing` | Nothing there. A declared instance active on this host warns on sync. |
| `not_directory` | A file or other entry. |
| `symlink` | A final symbolic link; reported, never followed. |
| `outside_home` | Absolute and outside HOME; the manifest cannot express it, so it cannot be added. |
| `unreadable` | `Lstat` failed for another reason; the error class only. |

Rules: no file is opened; no HOME listing without `--probe`; the probe reads
the entry names of HOME once, keeps only names that match a documented
directory name with a suffix, discards every other name immediately (never
returned, logged, stored or counted) and caches nothing; no relocation variable
is persisted; an explicit `dir` always wins over a variable, as the adapters
already do; `generic` has no default and appears only when declared.
Resolution moves into one place: each adapter package exports its existing
home resolver (`codex.New` already resolves `CODEX_HOME`, `geminiHome`,
`opencodeHome`), and a new `internal/core/discover` receives them as an
injected table from `cmd/kitbag`, so core still imports no adapter. `init`
detection and the doctor instance check then use the same table, which fixes
doctor for Gemini and OpenCode.

Environment: discovery uses the **caller's** relocation variables, like
settings resolution. Through the daemon, the client sends only the allow-listed
variables of known adapters; the daemon uses them for this request and never
stores them. When the caller's and the daemon's resolution of an implicit
default differ (typical when an agent shell exports `CLAUDE_CONFIG_DIR`), the
snapshot carries a warning recommending an explicit instance.

### Wording change to the "never lists HOME" promise

Today [init.md](../init.md#detection-and-existing-repositories) says
"Detection never lists HOME or reads agent settings, credentials, sessions or
instance files". `kitbag init` keeps that behavior unchanged. The promise is
narrowed from "never" to "never unless asked" for the new commands, and the
pull request that adds `--probe` (PR 3 in the
[plan](#7-tests-and-staged-implementation)) replaces the sentence with:

> Detection never lists HOME on its own. Only an explicit request
> (`kitbag agent list --probe` or **Look for more directories** in the
> terminal interface) reads the names of HOME's direct entries, once, without
> descending into or following any of them. Names that do not match a known
> agent directory are discarded without being shown or stored. Detection never
> reads agent settings, credentials, sessions or instance files, and never runs
> an agent executable.

The same text goes into the CLI help of `--probe`, and `docs/agents.md` links
to it. Doctor and sync never probe.

## 2. Where an instance applies: manifest schema 2

### Considered options

- **S1. Target level only** (`[targets.<t>.instances.<n>]`, schema 1). Every
  host gets it; a host without the directory warns and exits 1 on every sync.
  This is today's `~/.claude_2` problem.
- **S2. Schema-1 host overrides.** Readable by v0.1.x, but a host set replaces
  the target set, so a host-only addition must copy the target's set and later
  target-level additions no longer reach that host.
- **S3. Schema 2 with a per-instance `hosts` filter**, like `files[].hosts`.
- **S4. Machine-local `config.toml` instance list.** Contradicts ADR 2, hides
  the instance from review and other machines, and creates a second source
  every planner must merge.

### Decision

**S3** (decided by the maintainer): manifest schema 2 adds an allow-list
`hosts` on each instance, and host tables stop replacing instance sets.
Instance lists are never copied into host overrides. S2's copying and S4 are
rejected. (A held, unpublished change in the [publication queue](#3-automatic-publication)
gives consumer-only machines a visible, temporary alternative to S4.)

### Schema 2 definition

Schema 2 is schema 1 plus the following; every other key and rule is
unchanged.

| Key | Type | Default | Meaning |
| --- | --- | --- | --- |
| `schema` | integer | — | `1` or `2`. A schema-2 tool reads both. |
| `targets.<t>.instances.<n>.hosts` | non-empty unique array of host names | absent: every host | The instance is active only on the listed hosts. Same grammar and matching as `files[].hosts`. |
| `hosts.<h>.targets.<t>.instances.<n>` | table with only `enabled`, `mode`, `settings` | none | **Patches** the target's instance `<n>` on host `<h>`. `<n>` must exist at target level. `dir`, `roots`, `primitives` and `hosts` are rejected here with a message pointing to the target-level `hosts` key. |
| `publish.auto_merge` | boolean | `false` | In `pr` mode, request auto-merge when the repository allows it ([section 3](#3-automatic-publication)). |
| `publish.message` | title template | `chore(kitbag): {summary}` | Commit and pull request title for KitBag-generated changes; placeholders `{summary}`, `{host}`, `{kind}`. Must render a Conventional Commits header of at most 72 characters. |

Effective instances of target `t` on host `h`: every target-level instance
whose `hosts` is absent or contains `h`, after applying host patches; an
instance patched to `enabled = false` produces no operations there. The rule
"listing any instance replaces the implicit default" is unchanged and does not
depend on filters, so a target whose listed instances all exclude `h` has no
instance on `h`; `agent list` and doctor say so instead of silently falling
back. A host name in a `hosts` list that is neither this machine nor declared
under `[hosts]` is a doctor WARN (likely typo), not an error, because hosts
without a table are valid.

Why patch rather than replace in schema 2: every other host key
(`targets.<t>.enabled`, `targets.<t>.mode`) already patches, and a schema-1
replacement table always contains `dir`, which schema 2 rejects there. A file
whose `schema` is bumped by hand without converting therefore fails with a
positioned error instead of being misread.

The maintainer's `~/.claude_2` case becomes:

```toml
[targets.claude.instances.second]
dir = "~/.claude_2"
hosts = ["mac"]        # only the machines that have this account
```

Other hosts no longer see `second`, never warn about it and never exit 1 for
it. Where the directory is missing on a host where it *is* active, the sync
warning gains a fix hint: `restrict it: kitbag agent edit claude/second
--hosts <hosts>`.

### Reading, compatibility and `min_tool_version`

- **Preflight (lands first, in a schema-1 release).** `pkg/manifest` reads
  `schema` and `min_tool_version` from the parsed TOML before full validation.
  A `min_tool_version` above the running tool returns the existing
  `*manifest.VersionError` even when the rest of the document uses keys this
  tool does not know. A `schema` above the newest supported version returns a
  new `*manifest.SchemaError{Found, Supported}`, which the CLI maps to exit 4
  and `compatibility` to the `tool-too-old` event, like `VersionError`. A
  `schema` below the supported window keeps the "run the explicit migration"
  error (exit 2). Shipping this before schema 2 means every machine updated to
  that release reports a later schema bump as "update kitbag" with exit 4.
- **v0.1.x machines** cannot be changed: on a schema-2 manifest they stop
  before planning with the validation message above (exit 2), apply nothing
  and recover once the tool is updated. The rollout below therefore updates
  every machine before the manifest is upgraded.
- **Schema-2 tool, schema-1 file:** read with schema-1 semantics (replacement
  host sets, no `hosts` key), exactly as today. No warning on sync.
- **`min_tool_version`:** `kitbag manifest upgrade` writes it as the larger of
  the existing value and the first release that reads schema 2, so the
  requirement is visible in the file and in doctor. A schema-2 tool treats an
  absent value as that release.
- **JSON Schema:** `schemas/kitbag.schema.json` describes schema 2;
  `schemas/kitbag.v1.schema.json` keeps schema 1 for editors of old files.
  Examples move to `schemas/examples/kitbag/v1` and `v2`, both validated by
  `internal/schemacheck` against both readers. The SchemaStore handoff gains a
  versions entry; nothing is published as part of this work.

### Migration: `kitbag manifest upgrade`

A new command group `kitbag manifest` holds operations on `kitbag.toml`
(machine-local `config.toml` stays under `kitbag config`). `kitbag manifest
upgrade [--dry-run] [--json] [--yes]` converts schema 1 to 2 losslessly, with
the [position-preserving editor](#editing-without-losing-the-file) described
below:

1. `schema = 1` becomes `schema = 2`; `min_tool_version` is written as above.
2. For each host override `O` of target `t` with target set `T`, compared by
   name and identical definition:
   - an instance in `O` and `T` stays in `T`;
   - an instance only in `O` moves to `T` with `hosts` listing every host whose
     override contains it;
   - an instance in `T` missing from `O` gets a host patch `enabled = false`.
   - When `T` is empty (implicit default) and an override exists, the default
     is declared as `main` with its documented `~/` directory, and the preview
     warns that machines relocating it with a variable must declare the real
     directory.
3. Refused, with the position and a manual-edit hint (exit 2): the same name
   with different definitions in `T` and `O`, or two names for one identity.
4. The result is reparsed as schema 2 and its resolved instance sets are
   compared with the schema-1 resolution for every host named in the file
   plus one undeclared host; any difference aborts without writing.

The preview lists each conversion and, as an advisory, the last KitBag version
seen per host from `Kitbag-Host`/`Kitbag-Version` commit trailers
([section 3](#3-automatic-publication)); hosts seen on a tool older than
schema-2 support are named as "will stop syncing until updated", and declared
hosts never seen in a trailer are listed as unknown. The upgrade
is published like every other KitBag change, as
`chore(kitbag): upgrade manifest to schema 2`.

### `pkg/manifest` public API

All changes are additive and recorded through the `apidiff` baseline
(`docs/api/manifest.export`) in the pull request that makes them; the module
path stays the same and no signature changes:

- `Instance.Hosts []string`; `Publish.AutoMerge *bool`; `Publish.Message string`.
- `SchemaError` (with `Found`, `Supported`), constants `SchemaLatest = 2` and
  `SchemaOldest = 1`.
- `CheckName`, `CheckHomePath`, `CheckHostName`: the grammar validators the
  UI models use per keystroke.
- Behavior: `Parse`/`Load` accept 1 and 2; `ResolveHost` filters by `hosts` and
  applies schema-2 patches. `HostTarget.Instances` keeps its type; its doc
  states that it replaces the set in schema 1 and patches by name in schema 2.
  Library users that read host overrides directly must branch on
  `Manifest.Schema`; `ResolveHost` is the supported path. Package and type docs
  stop saying "schema-v1".

### Doctor and `kitbag config` with a schema-1 manifest

- New doctor check `manifest-schema`, after Manifest/host: OK for schema 2; OK
  for schema 1 with "schema 1 (supported); `kitbag manifest upgrade` enables
  per-host instances"; FAIL with exit 4 when the schema is newer than the tool
  (as for `min_tool_version`); FAIL with the migration hint when it is older
  than the supported window.
- The Instances check, on a schema-1 manifest, adds to a missing-directory
  WARN the fix `upgrade the manifest (kitbag manifest upgrade), then restrict
  the instance with kitbag agent edit`.
- `kitbag agent add/edit/remove` on a schema-1 manifest: changes valid in
  schema 1 (all-hosts additions, removals, `enabled`) proceed; anything that
  needs `hosts` is refused (exit 2) with `this needs manifest schema 2; preview
  the upgrade with kitbag manifest upgrade --dry-run`. The wizard shows the
  same as a step action instead of failing.
- `kitbag config` is unaffected: it manages `config.toml`, whose own schema
  stays 1; it never reads, migrates or rewrites `kitbag.toml`, and this design
  adds no `config.toml` keys.

### Editing without losing the file

The manifest is never re-encoded. A new `internal/core/manifestedit` package
inserts, updates or deletes keys and whole table blocks at positions found
with the same `go-toml/v2/unstable` parser that `pkg/manifest` uses for error
positions. After every edit it reparses the result and checks that the decoded
manifest equals the old one plus exactly the intended change; anything else
(inline tables, dotted keys defining the table, an unexpected difference) is
refused with a message to edit by hand. Bytes outside the edited block are
unchanged.

Every change is a compare-and-swap: the preview returns a **revision** (hash of
the effective manifest bytes and the queue state); applying with a stale
revision fails with a conflict (exit 3). The edit runs under the state lock, so
it never races a daemon fast-forward.

## 3. Automatic publication

The maintainer wants full automation: no manual commit, no commit message to
write, no push to remember, and Git stays the source of truth. This section is
the proposed basis for KIT-53's scope.

### Options

**(a) Generated Conventional Commit messages.** KitBag computes a *semantic
diff* of the decoded manifests (before, after) and renders one line per change:
`add claude instance ~/.claude_2 for host mac`, `restrict claude instance
second to hosts mac, runner02`, `upgrade manifest to schema 2`. The title is
`publish.message` (default `chore(kitbag): {summary}`); with several changes
the summary becomes `update 3 agent instances for host mac` and the body lists
each line. Trailers make commits machine-readable:

```text
chore(kitbag): add claude instance ~/.claude_2 for host mac

- add claude instance second (~/.claude_2), hosts: mac

Kitbag-Host: mac
Kitbag-Op: agent.add
Kitbag-Change: 01JB9Q8W2V5R3T0F6M4K7XNZ1C
Kitbag-Version: 0.3.0
```

`Kitbag-Change` is the queue entry ID and makes publication idempotent after a
crash; `Kitbag-Version` feeds the upgrade readiness hint. A generic fallback
(`chore(kitbag): update kitbag.toml`) covers changes the diff cannot name.
Accepted: the user never writes a message, and the history stays reviewable.

**(b) Debounce and batching.** Changes wait in a queue for a quiet period
(30 s after the last change, at most 5 min after the first) and become one
commit. A wizard session is one batch by construction. Coalescing is
structural: add then remove of the same identity cancels; add then edit
merges. Accepted; `kitbag push` publishes the queue immediately.

**(c) Publish modes, from the existing `[publish]` policy.**

- `direct`: commit on top of the fetched upstream and push to `branch`. On a
  non-fast-forward rejection, fetch and **re-derive** the change from its
  structured intent on the new upstream (below) instead of rebasing text;
  never force.
- `pr`: push a branch `kitbag/{host}/{slug}` (the documented name) and open a
  pull request; while that pull request is open, later batches from the same
  host are appended to it as new commits, so one host has at most one open
  KitBag pull request. If it can no longer be fast-forwarded or merged
  cleanly, KitBag opens a fresh one from the current upstream with the
  re-derived changes and closes the old one with "superseded by #n", instead
  of force-pushing. With `publish.auto_merge = true` KitBag requests auto-merge
  with the repository's allowed method; when the repository or token does not
  allow it, the pull request stays open and status says "waiting for review".
- A per-host branch without a slug (`kitbag/<host>`) was considered: reusing
  one name after a squash merge would need a force push or a delete, so it is
  rejected in favor of the documented slug form.

Accepted, as already declared in schema 1.

**(d) Local-only.** The change is applied on this machine from a local
**overlay** and held unpublished until the user publishes it. Accepted as a
per-change choice (**Apply here only**, `--keep-local`) and a per-machine hold
(`kitbag push --hold` / `--release`), stored in the queue, not in
`config.toml`. Held entries are always visible in `status`, the TUI and doctor
(WARN after 7 days), so they cannot become a hidden second source of truth.

**(e) Non-Git transports.**

| Transport | Verdict | Reason |
| --- | --- | --- |
| Synced overlay file (Syncthing, iCloud, Dropbox) | Rejected | No review, no history, last-writer-wins conflicts, a second source of truth; ADR 2 already rejected file synchronization. |
| Hosted KitBag API or relay | Rejected | A new authoritative server with accounts and credentials to protect; ADR 2 rejected it. |
| A mailbox ref on the same remote (`refs/kitbag/pending/<host>`) | Rejected | Still Git, but hides changes from normal review and from forge protections. |
| Forge commit API (GitHub `createCommitOnBranch`) | Deferred | Still Git and reviewable; needs no local signing key (the forge signs) and no push credential. Useful for machines with a token but no SSH key; it belongs to the forge abstraction of KIT-54. |
| Peer "fetch now" hints | Deferred | ADR 2 already allows them for latency only; after a publish, other machines could fetch early. Daemon notification work. |

### Recommended design: a publication queue with intent replay

1. **Record.** A KitBag-originated change (`agent add/edit/remove`, `manifest
   upgrade`) is not written into the working copy. It is appended to a
   **publication queue** in the state directory: a versioned, locked journal of
   entries `{id, ops (structured), base revision, created, state}`. The working
   copy stays clean, so `sync` keeps pulling and never diverges because of
   KitBag's own changes.
2. **Apply locally at once.** The planner's input is the *effective manifest*:
   the upstream `kitbag.toml` with every pending or held entry applied in
   memory by `manifestedit`, then parsed by the normal reader. The planner sees
   one manifest; there is no merge of two sources. An entry that no longer
   applies is skipped with a warning and marked `conflict`.
3. **Publish.** After the debounce, the publisher fetches, re-derives the batch
   on the fetched upstream manifest, runs the KIT-53 checks (manifest
   validation, dry-run plan, secret scan), builds the commit **without touching
   the working copy or the current branch** (from the upstream tree plus the
   new `kitbag.toml` blob), and pushes that commit non-forcing to the policy
   branch. Only `kitbag.toml` may change in a structured commit; anything else
   is refused.
4. **Complete.** An entry is done when re-deriving it on upstream is a no-op
   (its intent is present), which covers direct pushes and squash-merged pull
   requests alike. The next ordinary pull fast-forwards the working copy.

Hand edits in the working copy are not queued: `kitbag push` commits them in
place with a message generated from the same semantic diff and pushes, and a
divergence stops with the working-copy path, as KIT-53 already specifies.
Whether agent-edited drift (link mode) is also published without asking is an
open question; this design publishes KitBag-originated changes automatically
and leaves drift to `kitbag push` until decided.

### Failure handling

| Situation | Behavior |
| --- | --- |
| Offline or remote unreachable | Entry stays `pending`, still applied locally; the daemon retries on its poll interval, the CLI reports "queued" (exit 0 with a warning). |
| Push rejected, non-fast-forward | Fetch, re-derive, rebuild, push again; at most 3 attempts per cycle, then `pending` until the next cycle. |
| Intent contradicts upstream (same name with another `dir`, target removed, layout refused, newer schema) | Entry `conflict`; not applied; status and TUI show both sides with **Drop**, **Edit** or **Keep here only**. Exit 3 from `kitbag push`. |
| Protected branch in `direct` mode (`remote rejected`) | With a forge credential, publish as a pull request this time and say so; otherwise `blocked` with the fix "set `publish.mode = pr` for this host". Never retried in a loop. |
| Authentication or permission failure | `blocked` until credentials change or the user runs `kitbag push`; the daemon does not retry it. |
| Check failure (validation, secret scan) | `blocked` with the finding; nothing is pushed. |
| Pull request closed without merge | Entry `rejected`; no longer applied; offered for **Drop** or **Publish again**. |
| Crash between push and journal update | On restart, an upstream commit or open pull request carrying the entry's `Kitbag-Change` trailer marks it published; no duplicate. |

### Security

- **Credentials.** Git transport uses the existing paths (system Git with its
  SSH agent and credential helpers, or embedded SSH); KitBag stores no Git
  credential. `pr` mode needs a forge token: `GH_TOKEN`/`GITHUB_TOKEN` from the
  caller's environment or the OS keyring through a new platform credential
  contract; never in `kitbag.toml`, `config.toml`, the queue, logs, JSON or API
  results. Recommended token: fine-grained, one repository, contents and pull
  requests write.
- **Daemon.** Unattended publishing works only when the service can reach the
  credentials; otherwise entries stay `blocked: auth` and a session command
  publishes them. The `publish.*` API methods are owner-only like every
  method (ADR 10).
- **Author.** Commits use this machine's Git identity (system Git
  configuration, or `git.identity` for embedded Git), so the human owner stays
  accountable and signing keys match. A runner can set a dedicated identity
  (for example `KitBag on runner02`) through the same settings; the
  `Kitbag-Host` trailer records the machine either way. A built-in bot identity
  is rejected: it hides who made the change and cannot sign.
- **Signing.** Commits built by the system backend must honor
  `commit.gpgSign` and the signing key, which the backend contract test must
  prove with a test key; embedded Git keeps refusing when signing is required.
  Plumbing commits bypass local `pre-commit` and `commit-msg` hooks; KitBag's
  own checks run instead, and repository CI remains the authority.
- **Scope.** The publisher writes only to the policy branch or the
  `kitbag/` prefix, never force-pushes, and structured commits touch only
  `kitbag.toml`.

### Consequences for KIT-53 and KIT-54

KIT-53 ("Publish assistant") becomes the publication engine plus `kitbag push`
as one front end; `--message` remains an optional override and `$EDITOR` opens
only with `--edit`. Proposed breakdown:

| Ticket | Scope | Size |
| --- | --- | --- |
| KIT-53a | Semantic manifest diff and Conventional Commit generator with trailers and `publish.message` (pure core) | M |
| KIT-53b | `vcs`: build a commit from a base revision plus file contents without touching the working copy; push a revision to a branch without force; classify rejections (non-fast-forward, remote policy, auth); system and embedded backends, signing test | M |
| KIT-53c | Publication queue (versioned journal under the state lock), effective-manifest overlay for plan and sync, `status`/doctor visibility | L |
| KIT-53d | Direct publisher: debounce, batching, re-derivation, failure states, idempotency trailer; `kitbag push [--hold|--release|--json]` | L |
| KIT-53e | KIT-53's checks and working-copy path: validation, dry-run plan, secret and Unicode scans, generated message for hand edits, drift collection | L |
| KIT-53f | API `publish.status`, `publish.flush`, `publish.drop` and the model for it | M |
| KIT-54 | `pr` mode: branch, one open pull request per host, supersede instead of force, auto-merge, forge credential, closed-without-merge handling | L |

## 4. CLI surface

```text
kitbag agent list    [--json] [--probe]
kitbag agent add     <kind> [<dir>] [--name <n>] [--target <t>]
                     [--hosts <h,...> | --this-host | --all-hosts] [--settings=on|off]
                     [--keep-local] [--dry-run] [--json] [--yes] [--revision <r>]
kitbag agent edit    <target>/<instance> [--hosts <h,...> | --all-hosts]
                     [--enable-here | --disable-here] [--settings=on|off]
                     [--keep-local] [--dry-run] [--json] [--yes] [--revision <r>]
kitbag agent remove  <target>/<instance> [--this-host]
                     [--keep-local] [--dry-run] [--json] [--yes] [--revision <r>]
kitbag manifest upgrade [--dry-run] [--json] [--yes]
kitbag push          [--hold | --release] [--json]
```

- `list` prints, per kind, the declared instances (target, name, directory,
  hosts, settings merge, enabled, active on this host, presence), then
  undeclared candidates, then pending queue entries that touch agents.
  `--host` selects whose view is shown, as everywhere.
- `add` defaults: `<dir>` is the kind's resolved default when undeclared, else
  required; `--name` is derived from the directory (`~/.claude_2` →
  `claude_2`) and validated with the manifest name grammar; scope is
  `--this-host` (`hosts = [<this host>]`) when the directory is present here,
  else `--hosts` or `--all-hosts` must be given (on a schema-1 manifest only
  `--all-hosts` is possible and must be given explicitly); `--settings` defaults to on and
  is rejected for kinds or targets without a settings fragment.
- `edit --disable-here`/`--enable-here` write a schema-2 host patch.
- `remove` deletes the declaration. With `--this-host`, an instance with a
  `hosts` list loses this host (and is deleted when the list would become
  empty); an instance without one gets a host patch `enabled = false`. The
  preview shows the removals the next sync will perform for files KitBag owns
  there. Removing the last listed instance re-enables the implicit default,
  which the preview states.
- Mutating commands show the manifest diff, the plan preview and how the
  change will be published (`direct to main`, `pull request`, `held`), then ask
  on a terminal; without one they require `--yes` or `--dry-run`. `--dry-run`
  never writes and exits 0. There is no `--commit` and no `--message`.
- JSON: `list` emits `agents_schema: 1`; mutations emit `agent_change_schema: 1`
  with `change`, `manifest` (`path`, `schema`, `revision_before`,
  `revision_after`, `diff` lines), `plan` (the `plan_schema` target and
  operation objects for affected targets), `publication` (`state`: `published`,
  `queued`, `held`, `pull_request`, `blocked`; `commit`, `url`, `reason`),
  `warnings`. `manifest upgrade` emits `manifest_upgrade_schema: 1`, `push`
  emits `publish_schema: 1`. Schemas go to `schemas/`.
- Exit codes keep the shared meanings: 0 success, dry run or queued; 1
  operational (applied locally but publication `blocked`, reported as such);
  2 usage or validation (unknown kind, invalid name, path or host, duplicate
  identity, `outside_home`, non-interactive without `--yes`, a layout
  `manifestedit` refuses, a schema-2 feature on a schema-1 manifest); 3
  conflict (stale revision, queue conflict); 4 tool too old; 5 unsupported OS.

## 5. API methods and view models

Local API v1.1 adds methods; older clients are unaffected and older daemons
answer method-not-found, after which the client uses the in-process path, as
status and plan do.

| Method | Params | Result |
| --- | --- | --- |
| `agents.list` | `env` (allow-listed relocation variables), `probe` | `AgentsSnapshot` |
| `agents.preview` | `AgentChange` | `AgentPreview` (diff, affected plan, publication route, warnings, `revision`) |
| `agents.apply` | `AgentChange`, `revision`, `keep_local` | `AgentApplied` (new revision, `Publication`, warnings) |
| `publish.status` | — | `Publication` queue (entries, states, reasons) |
| `publish.flush`, `publish.drop` | entry IDs or all | updated `Publication` |

`internal/ui/model` gains these types and `Client` methods;
[ui.md](../ui.md#proposed-agents-and-instances-adr-12) lists them in full. Two
models follow the existing state-machine pattern: `Agents` (loading, ready,
failed; selection follows (kind, dir) across reloads) and `AgentWizard` (steps
below; sequence-numbered previews so a slow answer never replaces a newer
draft). Field validation (name grammar, home-path and host grammar, duplicate
identity, name taken in the target, settings applicability, schema-2 features
on a schema-1 manifest) runs in the model on every keystroke using the
validators exported from `pkg/manifest`, and again authoritatively on the
server. New error classes reuse existing kinds: stale revision and queue
conflict → `conflict`; refused layout and validation → `invalid`.

## 6. Terminal interface

A fifth tab, **5 Agents**, is appended so the v0.1.0 keys `1`–`4` keep their
meaning. It lists declared instances grouped by kind, then undeclared
candidates, with source, presence, hosts and publication state as text (never
colour alone). Keys: `↑`/`↓`, `k`/`j`, Enter details, `a` add (prefilled from
the selected candidate), `e` edit hosts, `x` remove, `p` look for more
directories, `r` refresh. The status line shows the queue (`1 change
publishing`, `1 held`, `1 blocked: auth`).

The add wizard is a modal sequence; each step shows its title, a one-line
explanation and the validation message of the current draft:

1. **Agent kind:** native kinds, detected kinds first; `generic` is shown as
   available from the CLI only, because it needs explicit roots.
2. **Directory:** resolved candidates for that kind with presence, plus
   **Other path…**, a line editor accepting `~/` paths, and **Look for more
   directories** (the probe). Missing directories are allowed with the warning
   that sync skips them until the agent creates them; `outside_home` and
   duplicates block **Next**.
3. **Name and target:** derived name, editable; target chooser only when
   several targets of the kind exist.
4. **Where to use it:** **This machine (`mac`)**, **All machines** or
   **Choose machines…** (declared host names plus free entry). On a schema-1
   manifest only **All machines** is enabled, with **Preview manifest
   upgrade** as an action.
5. **Settings merge:** on or off, only when the target declares a settings
   fragment and the adapter merges settings; otherwise an explanatory line.
6. **Preview:** the manifest diff, the affected plan, and the publication
   route and generated title, loaded with `agents.preview`.
7. **Confirm:** **Apply** (applies here and publishes per policy) or **Apply
   here only** (held); then the result with the publication state and an offer
   to sync now (`s`).

Keyboard: Enter or `Ctrl-N` next, Esc or `Ctrl-P` back, Esc on step 1 cancels
(asking first when a draft exists), `Tab`/`Shift-Tab` between fields,
`↑`/`↓` in lists, Space or `←`/`→` toggles, `Ctrl-C` quits as everywhere. While
the line editor is focused, typed keys go to it. Mouse, once the tab-click work
of KIT-80 lands: click a tab, click a row to select, double-click to open,
wheel to scroll, and click the **Back**, **Next**, **Cancel** and **Apply**
buttons. Every mouse action has a key; mouse reporting must not break SSH
sessions or `NO_COLOR` output.

## 7. Tests and staged implementation

Tests:

- **Discovery (core, fake platform):** variables set, unset, relative and
  outside HOME; directory, file, symlink, missing; implicit default versus
  explicit `dir`. A recording file system fails the test on any `ReadFile`,
  `Open` or `ReadDir` other than the probe's single HOME listing, and on any
  non-matching name leaving the probe. Negative control: plant
  `.credentials.json` and `auth.json` and make discovery read them; the
  recorder must fail.
- **Schema 2:** valid and invalid examples for every new key and for each
  rejected key in a host patch; preflight cases (newer schema → `SchemaError`,
  high `min_tool_version` with unknown keys → `VersionError`); a v0.1.0 binary
  testscript on a schema-2 manifest asserting it stops without applying.
- **Upgrade:** for every schema-1 fixture and the maintainer's manifest, the
  resolved instance sets per host before and after are equal; refusal cases;
  goldens asserting byte equality outside edited blocks.
- **Manifest editing:** refusal of inline and dotted layouts; a fuzz target
  over the manifest corpus asserting "refuse, or reparse equals intent".
- **Publication:** fake remote and real local bare repositories: direct
  publish; non-fast-forward with re-derivation; contradicting upstream →
  conflict; protected-branch rejection; offline queue survives restart; crash
  after push → no duplicate (trailer); batch coalescing; generated titles
  golden; signed commit with a test key (system backend); working copy stays
  clean throughout. Negative control: make the publisher write the working copy
  and assert `sync` then skips the pull.
- **Preview equals sync:** on the fake host, the preview plan of a change equals
  the plan after it is queued, and again after it is published.
- **CLI (testscripts, throwaway HOME):** `agent list` with and without
  variables and `--probe`, JSON validated against its schema; `add --dry-run`
  leaves manifest and queue unchanged; `add`, then `plan` shows the instance,
  `sync` applies it and a repeat is silent; `--hosts` on schema 1 exits 2;
  `remove --this-host`; stale revision exits 3; non-interactive without `--yes`
  exits 2.
- **API, models, TUI:** schema-generated contract cases and method-not-found
  fallback against a v1.0 server; wizard transitions, per-keystroke
  validation, stale preview discard, every text key; teatest goldens at 80×24
  and 40×10, `NO_COLOR`, cancel with a draft, mouse once available.

Staged plan, one focused change per PR (S under 200 changed lines, M up to
600, L up to 1200, excluding generated files and goldens). Releases are named
by role; numbers are assigned at release time.

| # | PR | Size | Depends on | Ticket |
| --- | --- | --- | --- | --- |
| 1 | `feat(manifest)`: schema/`min_tool_version` preflight, `SchemaError` → exit 4 and `tool-too-old`, doctor `manifest-schema` | M | this ADR | KIT-80 |
| 2 | `refactor(discover)`: shared home resolvers and `internal/core/discover`; `init` and `doctor` use it | M | — | KIT-80 |
| 3 | `feat(agent)`: read-only `kitbag agent list` with `agents_schema`, `--probe` and the init.md wording change | M | 2 | KIT-80 |
| 4 | `feat(manifest)`: exported `CheckName`/`CheckHomePath`/`CheckHostName` | S | — | KIT-80 |
| 5 | `feat(manifestedit)`: position-preserving edits with reparse verification and fuzzing | L | — | KIT-80 |
| — | **Release R1** (reads schema 1; preflight active). Update every machine. | | 1 | |
| 6 | `feat(manifest)`: schema 2 reading (`hosts`, host patches, publish keys), JSON Schemas v1/v2, examples, `apidiff` baseline, manifest.md | L | 1 | new |
| 7 | `feat(publish)`: semantic diff and message generator | M | 6 | KIT-53a |
| 8 | `feat(vcs)`: commit from base plus files, push revision, rejection classes | M | — | KIT-53b |
| 9 | `feat(publish)`: queue and effective-manifest overlay | L | 5, 6 | KIT-53c |
| 10 | `feat(publish)`: direct publisher and `kitbag push` | L | 7, 8, 9 | KIT-53d |
| 11 | `feat(publish)`: KIT-53 checks gate (validation, plan, secret scan) | L | 10 | KIT-53e |
| — | **Release R2** (reads schema 1 and 2; publishes directly). Update every machine; doctor shows readiness. | | 6–11 | |
| 12 | `feat(manifest)`: `kitbag manifest upgrade` | M | 5, 9, 10 | new |
| 13 | `feat(agent)`: `agent add/edit/remove` through the queue | L | 3, 4, 9, 10 | KIT-80 |
| 14 | `feat(api)`: `agents.*` and `publish.*` (API v1.1) and the `apiclient` adapter | M | 13 | KIT-80, KIT-53f |
| 15 | `feat(ui)`: `Agents`, `AgentWizard` and publication models with texts | M | 4, 14 | KIT-80 |
| 16 | `feat(tui)`: Agents tab, wizard and queue status | L | 15, KIT-80 mouse for clicks | KIT-80 |
| 17 | `feat(publish)`: `pr` mode | L | 10 | KIT-54 |

Rollout for the maintainer's repository: after R2 is on every machine (doctor
`manifest-schema` and the trailer hint show it), run `kitbag manifest upgrade`
on mac, then `kitbag agent edit claude/second --hosts mac` (or the wizard);
both publish automatically. Before R2, the `~/.claude_2` warning remains on
hosts without that directory; no interim copy is written. runner02, whose
policy is `pr`, publishes through pull requests only once PR 17 lands; until
then its KitBag changes are applied locally and held, visible as such.

## Consequences

- The maintainer can add `~/.claude_2` for the machines that have it, from the
  TUI or CLI, see the diff, plan and publication route first, and never touch
  Git; comments in `kitbag.toml` survive.
- Hosts without a directory stop warning about it once the manifest is on
  schema 2; no instance list is ever copied.
- Machines on v0.1.x stop (fail closed, exit 2) after the upgrade until they
  are updated; machines on R1 or later report exit 4 with an update hint.
- `pkg/manifest` grows additively; library users reading host overrides must
  respect `Schema`.
- KitBag-originated changes never dirty the working copy, so they cannot block
  pulls or diverge; a pending change is applied locally from the queue and is
  visible until published.
- Credentials stay unread by discovery by construction, which tests enforce;
  publication uses existing Git credentials and an optional forge token.
- Plumbing-built commits skip local Git hooks; KitBag's checks and repository
  CI replace them.
- The local API grows by a minor version.

## Open questions for the maintainer

Decided in this revision: storage (schema 2 with `hosts`, former question 1),
Git (automatic publication, former question 2), discovery breadth (`--probe`
on request, former question 3). Still open:

1. **Drift publication:** should agent-edited drift in link mode also be
   published automatically after the checks, or stay with `kitbag push`
   (proposed until KIT-53e)?
2. **Protected `direct` branch:** fall back to a pull request automatically
   when a forge token exists (proposed), or always stop as `blocked`?
3. **Commit author:** this machine's Git identity (proposed), or a dedicated
   identity per runner by default?
4. **Auto-merge default:** `publish.auto_merge = false` (proposed) or `true` for
   `pr` hosts?
5. Default scope when the directory exists here: this host (proposed) or all
   hosts?
6. Should `remove` restore the instance's original backups like `unmanage`, or
   only drop the declaration and let the planner remove owned outputs
   (proposed)?
7. Should KitBag help launch the second account (for example print a
   `CLAUDE_CONFIG_DIR=~/.claude_2 claude` alias hint), or is that out of scope?
8. Tab position: append as 5 (proposed) or insert after Drift and renumber?
9. Is a held queue entry (option d) enough for consumer-only machines without
   push rights, or is a permanent machine-local instance list still needed?

## Links

- [KIT-80](https://linear.app/privacybrand/issue/KIT-80/tui-clickable-tabs-detected-agents-view-add-agentsecond-account-wizard),
  design step (b);
  [KIT-53](https://linear.app/privacybrand/issue/KIT-53/publish-assistant-collect-changes-validate-propose-a-conventional)
  and [KIT-54](https://linear.app/privacybrand/issue/KIT-54/publish-via-pull-request-per-host-policy-pr-creation-and-tracking),
  publication.
- [Manifest instances and hosts](../manifest.md#targetsname),
  [versioning rules](../manifest.md#versioning-and-reading-rules),
  [publish policy](../manifest.md#publish),
  [first-run detection](../init.md#detection-and-existing-repositories),
  [pull policy](../vcs.md#pull-policy), [doctor](../doctor.md),
  [manifest API](../api/README.md), [shared view models](../ui.md),
  [terminal interface](../tui.md), [ADR 2](0002-git-source-of-truth.md),
  [ADR 5](0005-ui-layers.md), [ADR 10](0010-local-api.md).
