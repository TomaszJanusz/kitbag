# Claude Code adapter

`claude.New(home, directory)` constructs a `target.Adapter` with absolute,
clean, caller-resolved paths. An empty directory selects `<home>/.claude`.
The caller resolves `CLAUDE_CONFIG_DIR` with `platform.Env.AgentHome` before
construction; `Declare` never reads the environment. Explicit manifest instances
use their listed `~/` directories regardless of that default. They carry
`Target.InstanceDir`, so a missing listed directory warns and is never created.
Default directories may be created by apply.

The adapter declares one materialized entry per skill, command, subagent, hook
script or rule. Commands, subagents and rules accept Markdown files and directory
trees containing Markdown; hook files keep their native content and executable
bits. `per_agent.claude` replaces same-name shared entries. Roots and primitive
policy follow the manifest contract. `skills/synced` belongs to Claude Code:
it is never scanned for stale links, and a repository skill named `synced` is
rejected. Root overrides of any primitive may not overlap that reserved tree.
Instructions use the shared renderer, defaulting to `import`; explicit
`concat` is supported. Missing host additions are returned in `Adapter.Plan`
`Result.Warnings` without adding synthetic targets or suppressing instructions.
`Declare` implements the planner contract and returns the same targets; callers
that display nonblocking renderer warnings use `Plan`.

Settings are `merged` targets, never links, with `Backup: Once`; core owns backup,
drift, atomic writes and state. Instance `settings = false` declares a preserved
path: no source read, file operation or ownership change, even if that instance
was previously managed. This additive `Target.Preserve` contract prevents the
planner from interpreting opt-out as a stale-file deletion.

## Native JSON merge and migration

`claude.Merger{Repository: repository}.Merge(current, fragment)` is pure and
returns fresh bytes. Objects merge recursively, keeping local-only keys; repo
scalars win. Arrays retain local order and duplicates, then append repo entries
not already present. Object equality ignores key order; numeric equality compares
numeric values. Empty/missing current bytes mean `{}`. Invalid or non-object
settings and malformed hooks are rejected, producing a planner skip diagnostic
and leaving the file alone. Unsafe repository fragments are rejected too.

Hooks follow the legacy jq program's group ownership: if any command in a group
ends in a managed marker, the entire group is removed before the current repo
groups are appended. Both `# agent-config-managed` and `# kitbag-managed` are
recognized; newly written commands end in `# kitbag-managed`. Inline mentions
that are not trailing markers do not confer ownership. Foreign hook groups,
including prompt hooks, remain; repository hooks must have string commands and
must not declare a non-command type.

`{repo}` is the canonical placeholder in string values, expanded literally to
the absolute repository path. Legacy `@REPO@` is also accepted when reading repo
fragments. Neither object keys nor existing local-only values are expanded.
New fragments should use `{repo}`. No environment-variable expansion occurs.

The planner's existing conservative drift rules still apply: a simultaneous
source and local settings edit may conflict rather than overwrite local changes.
MCP and public `sync` CLI wiring remain separate work.

## Reproducing jq parity

The checked-in matrix under `internal/adapters/claude/testdata/merge` includes
empty input, foreign hooks, removed hooks, arrays, scalar overrides, invalid and
non-object JSON, both markers, both placeholders, non-command hooks, mixed hook
groups, Unicode and nested equality. Regenerate with Python 3 and jq on PATH:

```sh
python3 scripts/claude-goldens.py /path/to/agent-config/bin/agent-config-sync
mise exec -- go test ./internal/adapters/claude -run '^TestJQGoldens$'
```

The generator reads the actual script's `merge_program` without sourcing or
executing its synchronization commands. It emulates the shell's empty-file and
object checks. The filter runs unchanged; a small migration bridge maps the new
marker to the old one on input and maps trailing old markers to new ones on
output, and maps `{repo}` to the filter's legacy `@REPO@` placeholder. Each success
stores both raw `.legacy.json` output and normalized `.want.json`. Failures have
`.error.txt`; invalid files are left alone by the legacy shell precheck.

The Go formatter preserves insertion order, uses jq's two-space indentation,
compact empty collections, literal Unicode/HTML, control escaping and a final
newline. Numbers retain their input spelling, as jq 1.7 does for unmodified
literals. The generator used jq 1.7. Go tests compare bytes, verify input
immutability and verify a silent second merge. Regeneration is explicit and is
not part of `mise run check`; production merging needs neither Python nor jq.

The test-only apply driver accepts `--adapter claude` to run production declarations
through the real Unix apply engine in a throwaway HOME. Its script proves two
instances (one with settings off), backups, foreign-hook preservation, marker
migration, removal, reserved-link protection, opt-out after ownership, missing
instances and silent repeat runs. Installed CLI behavior is unchanged.
