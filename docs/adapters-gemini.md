# Gemini CLI adapter

`internal/adapters/gemini` implements the adapter contract and is registered in
public `kitbag sync`, plan, status and lifecycle commands. Locations and formats
come from the [Gemini inventory](agents.md#gemini-cli), documentation snapshot
**2026-10-24**. The inventory records no installed Gemini release version; these
are documentation contracts, not a minimum-version or observed-loading promise.

A target with `agent = "gemini"` and no instances uses `~/.gemini`.
`GEMINI_CLI_HOME` names its **parent**: `/custom` selects `/custom/.gemini`.
Relative overrides fail before planning. Resolution goes through `platform.Env`;
explicit instances instead name the configuration directory itself. A missing
listed instance warns and is skipped; the default directory may be created.

| Primitive | Default destination | Behavior |
| --- | --- | --- |
| Instructions | `<dir>/GEMINI.md` | Shared renderer, default `concat`, optional native `import`; managed marker, per-agent override and host addition. Unknown instruction symlink loading means no link render. |
| Skills | `~/.agents/skills/<name>` | Shared alias documented as winning within a tier. One materialized entry per child containing regular `SKILL.md`; no duplicate writes into `.gemini/skills`. |
| Commands | `<dir>/commands/<name>.toml` | Native TOML regular files copied unchanged. Same-name `per_agent.gemini` entries win. Other entries warn and are never written. |
| Settings | — | Declared fragments produce unsupported targets, never a settings merge. |

Instance `roots.instructions`, `roots.skills` and `roots.commands` replace their
respective directory roots. Primitive enable switches and manifest mode resolve
by the existing contract. Shared skills carry no instance guard, so they remain
available when a listed Gemini instance is missing, as for Codex. Their owners
are unioned with identical Codex declarations. Within one Gemini target, copy
wins over link across instances sharing the root. Different source or policy
at the same shared destination remains a planner error; align policies or use
explicit distinct roots. Instructions use one-time backups; skill and command
obstacles require adoption and are never silently replaced.

## Conservative boundaries

The inventory specifies TOML prompt definitions but does not establish a safe
Markdown/frontmatter conversion or placeholder equivalence. Following the
[manifest's native-format contract](manifest.md#collections-commands-agents-hooks-rules),
no conversion is attempted and no generated TOML is written. Author native files
under `commands.per_agent.gemini`. Only immediate regular files are accepted;
directory entries (including nested command trees) warn instead of being copied
without validating their contents. TOML bytes are not schema-validated by KitBag.

Settings and hooks need separate native merge semantics. Agents and rules are
not implemented in this ticket; explicit declarations warn. MCP is separate work.
System settings, credentials, history and generated memory are never managed.
Changing `context.fileName` in existing settings can hide `GEMINI.md`; KitBag does
not alter that setting. Instruction link loading and native Windows mappings
remain UNKNOWN. Windows is unsupported before 1.x.

## Verification

Fake-host unit and declaration goldens cover paths, relocation, modes, owners,
overrides, formats, warnings and instruction bytes. Update reviewed goldens with
`mise exec -- go test ./internal/adapters/gemini -run '^TestGolden$' -args -update`.
`gemini.txtar` exercises real `kitbag sync` in throwaway HOME, including silent
repeat, override, relocated parent, relative override rejection and missing
instance. `cli_shared_skills.txtar` enables Codex, Gemini and OpenCode together;
CLI unit tests also assert shared owners and reject conflicting sources.

Runner02: `command -v gemini` found no installed binary on 2026-10-25. Real Gemini
loading is **NOT verified**; no agent was run and no real configuration was read
or written. Native macOS and Windows agent loading are also unverified.
