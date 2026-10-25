# OpenCode adapter

`internal/adapters/opencode` implements the adapter contract and is registered in
public sync, plan, status and lifecycle commands. The source is the
[OpenCode inventory](agents.md#opencode), documentation snapshot **2026-10-24**.
No installed OpenCode release version was recorded there; no release-specific
loading behavior is claimed.

The default instance uses `$XDG_CONFIG_HOME/opencode`, otherwise
`~/.config/opencode`. The platform Env resolves XDG: empty or relative overrides
fall back to the default. Explicit listed instances name their directory itself;
missing ones warn and are skipped. An absent default directory may be created.
`OPENCODE_CONFIG`, `OPENCODE_CONFIG_DIR` and `OPENCODE_CONFIG_CONTENT` are distinct
inputs (custom file, additional directory, inline content), not replacement homes;
the adapter never interprets them as relocation variables.

| Primitive | Default destination | Behavior |
| --- | --- | --- |
| Instructions | `<dir>/AGENTS.md` | Shared `concat` renderer, managed marker, per-agent override and host additions; native import and link render are not offered. |
| Skills | `<dir>/skills/<name>` | Copies each child with regular `SKILL.md`; never also writes compatibility roots. |
| Commands | `<dir>/commands/<name>.md` | Native Markdown/frontmatter files copied unchanged; `per_agent.opencode` wins for the same name. |
| Agents | `<dir>/agents/<name>.md` | Native Markdown/frontmatter files copied unchanged, with the same override rule. |
| Settings | — | Explicit fragments warn through unsupported declarations; no merge or write. |

The plural resource directories are the documented preferred names. Instance
`roots` overrides replace the corresponding primitive directory. Instructions
use one-time backups; skills and collections refuse unmanaged obstacles.
Primitive enable switches follow target/instance policy. Skills, commands and
agents always declare copy mode because their symlink loading is UNKNOWN,
including when the manifest requests link mode. Other native frontmatter is
preserved byte for byte, not translated or schema-validated. Nonregular files,
other suffixes and directory entries warn and are not written.

The native skills root avoids incompatible copy/link declarations in the shared
`~/.agents/skills` alias used by Codex and Gemini. Enabling all three therefore
plans cleanly with one shared Codex/Gemini operation and one OpenCode copy.
Explicitly pointing roots at the same path with different sources or modes
remains an error rather than silently selecting one policy.

Hooks are JS/TS plugins, not Claude-style JSON files; this ticket never writes an
invented hooks file. Explicit hooks, rules and MCP declarations warn; MCP belongs
to a separate ticket. Settings merge, JSONC preservation and additional config
layers need follow-up work. Credentials, caches and generated memory remain
unmanaged. Instruction/skill link behavior and native Windows home/XDG expansion
are UNKNOWN; Windows is unsupported before 1.x.

## Verification

Fake-host unit tests and declaration goldens cover default/XDG/listed paths,
formats, modes, owners, settings protection, overrides and warnings. Update with
`mise exec -- go test ./internal/adapters/opencode -run '^TestGolden$' -args -update`.
`opencode.txtar` runs real public sync in throwaway HOME: first run, silent
repeat, override, native command/agent copies, XDG relocation and missing
instance. `cli_shared_skills.txtar` and CLI unit tests cover coexistence and
conflicting-source rejection.

Runner02: `command -v opencode` found no installed binary on 2026-10-25. Real
OpenCode loading is **NOT verified**; no agent was run and no real configuration
was read or written. Native macOS and Windows agent loading remain unverified.
