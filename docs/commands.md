# Everyday commands

The [generated CLI reference](cli/kitbag.md) lists the Cobra tree and flags.
Regenerate it with `mise run docs:cli`. `mise run docs:cli:check` checks freshness
without writing; `TestCLIReference` also runs in `mise run check` and CI, comparing
every generated page byte for byte and rejecting stale extra files. Generated
pages omit timestamps. The generator is test-only; it does not add a public CLI
command.

Start with [`kitbag init <git-url>`](init.md), or configure an existing clone in
machine-local `config.toml`:

```toml
schema = 1
[repo]
url = "git@github.com:example/agent-config.git"
path = "~/src/agent-config"
[git]
backend = "system"
[sync]
mode = "link"
```

`repo.path` defaults to `vcs.WorkingCopyDir`, the platform data directory plus
`repos/<URL basename without .git>`. Init prepares that clone and writes machine
configuration after displaying a dry plan; sync can also create a missing clone.
When `repo.branch` is set, sync refuses a different checked-out branch before
fetching or applying; it never switches branches automatically. The
configuration parser rejects unknown fields, invalid values and unsupported
schemas. An absent default config is allowed for `version`; everyday commands
require a repository URL. An explicit missing config is an error.

- `sync`: hold the state lock, fetch and fast-forward a clean clone, read the new
  manifest, check `min_tool_version`, resolve host/mode, recover pending work,
  build **one** plan, execute **once**, then ensure Git includes and record the
  result. No second plan or apply pass occurs. A dirty clone skips pull before
  fetch and prints a short message to stderr. `--no-pull` skips all remote work.
  Divergence stops before apply. `--force` only grants the existing bounded
  materialized-drift backup policy. An unchanged success writes nothing to either
  stream, including with `--quiet`; the last-sync record still advances.
- `plan`: plan the existing tree without fetching, applying, recovering or saving
  ownership. Human output is a logical target diff (`+` write/materialize/merge,
  `-` removal, `~` backup/metadata, `!` skip), without file contents. Colour requires
  stdout to be a terminal according to `platform.IsTerminal` and `NO_COLOR` to be
  unset (even an empty `NO_COLOR` disables it); `ui.color = "never"` also disables
  it. `ui.color = "always"` cannot force colour into a pipe. `--json` emits
  `plan_schema: 1`, validated against [the schema](../schemas/plan.schema.json)
  by testscript using actual process output. Arrays are always arrays.
- `status`: report cached upstream ahead/behind, working-copy dirtiness, last sync
  time/exit result, all five planner classification counts, and warnings.
  `--json` emits `status_schema: 1`, `repository`, `git`, `last_sync`, `drift` and
  `warnings`; absent sync is null and empty warnings are an array. It never
  fetches; remote counts reflect the last fetch. It uses the same read-only plan
  and refuses pending recovery or an unreadable/incompatible manifest/state.

Global flags work before or after the command: `--config`, `--host`,
`--git-backend`, `--mode`, `--verbose`, `--quiet`. `--config` beats `KITBAG_CONFIG`,
then the platform config directory. Host selection is flag → `KITBAG_HOST` →
config `host` → short hostname. Git backend selection is flag →
`KITBAG_GIT_BACKEND` → config `git.backend` → auto. Machine mode selection is flag
→ `KITBAG_MODE` → config `sync.mode` → link; manifest copy still wins. Forced Git
backends are checked even by `version`, retaining the documented failure messages.
`--verbose` reports backend resolution and clone location to stderr. `--quiet`
suppresses successful sync changes; warnings/errors and dirty-pull messages remain
visible. The two output flags are mutually exclusive. JSON stays on stdout;
verbose diagnostics and adapter warnings use stderr.

Exit codes remain 0 success, 1 warning/operational failure, 2 usage/configuration,
3 conflict, 4 tool too old and 5 unsupported OS. Read-only inspection exits 0 when
it succeeds even when it describes drift. A development build reports `dev` and
checks minimum versions as `0.0.0-dev`; it cannot bypass a newer-tool requirement.
Windows is refused before any command runs. Service and daemon installation
remain planned for M2; init prints the manual sync command.

See [last-sync persistence](state.md#last-sync-attempt-kit-23),
[explicit migration roots](manifest.md#configtoml-reference), and
[Git include safety](generic.md).

## Doctor

`kitbag doctor` diagnoses local prerequisites and remote reachability, grouping
OK/WARN/FAIL results with fix hints. `--json`, `--strict` and a local, redacted
`--report <path>` are described in [doctor.md](doctor.md). Report mode skips
network probes and never uploads data.

## Adopt, unmanage and revert

`kitbag adopt` takes over destinations that agent-config-sync already manages
with zero file changes (`--dry-run`, `--json` with adopt_schema 1,
`--move-real-dirs`); it requires `migration.legacy_roots`. `kitbag unmanage`
removes what KitBag created and restores what it replaced (`--dry-run`,
`--dry-run --json`). `kitbag revert` restores the targets from before the latest
journaled plan, or the latest `--to <n>` (`--force`, `--dry-run`, `--list`).
None of them fetches. See [adopt.md](adopt.md).

## Local API inspection

Status and plan use the authenticated [local API v1](api/local-api.md) when a
daemon listens, and run in-process when none is available. Text/JSON renderers
are shared. Permission, authentication and protocol failures do not fall back.
Verbose inspection runs locally to preserve backend diagnostics; sync remains
in-process. KIT-29 supplies the serving boundary without a daemon command.
