# Manifest and machine configuration

KitBag reads two TOML files. `kitbag.toml` is committed at the root of a
configuration repository and describes, for every machine, which repository
files become which agent primitives. `config.toml` lives on one machine and
describes that machine only. This document is the schema version 1 contract
for both; [`schemas/kitbag.schema.json`](../schemas/kitbag.schema.json) and
[`schemas/config.schema.json`](../schemas/config.schema.json) are the
machine-readable form (JSON Schema draft 2020-12), and the TOML files under
[`schemas/examples`](../schemas/examples) are validated against them by
`mise run check` (`internal/schemacheck`).

Nothing here claims observed runtime behavior of any agent application. Where
a key refers to an agent's default directory or file format, the source is the
documentation inventory in [agents.md](agents.md); the implemented adapters and their remaining unknowns are documented separately.
The repository parser is `pkg/manifest`; machine configuration is `internal/config`.

## Three contracts, three lifecycles

[ADR 9](adr/0009-skill-lock-interchange.md) separates intent from resolution and
from application. The manifest covers only the first.

| Contract | File | Versioned by | Holds |
| --- | --- | --- | --- |
| Intent | `kitbag.toml` (this document) | `schema` | Sources, selectors, targets, hosts, policy. No resolved commits, no hashes, no machine paths other than `~/`-relative destinations. |
| Provenance | `kitbag-provenance.toml` (name reserved, schema not defined here) | its own `schema` | Canonical source, selected subpath, requested selector, resolved commit or immutable artifact identity, named hash algorithm, per imported skill. Committed next to the manifest. |
| Local state | `state.json` in the platform state directory (not defined here) | its own version | Destination paths, last-applied hashes, ownership, backup and revert journal, per machine. Never committed. |
| Interchange | `skills-lock.json` version 1 (skills CLI) | upstream | Read and export of an explicitly bounded subset, named by `skills.interchange`. Never sufficient for frozen installation. |

The manifest schema rejects the top-level keys `provenance` and `state` so that
they stay available for those contracts. Each contract has a fail-closed
reader: an unsupported version is an error, never an empty result and never a
destructive rewrite (ADR 9, condition 3).

## Versioning and reading rules

- `schema` is a required integer in both files. A tool release that knows
  version N reads N and N−1. Any other value is an error: a higher value means
  "this manifest needs a newer kitbag" and the tool stops before planning
  ([ADR 2](adr/0002-git-source-of-truth.md)); a lower value means "run the
  explicit migration", which preserves the original file.
- `min_tool_version` lets a repository require a newer tool than the schema
  alone implies. A tool older than it stops without applying anything.
- Unknown keys are errors everywhere (`additionalProperties: false`), including
  misspelled optional keys. A new key arrives only with a new schema version;
  the N/N−1 window gives repositories a release in which every machine can
  update before the manifest bumps `schema`.
- Missing optional tables mean "this primitive is not managed". Nothing is
  inferred from the repository tree.
- A file is rejected as a whole; there is no partial application of a manifest
  that fails validation.
- Both files are independent: the manifest version and the configuration
  version can differ.

## Path and name grammar

| Grammar | Rule | Used by |
| --- | --- | --- |
| repository path | Relative to the repository root, `/` separated, no leading `/`, `~` or `$`, no `.` or `..` segment, no backslash, no trailing `/`. | every `src`, `source`, `dir`, `dirs`, `include`, `settings`, `per_agent` value |
| home path | Starts with `~/`, same segment rules. `~` is the applying user's home directory on that machine. Absolute paths are rejected. | `dest`, instance `dir`, `roots`, `repo.path` |
| name | `^[a-z0-9][a-z0-9_.-]{0,63}$` | target, instance, server and import names |
| host name | `^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$` | `hosts.<name>`, `files[].hosts`, config `host` |
| agent kind | `claude`, `codex`, `cursor`, `gemini`, `opencode`, `copilot`, `windsurf`, `generic` | `agent`, `per_agent` keys, `mcp.servers.<n>.agents` |
| primitive name | `instructions`, `skills`, `commands`, `agents`, `hooks`, `rules`, `mcp` | `roots` keys, `primitives` keys |
| semantic version | `MAJOR.MINOR.PATCH` with optional pre-release and build | `min_tool_version` |
| duration | one or more `<integer><ms|s|m|h>` groups, for example `15m` or `1h30m` | `sync.poll_interval` |

Template placeholders are literal `{host}` (the selected host name) in
`instructions.hosts` and `{repo}` (absolute working copy path) inside settings
fragments. No other placeholder or environment variable is expanded.

## `kitbag.toml` reference

### Top level

| Key | Type | Required | Default | Meaning |
| --- | --- | --- | --- | --- |
| `schema` | integer | yes | — | Must be `1`. |
| `min_tool_version` | semantic version | no | none | Lowest tool release allowed to apply the manifest. |
| `mode` | `link` \| `copy` | no | unset | Manifest-wide materialization default; see [mode](#materialization-mode). |
| `instructions` | table | no | not managed | Global instructions. |
| `skills` | table | no | not managed | Skill directories and import intent. |
| `commands`, `agents`, `hooks`, `rules` | collection table | no | not managed | Native-format file collections. |
| `mcp` | table | no | not managed | MCP servers defined once. |
| `files` | array of tables | no | none | Arbitrary files. |
| `git` | table | no | not managed | Git configuration includes. |
| `targets` | table of targets | no | none | Agents that receive primitives. Without targets only `files` and `git` apply. |
| `hosts` | table of hosts | no | none | Per-host overrides. |
| `publish` | table | no | `mode = "direct"` | Publication policy for `kitbag push`. |

### `[instructions]`

| Key | Type | Required | Default | Meaning |
| --- | --- | --- | --- | --- |
| `source` | repository path | yes | — | The shared always-loaded instructions file (Markdown). |
| `hosts` | repository path template containing `{host}` | no | none | Per-host additions appended after `source`. A missing file for the selected host is reported and skipped; it is not an error. |
| `per_agent.<kind>` | repository path | no | none | Replaces `source` for that agent kind (the dotagents pattern). Host additions still apply. |

Each enabled target renders one managed instructions file at the adapter's
documented instructions root, or at the instance's `roots.instructions`. The
render style is `primitives.instructions.render`:

- `import`: the file imports the source files with the agent's native import
  syntax, so editing the source changes the next session without a sync.
- `concat`: the file holds the concatenated content of the source and the host
  additions, in that order, after a managed-file marker.
- Unset: the adapter's documented default. Claude Code and Gemini CLI document
  file imports; Codex and `generic` have none, so `concat` is their default.
  Requesting `import` for an adapter without documented import syntax is a
  plan-time error, not a silent fallback.

[instructions.md](instructions.md) describes the shared renderer, its marker
and warnings.

### `[skills]`

| Key | Type | Required | Default | Meaning |
| --- | --- | --- | --- | --- |
| `dirs` | array of repository paths, non-empty, unique | yes | — | Each child directory containing `SKILL.md` is one skill. Names must be unique across all entries; adapters may reserve names (Claude Code reserves `synced`). |
| `interchange` | repository path | no | none | A skills CLI `skills-lock.json` version 1. KitBag may read it and export to it as a bounded subset; it carries no authority over what is applied. |
| `imports.<name>` | table | no | none | Import intent for the vendored skill `<name>`. |
| `imports.<name>.source` | string without whitespace | yes | — | GitHub shorthand `owner/repo`, an `https://` or `ssh://` URL, or `git@host:path`. The supported subset is settled by the skills work, not widened here. |
| `imports.<name>.path` | repository path | no | source root | Directory of the skill inside the source repository. |
| `imports.<name>.ref` | Git ref | no | source default branch | Requested selector (branch or tag). Intent only: the resolved commit belongs to provenance. |
| `imports.<name>.dir` | repository path | no | first entry of `dirs` | Which `dirs` entry holds the vendored copy. Must be one of `dirs`. |

Every skill, imported or local, is materialized as one entry per skill at each
enabled target's documented skills root (for example `<dir>/skills/<name>` for
Claude Code and the shared `~/.agents/skills/<name>` documented for Codex and
several other agents), or at the instance's `roots.skills`. Several targets
that resolve to the same destination produce one operation with shared
ownership; two different sources for one destination are a plan-time error.

### Collections: `[commands]`, `[agents]`, `[hooks]`, `[rules]`

| Key | Type | Required | Default | Meaning |
| --- | --- | --- | --- | --- |
| `dir` | repository path | at least one of `dir`, `per_agent` | none | Shared directory; each immediate child is one entry. |
| `per_agent.<kind>` | repository path | | none | Directory whose entries are added for that kind only and replace same-name entries from `dir`. |

Entries keep their native format. An entry is applied to a target only when
that adapter declares the entry's format native for the primitive (for example
Markdown with YAML frontmatter for Claude Code commands, TOML for Gemini CLI
commands); otherwise the plan reports the entry as unsupported for that target.
Version 1 performs no format conversion; `per_agent` is the way to provide a
different format per agent. `hooks` covers hook files only; hooks that live
inside an agent's settings file are part of the target's `settings` fragment.

### `[mcp]`

| Key | Type | Required | Default | Meaning |
| --- | --- | --- | --- | --- |
| `policy` | `merge` \| `replace` | no | `merge` | `merge` keeps servers the manifest does not mention and overwrites same-name servers. `replace` makes the native server set equal to the manifest's. In both, servers KitBag previously wrote (per local state) and that are no longer declared are removed. |
| `servers.<name>` | table | yes | — | One server, rendered to every supporting target's native file and format. |
| `servers.<name>.transport` | `stdio` \| `http` \| `sse` | no | `stdio` with `command`, `http` with `url` | Transport. |
| `servers.<name>.command` | string | stdio only | — | Executable. Exactly one of `command` and `url` is required. |
| `servers.<name>.args` | array of strings | stdio only | `[]` | Arguments. |
| `servers.<name>.env` | table of strings, keys are environment variable names | stdio only | `{}` | Literal variables written into the native file. Never secrets. |
| `servers.<name>.url` | `http(s)://` URL | remote only | — | Endpoint. |
| `servers.<name>.headers` | table of strings | remote only | `{}` | Literal headers. Never secrets. |
| `servers.<name>.secret_env` | array of environment variable names | no | `[]` | Variables the server needs whose values must come from the machine (its environment or credential store), never from the repository. How a machine supplies them is machine-local. |
| `servers.<name>.agents` | array of agent kinds | no | all supporting targets | Restricts rendering to these kinds. |

Only the server definition keys are written; other content of the native file
(including state an agent stores next to its servers) is preserved. The native
file and key for each adapter are documented in [agents.md](agents.md). Where
the inventory marks a location unknown (Claude Code's user MCP file under a
relocated configuration directory), the instance must set `roots.mcp` or the
plan reports MCP as unsupported for that instance.

### `[[files]]`

| Key | Type | Required | Default | Meaning |
| --- | --- | --- | --- | --- |
| `src` | repository path | yes | — | File or directory in the repository. |
| `dest` | home path | yes | — | Destination. |
| `mode` | `link` \| `copy` | no | unset | Per-file mode; the most specific setting. |
| `hosts` | array of host names | no | all hosts | Apply only on these hosts. |

### `[git]`

| Key | Type | Required | Default | Meaning |
| --- | --- | --- | --- | --- |
| `include` | array of repository paths, non-empty, unique | yes | — | Each path is added as an `include.path` of the user's global Git configuration when not already present; existing entries are never removed. |

### `[targets.<name>]`

| Key | Type | Required | Default | Meaning |
| --- | --- | --- | --- | --- |
| `agent` | agent kind | yes | — | Adapter that supplies default roots, formats and merge rules. |
| `enabled` | boolean | no | `true` | `false` keeps the target declared but produces no operations. |
| `mode` | `link` \| `copy` | no | unset | Target-level mode. |
| `settings` | repository path | no | none | Native settings fragment merged into each instance with `settings` enabled. Not allowed for `generic`. Adapters without a documented merge implementation report it as unsupported. |
| `primitives.<primitive>` | table | no | all supported | `enabled` (default `true`), `mode`, and for `instructions` also `render`. |
| `instances.<name>` | table | `generic`: yes; others: no | the adapter's single documented default directory | Configuration directories of this agent. |
| `instances.<name>.dir` | home path | yes | — | The configuration directory itself (for example `~/.claude_2`). Never a relocation variable: a relocated instance is declared by its actual directory. |
| `instances.<name>.enabled` | boolean | no | `true` | |
| `instances.<name>.mode` | `link` \| `copy` | no | unset | |
| `instances.<name>.settings` | boolean | no | `true` | `false` skips the settings merge for this instance (today's `settings=off`). |
| `instances.<name>.roots.<primitive>` | home path | `generic`: yes | adapter default | Replaces the adapter's documented root for one primitive on this instance. For `generic` only primitives with a root are applied. |
| `instances.<name>.primitives.<primitive>` | table | no | target value | Same keys as the target level; wins over it. |

Two targets may use the same agent kind (for example two `generic` targets).
Instance identity is the pair (agent kind, `dir`); the same pair declared twice
is a plan-time error. The `copilot` kind denotes the `~/.copilot` home contract
shared by Copilot CLI and the VS Code Copilot harness; the VS Code local
harness and Windsurf's Devin-era layout are not kinds of their own and must
be declared explicitly (`generic` with `roots`, or a separate instance `dir`),
never inferred from an existing instance.

#### Settings merge rules

The fragment is in the agent's native format (JSON for Claude Code). Merging
into the instance's settings file follows today's `agent-config-sync` rules:

1. Objects merge key by key; a scalar from the repository wins, so a local edit
   of a key the fragment defines is overwritten on the next sync. Keys the
   fragment does not define are left alone.
2. Arrays are unioned: local additions survive, and removing an entry from the
   fragment does not remove it from the instance.
3. `hooks` entries must be `command` hooks. Each written command carries the
   trailing marker `# kitbag-managed` (the old `# agent-config-managed` marker
   is recognized for migration); every sync removes hook groups carrying either
   marker and adds the current ones, so deleting a hook in the fragment removes
   it everywhere and hooks written by other tools stay. Hooks that an
   application installs itself must not be duplicated in the fragment.
4. `{repo}` in string values expands to the absolute working copy path. Claude
   also accepts legacy `@REPO@` in fragments; new fragments use `{repo}`.
5. The settings file is never linked; the first change leaves a one-time
   backup, as recorded in local state.

Marker migration: hooks carrying the previous `# agent-config-managed` marker
are not recognized as managed; remove them before the first KitBag sync.

### `[hosts.<name>]`

| Key | Type | Required | Default | Meaning |
| --- | --- | --- | --- | --- |
| `mode` | `link` \| `copy` | no | unset | Host-level mode. |
| `publish` | publish table | no | top-level `publish` | Per-host publication policy. |
| `targets.<target>.enabled` | boolean | no | target value | Drop or keep a target on this host. |
| `targets.<target>.mode` | `link` \| `copy` | no | target value | |
| `targets.<target>.instances.<name>` | instance table | no | target instances | Replaces the target's whole instance set on this host (no partial merge). |

The host name comes from `config.toml` `host`, else the machine's short host
name. A host without a `hosts.<name>` table uses the defaults; this is not an
error. `files[].hosts` and `instructions.hosts` use the same name.

### `[publish]`

| Key | Type | Required | Default | Meaning |
| --- | --- | --- | --- | --- |
| `mode` | `direct` \| `pr` | no | `direct` | `direct` pushes to `branch`. `pr` pushes a `kitbag/{host}/{slug}` branch and opens a pull request against `branch`. |
| `branch` | Git ref | no | remote default branch | Branch that receives published changes. |

`hosts.<name>.publish` overrides both keys for that host. Publication never
force-pushes, rewrites history or resolves conflicts (plan §16.4).

### Materialization mode

`mode` may be set at the manifest, host, target, host-target, instance,
primitive and instance-primitive level, and on each `files` entry. Resolution:

1. Within the manifest the most specific setting wins, in the order manifest →
   host → target → host-target → instance → target primitive → instance
   primitive (`files[].mode` stands alone for its entry).
2. The machine supplies a default where the manifest leaves it unset:
   `--mode`, then `KITBAG_MODE`, then `config.toml` `sync.mode`, then the
   platform default (`link` on macOS and Linux, `copy` on Windows,
   [ADR 3](adr/0003-materialization-and-platform.md)).
3. `copy` always wins: if either the manifest result or the machine says
   `copy`, the operation copies. A repository therefore declares `copy` for an
   agent or primitive whose link support is unknown, and a machine can force
   `copy` everywhere, but neither can force `link` where the other refuses it.
4. A platform that cannot create the requested link reports the operation as
   unsupported. There is no silent fallback.

Link capability is tracked per primitive and operating system in
[agents.md](agents.md); where it is unknown the safe declaration is `copy`.

## Precedence summary

- Instructions: `per_agent.<kind>` replaces `source`; `hosts` additions are
  appended for the selected host.
- Collections: `per_agent.<kind>` entries win over same-name `dir` entries.
- MCP: manifest definitions overwrite same-name native servers; `policy`
  decides whether unmentioned native servers survive.
- Settings: repository scalars win, arrays union, managed hooks replaced.
- Instances: `hosts.<name>.targets.<t>.instances` replaces the target's set;
  `enabled = false` at any level removes the operations below it.
- Mode: most specific manifest level, then machine default, and `copy` wins.
- Publish: `hosts.<name>.publish` over `publish`.

## Security notes

- No secrets in either file. MCP `env` and `headers` are literal and
  committed; name secret variables in `secret_env` instead. `repo.url` carries
  no credentials. `kitbag push` scans for secrets before committing (plan §16.2).
- Every repository path is validated by grammar before use: no `..` segment,
  no absolute path, no `~` or `$` prefix, no backslash. Destinations are `~/`
  relative only, so a manifest cannot address system directories or another
  user's home. The grammar is enforced by the schema and again by the parser.
- Sources are read from the Git working copy only. Symbolic links inside a
  source tree are not followed in `copy` mode; the plan reports them. In
  `link` mode the destination link points at the working copy path itself.
- Destinations: an existing unmanaged file, directory or foreign link is never
  replaced silently. It is reported, adopted on request, or backed up once
  according to local state ownership (ADR 3). Managed links are replaced only
  when they point into the working copy or dangle.
- Hook commands and skill scripts are executable content; the manifest only
  places them. Review them in the pull request, and expect `kitbag push` to
  flag new executables and hook changes.
- Applying a manifest never requires elevated privileges.

## Example: today's `agent-config` layout

This manifest reproduces what `bin/agent-config-sync` does for the
`TomaszJanusz/agent-config` repository without moving a file. It is the fixture
`schemas/examples/kitbag/valid/agent-config.toml`, validated by the quality gate.

<!-- example: schemas/examples/kitbag/valid/agent-config.toml -->
```toml
# kitbag.toml reproducing today's TomaszJanusz/agent-config layout without
# moving a file. Everything bin/agent-config-sync does is declared here; the
# script's self-link into ~/.local/bin is the script's own concern and has no
# counterpart.
schema = 1
min_tool_version = "0.1.0"

[instructions]
# global/AGENTS.md is loaded into every session of every agent.
source = "global/AGENTS.md"
# hosts/mac.md and hosts/runner02.md are appended for that host only.
hosts = "hosts/{host}.md"

[skills]
# Every child directory with a SKILL.md is one skill; names must be unique
# across both directories. ~/.agents/skills and <claude dir>/skills receive one
# link per skill (the adapters' documented default roots).
dirs = ["skills", ".agents/skills"]
# The skills CLI lock stays readable and exportable as a bounded subset.
interchange = "skills-lock.json"

# Import intent for vendored skills with a documented upstream directory
# (skills/SOURCES.md). Skills without one (caveman*, copywriting,
# frontend-design, linear-cli, release-manager) are plain local skills.
[skills.imports.agent-browser]
source = "vercel-labs/agent-browser"
path = "skills/agent-browser"
dir = ".agents/skills"

[skills.imports.gh-stack]
source = "github/gh-stack"
path = "skills/gh-stack"

[skills.imports.notion-cli]
source = "makenotion/skills"
path = "skills/notion-cli"

[[files]]
src = "agent-browser/config.json"
dest = "~/.agent-browser/config.json"

[git]
# Added as include.path to the global Git configuration if missing.
include = ["git/agent.gitconfig"]

[targets.claude]
agent = "claude"
# Merged into <dir>/settings.json of each instance, never linked.
settings = "claude/settings.json"

[targets.claude.instances.main]
dir = "~/.claude"

[targets.claude.instances.second]
dir = "~/.claude_2"

[targets.codex]
agent = "codex"
# Codex gets AGENTS.md (concatenated) and the shared ~/.agents/skills root;
# Codex settings are not synchronized today.

[targets.codex.instances.main]
dir = "~/.codex"

[publish]
mode = "direct"

[hosts.mac]
publish = { mode = "direct" }

[hosts.runner02]
# Agents on runner02 propose changes as pull requests.
publish = { mode = "pr" }
```

What each part of today's script maps to:

| Script behavior | Manifest |
| --- | --- |
| Links `skills/*` and `.agents/skills/*` into `~/.agents/skills` and each `<claude dir>/skills` | `skills.dirs` plus the documented default skills roots of the `codex` and `claude` adapters |
| Writes `<codex dir>/AGENTS.md` as global instructions plus `hosts/<host>.md` | `instructions` with the Codex default `concat` render |
| Writes `<claude dir>/CLAUDE.md` with `@` imports of the same files | `instructions` with the Claude default `import` render |
| Merges `claude/settings.json` into each instance (`settings=off` opts out) | `targets.claude.settings`, instance `settings = false` |
| Adds `git/agent.gitconfig` as `include.path` | `git.include` |
| Links `agent-browser/config.json` to `~/.agent-browser/config.json` | `files` |
| Reads `hosts/<host>.instances` | `targets.<t>.instances` and `hosts.<h>.targets.<t>.instances` |
| `AGENT_CONFIG_HOST`, `AGENT_CONFIG_INSTANCES` | `config.toml` `host`; instance overrides live in the manifest |
| Pull with `--ff-only`, skipped on local changes | `config.toml` `sync` and the tool's sync cycle (ADR 2) |
| Links itself into `~/.local/bin` | No counterpart; the tool is installed separately (ADR 6) |
| `--adopt`, one-time backups | Apply-time policy recorded in local state, not manifest keys |

## Example: minimal manifest

<!-- example: schemas/examples/kitbag/valid/minimal.toml -->
```toml
# The smallest useful manifest: one instructions file for Claude Code.
schema = 1

[instructions]
source = "AGENTS.md"

[targets.claude]
agent = "claude"
```

## `config.toml` reference

Location: `~/.config/kitbag/config.toml` on macOS and Linux (the Windows
location follows the platform work in M6). The everyday CLI supports
`--config`/`KITBAG_CONFIG`, `--host`/`KITBAG_HOST`,
`--git-backend`/`KITBAG_GIT_BACKEND` and `--mode`/`KITBAG_MODE`;
flag, then variable, then file, then default is the precedence ([ADR 4](adr/0004-git-backends.md) fixes it for
`git.backend`). The file is machine-local and never committed to the
configuration repository.

| Key | Type | Required | Default | Meaning |
| --- | --- | --- | --- | --- |
| `schema` | integer | yes | — | Must be `1`; same N/N−1 rule as the manifest. |
| `host` | host name | no | short host name of the machine | Selects `hosts.<name>` and `{host}` in the manifest. |
| `repo.url` | string without whitespace | yes | — | Clone URL (`https://`, `ssh://`, `git@host:path`). No credentials. |
| `repo.branch` | Git ref | no | remote default branch | Branch to follow. |
| `repo.path` | home path | no | platform data directory, for example `~/.local/share/kitbag/repos/<name>` | Working copy location. An existing clone can be reused. |
| `git.identity.name` | nonempty string without angle brackets or line breaks | no | Git configuration | Fallback unsigned commit author name for embedded Git; supply name and email together. |
| `git.identity.email` | email string without whitespace or angle brackets | no | Git configuration | Fallback unsigned commit author email for embedded Git. |
| `git.backend` | `auto` \| `system` \| `embedded` | no | `auto` | Git backend selection (ADR 4). |
| `sync.poll_interval` | duration | no | `60s` | Daemon fetch interval. |
| `sync.mode` | `link` \| `copy` | no | platform default | Machine default where the manifest leaves mode unset. |
| `ui.theme` | `auto` \| `dark` \| `light` | no | `auto` | TUI and window theme. |
| `ui.color` | `auto` \| `always` \| `never` | no | `auto` | Terminal color output. |
| `ui.notifications` | boolean | no | `true` | System notification after changes are applied. |
| `migration.legacy_roots` | unique array of home paths | no | `[]` (strict) | Explicit old agent-config clone/shared-skill roots eligible for legacy link replacement. No locations are guessed. |

`git.identity` is an additive schema-1 setting. Embedded Git reads the ordinary
Git configuration first and uses this fallback when fields are missing; a
missing complete identity stops publishing before staging. It does not edit
`.gitconfig`. System Git continues using its own identity, hooks and signing.

<!-- example: schemas/examples/config/valid/identity.toml -->
```toml
schema = 1
[repo]
url = "https://example.com/config.git"
[git]
backend = "embedded"
[git.identity]
name = "KitBag Publisher"
email = "publisher@example.com"
```

`migration.legacy_roots` is an additive schema-1 machine setting introduced
before the first release. Supply the actual old clone and shared-skill roots
only after reviewing them; the default is no migration roots. Each `~/` path
uses the home-path grammar above. The CLI expands it through platform HOME and
passes it to planning/apply. With roots configured, the existing legacy policy
also permits dangling links; live links outside those roots stay protected.

<!-- example: schemas/examples/config/valid/legacy-roots.toml -->
```toml
schema = 1
[repo]
url = "git@github.com:example/config.git"
[migration]
legacy_roots = ["~/src/agent-config", "~/.agents/skills"]
```

The paths above are examples, not inferred defaults. The maintainer's actual
migration paths are still an open deployment question. Other UI and polling
settings remain reserved for their respective features; no daemon is implemented.

Publication policy is deliberately not in `config.toml`: it is the repository
owner's rule per host and lives in the manifest.

<!-- example: schemas/examples/config/valid/runner02.toml -->
```toml
# ~/.config/kitbag/config.toml on runner02: follows agent-config and syncs
# every 15 minutes with system Git.
schema = 1
host = "runner02"

[repo]
url = "git@github.com:TomaszJanusz/agent-config.git"
branch = "main"

[git]
backend = "system"

[sync]
poll_interval = "15m"
mode = "link"

[ui]
color = "auto"
notifications = false
```

## Validation in the quality gate

`internal/schemacheck` compiles both schemas with a draft 2020-12 validator,
decodes every TOML example into the JSON data model and checks that each
`valid` example passes and each `invalid` example fails with the error named on
its first line (`# expect: <fragment>`). It also checks that the example blocks
above are byte-identical to the fixtures. `mise run check` runs it, so CI does
too once KIT-9 lands. Adding a key to a schema therefore requires an example
that uses it and, where a constraint is added, an invalid example that violates
it.

## Inventory review notes

Each manifest implication from [agents.md](agents.md#manifest-implications-for-kit-4)
and the review inputs on KIT-4, mapped to the key that addresses it.

| Input | Addressed by |
| --- | --- |
| 1. Model resource roots independently (Codex skills under `~/.agents`, Claude MCP outside the settings tree) | `instances.<n>.roots.<primitive>` per instance; adapters carry a documented default root per primitive, not one prefix. The example relies on the documented Codex skills root. |
| 2. Keep agent, client surface, instance and version distinct; do not reinterpret an existing instance | Instance identity is (agent kind, `dir`); `copilot` is defined as the `~/.copilot` contract only; VS Code local harness and Windsurf's redirected Devin layout need an explicit `generic` target or a separate `dir`. No kind is inferred from a directory. |
| 3. Separate placement from structured merging and activation | File collections place files; `settings` fragments and `mcp` have written merge rules (`policy`, ownership from local state); hooks in settings follow the marker rule. Instruction precedence inside an agent is not modelled. |
| 4. Preserve native formats; unknown primitives are explicit unsupported results | Collections keep native formats, `per_agent` supplies per-format sources, no conversion in version 1; unsupported entries, `import` without import syntax and `settings` on `generic` are plan-time errors, not defaults. |
| 5. Relocation needs per-variable semantics | The manifest never names a relocation variable; `dir` is always the configuration directory itself, and the variable semantics stay in agents.md and the adapters. |
| 6. Track link capability per primitive and OS; allow copy per instance/primitive | `mode` at every level down to instance primitive, `copy` wins, platform refusal is explicit. |
| 7. Keep authored configuration apart from runtime state | Only named sources are managed: skill entries, instruction files, settings keys, MCP server definitions, listed files. Whole-root synchronization has no key. |
| Windsurf documentation redirects to Devin | `windsurf` keeps today's documented `~/.codeium/windsurf` layout; a Devin layout is a separate instance or `generic` target. Deferred: a dedicated kind until a named installed release is verified. |
| VS Code harness variants | Not a kind in version 1; `generic` with explicit roots. Deferred: a `vscode` kind once the profile storage contract is documented. |
| ADR 9: manifest carries intent only | `skills.imports` holds source, path, ref and dir only; no commit, hash or host path. |
| ADR 9: provenance is a separate versioned contract | Name `kitbag-provenance.toml` and manifest key `provenance` reserved; schema deferred to the skills work. |
| ADR 9: local state is separate; fail-closed readers | `state.json` not defined here; N/N−1 rule with explicit errors for every other version, no empty result. |
| ADR 9: skills-lock.json v1 is an explicit read/export subset; no apm.lock | `skills.interchange`; no APM lock key exists. |

## Deferred and open

- Project scope (`.agents/` in a project repository) and layered repositories
  (plan open question 7) have no keys; `scope` and `layers` are not reserved.
- MCP `secret_env` names the secrets; how a machine supplies them (keyring,
  environment) is machine-local design for the adapters.
- Hooks have no cross-agent structured model; native files and settings
  fragments are the only inputs in version 1.
- The supported subset of `skills.imports.<n>.source` notations and the
  collision policy for same-name skills from different sources are settled by
  the skills work (ADR 9, condition 1).
- Format conversion between agents (for example Markdown command to Gemini
  TOML) is not planned for version 1.
