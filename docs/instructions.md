# Instruction rendering

`internal/adapters/instructions` is the only code that formats global
instructions. Adapters pick a strategy and a destination; they never build
instruction bytes themselves. The package reads through the supplied
`manifest.FileSystem` only, never reads the destination, and is deterministic.

```go
strategy, err := instructions.Choose(render, instructions.Import, instructions.Import, instructions.Concat)
result, err := instructions.Declare(fsys, instructions.Request{
	Instructions: resolved.Instructions, // from Manifest.ResolveHost
	Repository:   repository,
	Path:         destination,
	Agent:        "claude",
	Strategy:     strategy,
	Owners:       owners,
})
// result.Target is a target.Target; result.Warnings are non-blocking.
```

## Inputs

- `instructions.source` is the shared file.
- `instructions.per_agent.<kind>` replaces `source` entirely for that agent
  kind. Host additions still apply, as documented in [manifest.md](manifest.md).
- `instructions.hosts` must already be host-resolved (`{host}` expanded). The
  host addition follows the source. A missing host file adds a warning to
  `Result.Warnings` and renders the source alone; it is not an error.

Inputs are checked with `Lstat`. A missing source, a source or host addition
that is not a regular file (links included), or `link` with a host addition
produces a declaration with `Target.Warning`, so the planner skips it and keeps
any previous ownership. Other read errors fail.

## Strategies

| Strategy | Declaration | Content |
| --- | --- | --- |
| `import` | `generated` | Marker, blank line, one `@<absolute path>` line per input (Claude Code, Gemini CLI). |
| `concat` | `generated` | Marker, then each input separated by a blank line, one final newline (Codex, `generic`). |
| `link` | `materialized` | The effective source itself, linked or copied according to the mode. It cannot include a host addition. |

`Choose` maps `primitives.instructions.render` (`import`, `concat` or unset)
to a strategy and rejects one the adapter does not support. Adapters that
prefer `link` pass it as their default.

`import` and `concat` output is byte-identical to `agent-config-sync`'s
`CLAUDE.md` and `AGENTS.md` except for the marker line. Like the script's
`$(cat file)`, `concat` drops every trailing newline of each input before
joining. `testdata/legacy` holds the script's own output for the fixture, and
`TestLegacyCompatibility` compares against it.

## Marker

Generated files start with this line:

```text
<!-- Managed by kitbag. Edit the source in the configuration repository, not this file. -->
```

It contains no host-specific path, so output is the same wherever the working
copy lives. `Managed` recognizes it as the first line. Ownership still comes
from local state, not the marker: a file carrying the old `agent-config-sync`
marker is unmanaged to KitBag and is backed up once.

## Backups

Every declaration uses `Backup: Once`. The planner backs up an unmanaged
regular file once to `<destination>.kitbag-backup` ([plan.md](plan.md)). This
package does not back up anything itself. Unlike the script, an empty
unmanaged file is also backed up, and an existing destination link is never
replaced or followed.

## Golden files

Golden files live in `internal/adapters/instructions/testdata/golden`. After
reviewing a change, update them explicitly:

```sh
mise exec -- go test ./internal/adapters/instructions -run '^TestGolden$' -args -update
```
