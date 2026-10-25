# Codex adapter

`internal/adapters/codex` declares Codex CLI targets for the planner
([plan.md](plan.md)). It covers what `agent-config-sync` does for Codex today
plus prompts. Every location comes from the [inventory](agents.md#codex-cli);
nothing here claims observed Codex loading behaviour.

```go
adapter, err := codex.New(env)                 // platform.Env: HOME and CODEX_HOME
result, err := adapter.Plan(fsys, repo, resolved) // Targets plus non-blocking Warnings
// adapter also implements target.Adapter (Declare drops the warnings).
```

## Instances

- A `codex` target without `instances` has one instance, `default`, at
  `CODEX_HOME` when set and non-empty, otherwise `~/.codex`. A relative
  `CODEX_HOME` is an error ([platform.md](platform.md#agent-homes)). The default
  home is created when missing, as the script did.
- Listed instances use their `dir`; `CODEX_HOME` never moves them (the manifest
  names the directory itself). A missing listed directory is skipped with a
  warning (`Target.InstanceDir`).
- `CODEX_HOME` is resolved once by `New`; `Declare` reads no environment.
- Owners are `codex:<target>/<instance>` for each primitive.

## Primitives

| Primitive | Destination (default root) | Declaration |
| --- | --- | --- |
| instructions | `<dir>/AGENTS.md` | `concat` through [instructions](instructions.md), with the managed marker; `import` is a plan-time error. Byte-identical to the script apart from the marker (`TestLegacyCompatibility`). |
| skills | `~/.agents/skills/<name>` | One materialized entry per skill directory (a child with `SKILL.md`), shared by every instance; the planner unions owners. |
| commands | `<dir>/prompts/<name>` | Markdown files copied byte for byte; other entries are reported unsupported. `per_agent.codex` entries replace same-name `dir` entries. |
| settings | — | Not synchronized (as today). A declared `targets.<t>.settings` yields an unsupported `<dir>/config.toml` entry: reported, never written. |

`roots.<primitive>` of an instance replaces the default directory: the
directory holding `AGENTS.md`, the skills root, or the prompts directory.
Declared `agents`, `hooks`, `rules` and `mcp` are not placed yet and produce a
non-blocking warning; MCP in `config.toml` belongs to the MCP ticket.

Mode: skills follow `target.ManifestMode`. In the shared root every instance of
a target must declare the same entry, so `copy` wins if any instance with skills enabled resolves
`copy`, otherwise `link` if any resolves `link`. Prompts are always copied.

An existing real directory or foreign link in `~/.agents/skills` is an
unmanaged obstacle: reported (exit 3) and left untouched. Skills carry no
backup policy; an unmanaged `AGENTS.md` is backed up once.

## Unknowns and their handling

- **`<dir>/skills`**: the inventory documents only `~/.agents/skills` (plus
  repository and admin roots) for Codex user skills. Whether Codex also reads
  `$CODEX_HOME/skills` is not established, so the adapter never writes there.
- **Prompt format**: Codex documents `$CODEX_HOME/prompts/*.md` as deprecated
  Markdown prompts. Accepted frontmatter and link following are not documented,
  so prompts are placed unchanged as copies and only `.md` regular files are
  supported; no format conversion.
- **Instruction links**: Codex instruction symlink loading is UNKNOWN, so the
  `link` render is not offered.
- **`AGENTS.override.md`**: Codex prefers it over `AGENTS.md`. The adapter
  does not read or manage it; an existing override hides the managed file.
- **Windows**: every Codex Windows cell is UNKNOWN; Windows is unsupported
  before 1.x anyway.

## Tests

Unit and golden tests use the in-memory fake (`testdata/golden`; update with
`mise exec -- go test ./internal/adapters/codex -run '^TestGolden$' -args -update`).
The test-only driver runs the real adapter with `kitbag __e2e_apply
-adapter=codex <repo>`: `apply_codex.txtar` (default `~/.codex`),
`apply_codex_home.txtar` (`CODEX_HOME`) and `apply_codex_foreign_skill.txtar`.
