# 12. Discover agent instances and add them through the manifest

- Status: Proposed
- Date: 2026-10-26

## Context and Problem Statement

After using `kitbag tui` v0.1.0, the maintainer found that the detected agents
are not visible anywhere and that there is no way to add an agent, or a second
account of the same agent (for example Claude Code in `~/.claude_2`), other than
hand-editing `kitbag.toml` ([KIT-80](https://linear.app/privacybrand/issue/KIT-80/tui-clickable-tabs-detected-agents-view-add-agentsecond-account-wizard)).
The maintainer calls this foundational. This record proposes how instances are
discovered, where an added instance is stored, and the CLI, API, view-model and
terminal surfaces for it. It changes no code; implementation follows approval.

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
  schema version" ([manifest.md](../manifest.md#versioning-and-reading-rules)).
- `kitbag init` detection checks only whether the Claude and Codex default
  directories are real directories; it never lists HOME, reads agent files or
  runs an executable ([init.md](../init.md#detection-and-existing-repositories)).
  `kitbag doctor` resolves default homes separately and knows only Claude and
  Codex, so a Gemini or OpenCode target fails its instance check.
- Git is the source of truth ([ADR 2](0002-git-source-of-truth.md)); the
  mapping from the old script already decided that instance overrides live in
  the manifest, not in machine configuration.
- A dirty working copy makes `sync` skip the pull; a local commit that is not
  pushed diverges as soon as upstream moves, and divergence stops `sync`
  before apply. `vcs.Backend.Commit` commits the whole working tree.
  Publication (`kitbag push`, KIT-53) is not implemented yet.
- `config set` refuses comment-bearing files instead of re-encoding them; no
  comment-preserving manifest writer exists.
- UI behavior, validation and text live in `internal/ui/model`; renderers only
  draw ([ADR 5](0005-ui-layers.md)). The local API is versioned per request and
  authenticated ([ADR 10](0010-local-api.md)).

## Decision Drivers

- Never read, list or log credentials, sessions or any agent file content.
- One authoritative, reviewable declaration per repository (ADR 2), readable by
  every installed v0.1.x tool.
- No surprise on other machines: an instance added on one host must not make
  sync warn on hosts that do not have that directory.
- Preview before every write: the manifest diff and exactly what the next sync
  will change on this machine.
- Preserve the maintainer's hand-written manifest: comments, order, layout.
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

### Recommendation

D1 by default, D2 only behind an explicit `--probe` flag or the wizard's
**Look for more directories** action. D3 is rejected for discovery: file
markers sit next to credentials (`.credentials.json`, `auth.json`) and gain
little over a directory check, and executable probes already live in
`kitbag doctor`.

Every candidate reports a **source** (`default`, `environment` naming the
variable, `declared`, `probe`) and a **presence**:

| Presence | Meaning |
| --- | --- |
| `present` | A real directory. |
| `missing` | Nothing there. Declared listed instances will warn on sync. |
| `not_directory` | A file or other entry. |
| `symlink` | A final symbolic link; reported, never followed. |
| `outside_home` | Absolute and outside HOME; the manifest cannot express it, so it cannot be added. |
| `unreadable` | `Lstat` failed for another reason; the error class only. |

Rules: no file is opened; no HOME listing without `--probe`; no relocation
variable is persisted; an explicit `dir` always wins over a variable, as the
adapters already do; `generic` has no default and appears only when declared.
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

## 2. Where an added instance is stored

### Considered options

- **S1. Target level of the shared manifest** (`[targets.<t>.instances.<n>]`).
  Applies to every host. Correct when the directory exists everywhere; on other
  hosts a missing directory warns and exits 1 on every sync.
- **S2. Host override in the shared manifest**
  (`[hosts.<h>.targets.<t>.instances.<n>]`). Valid in schema 1 and readable by
  v0.1.0. Because a host set replaces the target set, the first host-only
  addition must copy the target's effective set into the override, and later
  target-level additions no longer reach that host.
- **S3. New per-instance filter** `hosts = ["runner02"]`, like `files[].hosts`.
  The smallest, clearest diff with no copy and no later divergence. It is a new
  key, so by the documented rule it needs manifest schema 2 (or an explicit
  exception) and makes older tools stop.
- **S4. Machine-local `config.toml`** (for example `[agents.<t>.instances]`).
  No commit and immediate; works on machines without push rights. It
  contradicts ADR 2 and the decision that instance overrides live in the
  manifest, hides the instance from review and other machines, and creates a
  second instance source that every planner, doctor and status must merge.

### Recommendation

S1 or S2, chosen per addition by an explicit **scope**, with mitigations for
S2's replacement semantics. S3 is the better long-term shape and is raised as
an open question for schema 2; S4 is rejected.

- **This host only** (the default when the directory was discovered on this
  machine): write into `[hosts.<host>.targets.<t>.instances]`. If the host has
  no override for that target yet, the edit first copies the target's effective
  set, and the preview lists the copied entries with the warning that later
  target-level instances will not reach this host.
- **All hosts:** write into `[targets.<t>.instances]`. For every host that
  already overrides that target's set, the preview offers (checked by default)
  to add the instance there as well, so "all hosts" really means all hosts.
- **Implicit default:** when the target lists no instances, adding one would
  silently drop the adapter default. The edit declares the default explicitly
  as instance `main` with its documented `~/` directory (never a relocation
  variable), and the preview says so.
- **Target:** reuse the only target of that kind; ask when several exist;
  create `[targets.<kind>] agent = "<kind>"` when none exists.
- `kitbag agent list` flags host overrides whose set differs from the target
  set, so S2 divergence is visible.

**Who commits and pushes.** Until publication exists (KIT-53):

1. `add` and `remove` write `kitbag.toml` in the working copy only. A
   following `sync` (which skips the pull while the copy is dirty) applies the
   new instance on this machine immediately.
2. `--commit` additionally creates a local commit with the message
   `chore(kitbag): add <kind> instance <name> for <scope>`. It is refused
   (conflict, exit 3) when the working copy had other changes before the edit,
   because `Commit` commits the whole tree.
3. Pushing stays the user's step; output prints the exact command and warns
   that an unpushed commit diverges as soon as upstream moves, which stops
   sync. When KIT-53 lands, `--publish` follows the manifest's per-host
   `[publish]` policy (`direct` or `pr`), so runner02 proposes a pull request.

Every write is a compare-and-swap: the preview returns a **revision** (hash of
the manifest bytes plus `HEAD`); applying with a stale revision fails with a
conflict, unlike today's `config.set`. The edit runs under the state lock, so
it never races a daemon fast-forward.

**Editing without losing the file.** The manifest is never re-encoded. A new
`internal/core/manifestedit` package inserts or deletes whole table blocks at
positions found with the same `go-toml/v2/unstable` parser that
`pkg/manifest` uses for error positions. After every edit it reparses the
result and checks that the decoded manifest equals the old one plus exactly the
intended change; anything else (inline tables, dotted keys defining the table,
an unexpected difference) is refused with a message to edit by hand. Bytes
outside the edited block are unchanged.

## 3. CLI surface

```text
kitbag agent list   [--json] [--probe]
kitbag agent add    <kind> [<dir>] [--name <n>] [--target <t>]
                    [--this-host | --all-hosts] [--settings=on|off]
                    [--dry-run] [--json] [--yes] [--commit] [--revision <r>]
kitbag agent remove <target>/<instance> [--this-host | --all-hosts] [--disable]
                    [--dry-run] [--json] [--yes] [--commit] [--revision <r>]
```

- `list` prints, per kind, the declared instances (target, name, directory,
  scope, settings merge, enabled, active on this host, presence), then
  undeclared candidates. `--host` selects whose view is shown, as everywhere.
- `add` defaults: `<dir>` is the kind's resolved default when undeclared, else
  required; `--name` is derived from the directory (`~/.claude_2` → `claude_2`)
  and validated with the manifest name grammar; scope is `--this-host` when
  the directory is present here, else `--all-hosts` must be given;
  `--settings` defaults to on; an explicit `--settings` is rejected for kinds
  or targets without a settings fragment.
- `remove` deletes the declaration; `--disable` writes `enabled = false`
  instead. The preview shows the removals the next sync will perform for files
  KitBag owns in that directory. Removing the last listed instance re-enables
  the implicit default, which the preview states.
- Both mutating commands show the manifest diff and the plan preview, then ask
  for confirmation on a terminal. Without a terminal they require `--yes` or
  `--dry-run`. `--dry-run` never writes and exits 0.
- JSON: `list` emits `agents_schema: 1`; `add`/`remove` emit
  `agent_change_schema: 1` with `change`, `manifest` (`path`,
  `revision_before`, `revision_after`, `diff` lines), `plan` (the
  `plan_schema` target and operation objects for affected targets), `warnings`,
  `written` and `commit` (null when none). Schemas go to `schemas/` and are
  checked like `plan.schema.json`.
- Exit codes keep the shared meanings: 0 success or dry run; 1 operational
  failure (for example written but the commit failed, reported as such);
  2 usage or validation (unknown kind, invalid name or path, duplicate
  identity, `outside_home`, non-interactive without `--yes`, a layout
  `manifestedit` refuses); 3 conflict (stale revision, dirty copy with
  `--commit`); 4 tool too old; 5 unsupported OS.

## 4. API methods and view models

Local API v1.1 adds three methods; older clients are unaffected and older
daemons answer method-not-found, after which the client uses the in-process
path, as status and plan do.

| Method | Params | Result |
| --- | --- | --- |
| `agents.list` | `env` (allow-listed relocation variables), `probe` | `AgentsSnapshot` |
| `agents.preview` | `AgentChange` | `AgentPreview` (diff, affected plan, warnings, `revision`) |
| `agents.apply` | `AgentChange`, `revision`, `commit` | `AgentApplied` (new revision, commit or null, warnings) |

`internal/ui/model` gains these types and `Client` methods
(`Agents`, `PreviewAgentChange`, `ApplyAgentChange`); [ui.md](../ui.md#proposed-agents-and-instances-adr-12)
lists them in full. Two models follow the existing state-machine pattern:
`Agents` (loading, ready, failed; selection follows (kind, dir) across
reloads) and `AgentWizard` (steps below; sequence-numbered previews so a slow
answer never replaces a newer draft). Field validation (name grammar, home-path
grammar, duplicate identity, name taken in the target, settings applicability)
runs in the model on every keystroke using validators exported from
`pkg/manifest` (`CheckName`, `CheckHomePath`; an additive public API change
recorded in `docs/api/manifest.export`), and again authoritatively on the
server. New error classes reuse existing kinds: stale revision → `conflict`,
refused layout and validation → `invalid`.

## 5. Terminal interface

A fifth tab, **5 Agents**, is appended so the v0.1.0 keys `1`–`4` keep their
meaning. It lists declared instances grouped by kind, then undeclared
candidates, with source, presence and scope as text (never colour alone).
Keys: `↑`/`↓`, `k`/`j`, Enter details, `a` add (prefilled from the selected
candidate), `x` remove, `p` look for more directories, `r` refresh.

The add wizard is a modal sequence; each step shows its title, a one-line
explanation and the validation message of the current draft:

1. **Agent kind:** native kinds, detected kinds first; `generic` is shown as
   available from the CLI only, because it needs explicit roots.
2. **Directory:** resolved candidates for that kind with presence, plus
   **Other path…**, a line editor accepting `~/` paths. Missing directories
   are allowed with the warning that sync skips them until the agent creates
   them; `outside_home` and duplicates block **Next**.
3. **Name and target:** derived name, editable; target chooser only when
   several targets of the kind exist.
4. **Scope:** this host (named) or all hosts; when relevant, the copy and
   existing-override notes from section 2.
5. **Settings merge:** on or off, only when the target declares a settings
   fragment and the adapter merges settings; otherwise an explanatory line.
6. **Preview:** the manifest diff and the affected plan (operations per
   destination, warnings such as the materialized default), loaded with
   `agents.preview`.
7. **Confirm:** **Write**, or **Write and commit** when the copy is clean;
   then the result with the push command and an offer to sync now (`s`).

Keyboard: Enter or `Ctrl-N` next, Esc or `Ctrl-P` back, Esc on step 1 cancels
(asking first when a draft exists), `Tab`/`Shift-Tab` between fields,
`↑`/`↓` in lists, Space or `←`/`→` toggles, `Ctrl-C` quits as everywhere. While
the line editor is focused, typed keys go to it. Mouse, once the tab-click work
of KIT-80 lands: click a tab, click a row to select, double-click to open,
wheel to scroll, and click the **Back**, **Next**, **Cancel** and **Write**
buttons. Every mouse action has a key; mouse reporting must not break SSH
sessions or `NO_COLOR` output.

## 6. Tests and staged implementation

Tests:

- **Discovery (core, fake platform):** variables set, unset, relative and
  outside HOME; directory, file, symlink, missing; implicit default versus
  explicit `dir`. A recording file system fails the test on any `ReadFile`,
  `Open` or `ReadDir` other than the probe's single HOME listing. Negative
  control: plant `.credentials.json` and `auth.json` and make discovery read
  them; the recorder must fail.
- **Manifest editing:** goldens over a copy of the maintainer's manifest and
  the schema examples, asserting byte equality outside the edited block;
  refusal of inline and dotted layouts; a fuzz target over the existing manifest
  corpus asserting "refuse, or reparse equals intent".
- **Preview equals sync:** on the fake host, the preview plan of a change equals
  the plan after writing it.
- **CLI (testscripts, throwaway HOME):** `agent list` with and without
  variables, JSON validated against its schema; `add --dry-run` leaves the
  manifest hash unchanged; `add`, then `plan` shows the instance, `sync`
  applies it and a repeat is silent; host scope and copied sets; implicit
  default materialization; `remove` and `--disable`; stale revision exits 3;
  `--commit` on a dirty copy exits 3; non-interactive without `--yes` exits 2.
- **API:** schema-generated real-socket contract cases for the three methods
  and method-not-found fallback against a v1.0 server.
- **Models:** wizard transitions, per-keystroke validation, stale preview
  discard, every new text key and argument count.
- **TUI (teatest):** golden screens for the Agents tab and each wizard step at
  80×24 and 40×10, `NO_COLOR`, cancel with a draft, and mouse clicks once
  available.

Staged plan, one focused change per PR (S under 200 changed lines, M up to
600, L up to 1200, excluding generated files and goldens):

| # | PR | Size | Depends on |
| --- | --- | --- | --- |
| 1 | `refactor(discover)`: shared home resolvers and `internal/core/discover`; `init` and `doctor` use it (fixes doctor for Gemini and OpenCode) | M | this ADR |
| 2 | `feat(agent)`: read-only `kitbag agent list` with `agents_schema` and `--probe` | M | 1 |
| 3 | `feat(manifest)`: exported `CheckName`/`CheckHomePath` | S | — |
| 4 | `feat(manifestedit)`: position-preserving table insert and delete with reparse verification and fuzzing | L | — |
| 5 | `feat(agent)`: `agent add`/`remove` with preview, revision, dry-run, JSON and `--commit` | L | 2, 3, 4 |
| 6 | `feat(api)`: `agents.list`/`preview`/`apply` (API v1.1) and the `apiclient` adapter | M | 5 |
| 7 | `feat(ui)`: `Agents` and `AgentWizard` models with texts | M | 3, 6 |
| 8 | `feat(tui)`: Agents tab and wizard | L | 7, KIT-80 mouse for clicks |
| 9 | `feat(agent)`: `--publish` through the per-host policy | M | KIT-53 |

### Consequences

- The maintainer can add `~/.claude_2` on one host from the TUI or CLI, see
  the diff and plan first, and keep comments in `kitbag.toml`.
- Credentials stay unread by construction, which tests enforce.
- Host-only additions under S2 copy instance sets, which can drift; `agent
  list` makes that visible until a schema 2 decides on S3.
- Until KIT-53, publishing remains a manual push, and an unpushed commit can
  stop sync on divergence; the CLI says so at the moment it matters.
- The local API grows by three methods and a minor version.

## Open questions for the maintainer

1. **Storage shape:** accept S2 host overrides (works with every v0.1 tool,
   but copies sets), or schedule a manifest schema 2 with a per-instance
   `hosts` filter (S3)? Is the current global `~/.claude_2` intended for every
   host, or should it move to a host override?
2. **Git:** should `add` write only (recommended until KIT-53), commit by
   default, or also push in `direct` mode? What should runner02, whose policy is
   `pr`, do before publication exists?
3. **Discovery breadth:** may an explicit `--probe` list HOME entry names to find
   siblings such as `~/.claude_2`, relaxing the "never lists HOME" rule of
   `init` when asked, or should discovery stay at defaults, variables and
   declarations?
4. Default scope when the directory exists here: this host (proposed) or all
   hosts?
5. Should `remove` restore the instance's original backups like `unmanage`, or
   only drop the declaration and let the planner remove owned outputs
   (proposed)?
6. Should KitBag help launch the second account (for example print a
   `CLAUDE_CONFIG_DIR=~/.claude_2 claude` alias hint), or is that out of scope?
7. Tab position: append as 5 (proposed) or insert after Drift and renumber?
8. Is a machine-local instance list needed for consumer-only machines without
   push rights, despite S4's rejection?

## Links

- [KIT-80](https://linear.app/privacybrand/issue/KIT-80/tui-clickable-tabs-detected-agents-view-add-agentsecond-account-wizard),
  design step (b).
- [Manifest instances and hosts](../manifest.md#targetsname),
  [first-run detection](../init.md#detection-and-existing-repositories),
  [shared view models](../ui.md), [terminal interface](../tui.md),
  [ADR 2](0002-git-source-of-truth.md), [ADR 5](0005-ui-layers.md),
  [ADR 10](0010-local-api.md).
