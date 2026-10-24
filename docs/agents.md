# Agent configuration inventory

Verified documentation snapshot: **2026-10-24**. Research for
[KIT-1](https://linear.app/privacybrand/issue/KIT-1/spike-inventory-of-agent-configuration-locations-and-formats).
Scope: eight v1 client surfaces and eleven generic targets; no adapter implementation.

## How to read the evidence

- **DOC** means the cited official documentation states the contract in that row.
  Paths retain the source's portable notation (`~`, environment placeholders).
  DOC is documentary evidence only: native path expansion, filesystem behavior,
  and support in a particular installed release are **UNKNOWN** unless explicitly
  qualified in the row or OS note. The three OS columns expose that distinction;
  they do not report tests on three machines.
- **UNKNOWN** means the behavior or OS mapping has not been established. It does
  not mean unsupported, absent, or safe to emulate. A partly documented row
  explicitly identifies the remaining unknown portion.
- Each DOC cell inherits its row's linked source(s) and this verification date.
  A source's platform-neutral contract is shown for macOS/Linux in its original
  notation. Windows cells stay UNKNOWN unless the cited material explicitly
  establishes a relevant native path or OS-independent configuration contract.
  No `%APPDATA%`, XDG, home-directory, or WSL mapping is guessed.
- `~` denotes the home directory in documentation, not this researcher's private
  home path. Relative dotted paths are repository/workspace paths unless stated.
  Directory paths are not proof of file schema, discovery order, or link support.
- These are live, unpinned documentation contracts, not a minimum-version support
  promise. Source repositories on `main` are documentation evidence, not named
  release observations. The only local version observation is recorded under Codex.
- No agents were invoked interactively, no user configuration was modified, and
  no configuration loading or symlink experiment was run. Existing sync Bash is
  not verification evidence and was not used to fill cells. Native macOS and
  Windows behavior remains unobserved; Linux loading behavior is also unobserved.

## Coverage

- [Claude Code](#claude-code)
- [Codex CLI](#codex-cli)
- [Cursor](#cursor)
- [Gemini CLI](#gemini-cli)
- [OpenCode](#opencode)
- [GitHub Copilot CLI](#copilot-cli)
- [GitHub Copilot in VS Code](#copilot-vscode)
- [Windsurf](#windsurf)
- [Amp (generic)](#amp)
- [Cline (generic)](#cline)
- [Roo Code (generic)](#roo)
- [Kilo Code (generic)](#kilo)
- [Continue (generic)](#continue)
- [OpenHands (generic)](#openhands)
- [Goose (generic)](#goose)
- [Pi (generic)](#pi)
- [Antigravity (generic)](#antigravity)
- [Factory Droid (generic)](#factory)
- [Aider (generic)](#aider)

Every client has the same eleven categories below, including **separate** link
cells for instructions and skills. The generic entries are research baselines,
not claims that a native adapter or every primitive exists.

## Manifest implications for KIT-4

These are evidence-backed review inputs, not changes to the schema specification.

1. **Model resource roots independently.** Codex user skills use `~/.agents/skills`
   while instructions/settings use Codex home; Claude user MCP lives in
   `~/.claude.json`, outside the settings tree. A single directory prefix is
   insufficient ([Codex](#codex-cli), [Claude](#claude-code)).
2. **Keep agent, client surface, instance, and version evidence distinct.** Copilot
   CLI and VS Code harnesses use different formats and prompt handling; Windsurf
   documentation now redirects to a product with legacy fallbacks. Do not silently
   reinterpret an existing instance ([Copilot](#copilot-vscode), [Windsurf](#windsurf)).
3. **Separate file placement from structured merging and activation.** MCP uses
   TOML tables, JSON `mcpServers`, JSON `servers`, JSON `mcp`, and YAML extensions.
   Hook definitions can be inline or separate. Instruction and rule precedence is
   not a universal last-file-wins rule ([OpenCode](#opencode), [Goose](#goose),
   [Copilot CLI](#copilot-cli)).
4. **Preserve native formats and selected capabilities.** YAML/Markdown frontmatter,
   `.mdc`, TOML command files, and executable extension files cannot safely be
   treated as interchangeable copies of one prompt. Unknown primitives need an
   explicit unsupported/unknown result at planning time, not invented defaults
   ([Cursor](#cursor), [Gemini](#gemini-cli), [generic entries](#amp)).
5. **Relocation needs per-variable semantics.** `CODEX_HOME` and `COPILOT_HOME`
   name complete homes; `GEMINI_CLI_HOME` names the parent of `.gemini`;
   OpenCode custom file, extra directory, and inline content are different inputs
   ([Codex](#codex-cli), [Copilot CLI](#copilot-cli), [Gemini](#gemini-cli),
   [OpenCode](#opencode)).
6. **Track materialization capabilities per primitive and OS.** Reading an
   instruction symlink does not verify skill scanning, writing through links,
   plugin packaging, Windows privileges, or managed-policy acceptance. Keep
   unknown link capability explicit and permit copy mode per instance/primitive
   without revising the accepted [platform decision](adr/0003-materialization-and-platform.md)
   ([Claude](#claude-code), [Codex](#codex-cli)).
7. **Keep authored configuration apart from runtime state.** Agent roots may hold
   account data, MCP credentials, history, generated memories, and installed
   caches. Plan only named managed files/keys, not whole-root synchronization
   ([Copilot CLI](#copilot-cli), [Cline](#cline), [Windsurf](#windsurf)).

## Detailed matrix

<a id="claude-code"></a>

### Claude Code

Windows: the settings documentation explicitly maps `~/.claude` to `%USERPROFILE%\.claude` [C2][C2]. Managed instruction roots differ: macOS `/Library/Application Support/ClaudeCode`, Linux `/etc/claude-code`, Windows `C:\Program Files\ClaudeCode` [C1][C1]. Direct project `AGENTS.md` discovery is version-gated (docs name v2.1.277+ and later fixes); it is not a universal replacement contract [C1][C1].

| Category | Documented contract and limits | macOS | Linux | Windows |
| --- | --- | --- | --- | --- |
| Instructions and precedence | `~/.claude/CLAUDE.md`; managed → user → ancestor/project → local content is concatenated. Nested files load on demand; conflicting prose is not an enforced priority rule. [C1][C1] | DOC | DOC | DOC |
| Skills | `~/.claude/skills/<name>/SKILL.md`; project `.claude/skills/`. Markdown with YAML frontmatter. Same-name priority: enterprise > personal > project. [C3][C3] | DOC | DOC | DOC |
| Commands / prompts | Legacy `~/.claude/commands/*.md` and project `.claude/commands/*.md`; skills are preferred for new commands. [C3][C3] | DOC | DOC | DOC |
| Subagents | `~/.claude/agents/*.md`, project `.claude/agents/`; YAML frontmatter + Markdown. Managed > CLI > project > user > plugin for name collisions. [C4][C4] | DOC | DOC | DOC |
| Hooks | `hooks` in user/project/local JSON settings; plugin `hooks/hooks.json`; skill/agent frontmatter can also define hooks. [C5][C5] | DOC | DOC | DOC |
| Rules / memories | User `~/.claude/rules/*.md`, project `.claude/rules/`; optional path frontmatter. Auto-memory is generated state, not a portable instruction source. [C1][C1] | DOC | DOC | DOC |
| MCP path and format | User and per-project local entries in `~/.claude.json`; shared project `.mcp.json`; JSON `mcpServers`. Local > project > user for same-name servers. [C6][C6] | DOC | DOC | UNKNOWN |
| Settings path and format | `~/.claude/settings.json`; project `.claude/settings.json`, local `settings.local.json`; JSON. Managed policy can constrain these. [C2][C2] | DOC | DOC | DOC |
| Relocation variables / flags | `CLAUDE_CONFIG_DIR` relocates the home configuration directory. **UNKNOWN** exact relocated `.claude.json` filename: do not derive it from this variable. [C2][C2] | DOC | DOC | DOC |
| Instruction symlinks | Documented reading through `CLAUDE.md` → `AGENTS.md` symlink. Windows link creation/checkout has prerequisites; edit tools refuse writing through the link. [C1][C1] | DOC | DOC | UNKNOWN |
| Skill symlinks | Personal/project/enterprise skill folder links are followed and deduplicated by target; plugin rules differ. Native Windows behavior **UNKNOWN**. [C3][C3] | DOC | DOC | UNKNOWN |

<a id="codex-cli"></a>

### Codex CLI

All paths retain official portable notation; native Windows expansion is **UNKNOWN** here. Local observation O-local: `codex --version` printed `codex-cli 0.163.0` on Linux on 2026-10-24; bundled README inspected before remote research. No configuration-loading behavior was observed. Current docs are not pinned to that installed version.

| Category | Documented contract and limits | macOS | Linux | Windows |
| --- | --- | --- | --- | --- |
| Instructions and precedence | `$CODEX_HOME/AGENTS.override.md`, else `AGENTS.md` (default home `~/.codex`); project root → cwd, one file per directory: override > AGENTS > configured fallback names. Later directories override earlier prose. [O1][O1] | DOC | DOC | UNKNOWN |
| Skills | User `~/.agents/skills/`, repository `.agents/skills/` up to repo root, admin `/etc/codex/skills/`; `SKILL.md` + YAML frontmatter. Duplicate names need not merge. [O2][O2] | DOC | DOC | UNKNOWN |
| Commands / prompts | Deprecated `$CODEX_HOME/prompts/*.md`; use skills for new reusable prompts. [O4][O4] | DOC | DOC | UNKNOWN |
| Subagents | User `~/.codex/agents/*.toml`, project `.codex/agents/*.toml`; TOML custom agent definitions. [O5][O5] | DOC | DOC | UNKNOWN |
| Hooks | `hooks.json` beside active configuration layers or inline `[hooks]` in `config.toml`. Matching hooks accumulate; higher layers do not replace lower hooks. Trust review applies. [O6][O6] | DOC | DOC | UNKNOWN |
| Rules / memories | Execution policy: `rules/*.rules` beside active config layers, e.g. `~/.codex/rules/default.rules`, using Starlark. This differs from prose instructions; portable auto-memory contract **UNKNOWN**. [O7][O7] | DOC | DOC | UNKNOWN |
| MCP path and format | `config.toml`, normally `~/.codex/config.toml`; `[mcp_servers.<name>]` TOML tables. Trusted project `.codex/config.toml` can also contribute. [O8][O8] | DOC | DOC | UNKNOWN |
| Settings path and format | `~/.codex/config.toml`; trusted project `.codex/config.toml`; TOML. CLI > project (closest wins) > selected profile file > user > system/defaults, subject to managed policy. [O3][O3] | DOC | DOC | UNKNOWN |
| Relocation variables / flags | `CODEX_HOME` replaces Codex home. It does not establish relocation of the separately documented `~/.agents/skills` root. [O1][O1] [O2][O2] | DOC | DOC | UNKNOWN |
| Instruction symlinks | **UNKNOWN** — Instruction-file symlink loading was not tested or established by these docs.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Skill symlinks | Skill directory symlinks are explicitly followed during discovery. Native Windows link creation and scanning **UNKNOWN**. [O2][O2] | DOC | DOC | UNKNOWN |

<a id="cursor"></a>

### Cursor

| Category | Documented contract and limits | macOS | Linux | Windows |
| --- | --- | --- | --- | --- |
| Instructions and precedence | Global User Rules are edited in Customize → Rules; backing global instructions file **UNKNOWN**. Team > project > user guidance on conflict; project `AGENTS.md` supports nested scopes. [U1][U1] | DOC | DOC | UNKNOWN |
| Skills | User `~/.cursor/skills/`, `~/.agents/skills/`; project `.cursor/skills/`, `.agents/skills/`; `SKILL.md` with YAML frontmatter. [U2][U2] | DOC | DOC | UNKNOWN |
| Commands / prompts | Skills provide reusable workflows; historical `.cursor/commands/` migration is documented. Current separate global commands directory contract **UNKNOWN**. [U2][U2] | DOC | DOC | UNKNOWN |
| Subagents | `~/.cursor/agents/`, project `.cursor/agents/`; Markdown + YAML frontmatter; project definitions win name conflicts. [U3][U3] | DOC | DOC | UNKNOWN |
| Hooks | User `~/.cursor/hooks.json`, project `.cursor/hooks.json`; JSON hook definitions; script working directory differs between user and project scopes. [U4][U4] | DOC | DOC | UNKNOWN |
| Rules / memories | Project `.cursor/rules/*.mdc`, frontmatter for activation; plain `.md` there is ignored. Global User Rules use the UI. Portable generated memories **UNKNOWN**. [U1][U1] | DOC | DOC | UNKNOWN |
| MCP path and format | User `~/.cursor/mcp.json`, project `.cursor/mcp.json`; JSON `mcpServers`. [U5][U5] | DOC | DOC | UNKNOWN |
| Settings path and format | Cursor CLI `~/.cursor/cli-config.json` is named in the skill reference. Desktop settings path/format **UNKNOWN** in reviewed evidence. [U2][U2] | DOC | DOC | UNKNOWN |
| Relocation variables / flags | **UNKNOWN** — No configuration-root relocation variable was established.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Instruction symlinks | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Skill symlinks | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |

<a id="gemini-cli"></a>

### Gemini CLI

System settings roots are explicitly OS-specific [G6]: macOS `/Library/Application Support/GeminiCli`, Linux `/etc/gemini-cli`, Windows `C:\ProgramData\gemini-cli`; files `settings.json` and `system-defaults.json`. Home relocation examples explicitly cover macOS/Linux and Windows [G7][G7].

| Category | Documented contract and limits | macOS | Linux | Windows |
| --- | --- | --- | --- | --- |
| Instructions and precedence | `~/.gemini/GEMINI.md` → environment/workspace hierarchy → JIT directory context; concatenated. `context.fileName` can change/add filenames. [G1][G1] | DOC | DOC | UNKNOWN |
| Skills | `~/.gemini/skills/`, `~/.agents/skills/`; workspace equivalents. Workspace > user > extension > built-in; `.agents` alias wins within a tier. `SKILL.md`. [G2][G2] | DOC | DOC | UNKNOWN |
| Commands / prompts | `~/.gemini/commands/**/*.toml`, project `.gemini/commands/`; TOML prompt definitions; project wins same-name command. [G3][G3] | DOC | DOC | UNKNOWN |
| Subagents | `~/.gemini/agents/*.md`, project `.gemini/agents/*.md`; Markdown + required YAML frontmatter. [G4][G4] | DOC | DOC | UNKNOWN |
| Hooks | `hooks` object in `.gemini/settings.json` (user/project/system scopes); JSON event/matcher definitions. [G5][G5] [G6][G6] | DOC | DOC | DOC |
| Rules / memories | `GEMINI.md` is instructional memory, with `@file` imports. Separate generated auto-memory should not be equated with this source. [G1][G1] | DOC | DOC | UNKNOWN |
| MCP path and format | `mcpServers` in user/project/system `settings.json`; JSON server map. [G8][G8] [G6][G6] | DOC | DOC | DOC |
| Settings path and format | User `~/.gemini/settings.json`; project `.gemini/settings.json`. Defaults < system defaults < user < project < system overrides < environment < CLI. [G6][G6] | DOC | DOC | DOC |
| Relocation variables / flags | `GEMINI_CLI_HOME` names the parent in which `.gemini` is created, not the `.gemini` directory itself. `GEMINI_CLI_SYSTEM_SETTINGS_PATH` and `GEMINI_CLI_SYSTEM_DEFAULTS_PATH` override system files. [G7][G7] [G6][G6] | DOC | DOC | DOC |
| Instruction symlinks | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Skill symlinks | Official command reference documents `gemini skills link <path>` as linking local skills via symlink. Native Windows link behavior **UNKNOWN**. [G9][G9] | DOC | DOC | UNKNOWN |

<a id="opencode"></a>

### OpenCode

Current docs prefer plural resource directories; singular legacy directory names remain compatible [P1][P1]. Native Windows home/XDG mapping is **UNKNOWN** in this inventory; do not substitute `%APPDATA%` without evidence.

| Category | Documented contract and limits | macOS | Linux | Windows |
| --- | --- | --- | --- | --- |
| Instructions and precedence | `~/.config/opencode/AGENTS.md`; fallback `~/.claude/CLAUDE.md`. Local `AGENTS.md` preferred to `CLAUDE.md`; first match per category; global OpenCode guidance preferred to Claude fallback. [P2][P2] | DOC | DOC | UNKNOWN |
| Skills | `~/.config/opencode/skills/`; `.opencode/skills/`; also user/project `.claude/skills/` and `.agents/skills/` compatibility roots; `SKILL.md` + YAML. [P3][P3] | DOC | DOC | UNKNOWN |
| Commands / prompts | `~/.config/opencode/commands/*.md`, project `.opencode/commands/`; frontmatter + Markdown, or `command` object in config. [P4][P4] | DOC | DOC | UNKNOWN |
| Subagents | `~/.config/opencode/agents/*.md`, project `.opencode/agents/`; YAML + Markdown, or `agent` config entries with `mode: subagent`. [P5][P5] | DOC | DOC | UNKNOWN |
| Hooks | Event hooks are JS/TS plugins in `~/.config/opencode/plugins/`, `.opencode/plugins/`, or configured packages. A Claude-style hooks file is **UNKNOWN**. [P6][P6] | DOC | DOC | UNKNOWN |
| Rules / memories | `AGENTS.md` plus config `instructions` paths/globs for additional guidance. Separate portable generated-memory layout **UNKNOWN**. [P2][P2] | DOC | DOC | UNKNOWN |
| MCP path and format | `mcp` object in `opencode.json` / `opencode.jsonc`; local entries use command arrays, remote entries URLs. [P7][P7] [P1][P1] | DOC | DOC | UNKNOWN |
| Settings path and format | `~/.config/opencode/opencode.json` (JSON/JSONC accepted); project config; separate `tui.json`. Later config layers merge conflicting keys; managed layers constrain them. [P1][P1] | DOC | DOC | UNKNOWN |
| Relocation variables / flags | `OPENCODE_CONFIG` custom file; `OPENCODE_CONFIG_DIR` additional configuration directory; `OPENCODE_CONFIG_CONTENT` inline config. They are distinct inputs, not one interchangeable home. [P1][P1] | DOC | DOC | UNKNOWN |
| Instruction symlinks | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Skill symlinks | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |

<a id="copilot-cli"></a>

### GitHub Copilot CLI

The setup guide explicitly maps the home root to `~/.copilot` on macOS/Linux and `$HOME\.copilot` on Windows [H6][H6]. The current directory reference documents settings-file symlink writes, but that does **not** verify instruction or skill symlink discovery [H1][H1].

| Category | Documented contract and limits | macOS | Linux | Windows |
| --- | --- | --- | --- | --- |
| Instructions and precedence | `~/.copilot/copilot-instructions.md`, `~/.copilot/instructions/**/*.instructions.md`; repository instructions and AGENTS files combine. Docs explicitly do not define general precedence among these files. [H2][H2] | DOC | DOC | DOC |
| Skills | User `~/.copilot/skills/` or `~/.agents/skills/`; project `.github/skills/`, `.claude/skills/`, `.agents/skills/`; `SKILL.md` + YAML frontmatter. [H3][H3] | DOC | DOC | DOC |
| Commands / prompts | **UNKNOWN** — Use skills for documented reusable prompts; a separate user prompt-file directory was not established.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Subagents | Personal definitions in `~/.copilot/agents/`; full file suffix/schema and collision order **UNKNOWN** in reviewed directory reference. [H1][H1] | DOC | DOC | DOC |
| Hooks | User `~/.copilot/hooks/*.json`, project `.github/hooks/*.json`; JSON. Inline settings hooks also documented. [H4][H4] [H1][H1] | DOC | DOC | DOC |
| Rules / memories | Additional personal instructions under `~/.copilot/instructions/`; portable generated-memory source/format **UNKNOWN**. [H2][H2] | DOC | DOC | DOC |
| MCP path and format | `~/.copilot/mcp-config.json`, JSON `mcpServers`; project `.mcp.json` / `.github/mcp.json` override same-name user servers. CLI does not read VS Code `servers` format. [H5][H5] | DOC | DOC | DOC |
| Settings path and format | `~/.copilot/settings.json` is editable JSONC; `.github/copilot/settings.json` and `settings.local.json` add repository/local scopes. `config.json` is managed state, not current editable preferences. [H1][H1] | DOC | DOC | DOC |
| Relocation variables / flags | `COPILOT_HOME` replaces the full `~/.copilot` directory; platform cache remains separate. [H1][H1] | DOC | DOC | DOC |
| Instruction symlinks | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Skill symlinks | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |

<a id="copilot-vscode"></a>

### GitHub Copilot in VS Code

Default VS Code settings roots [V7]: macOS `~/Library/Application Support/Code/User/settings.json`; Linux `~/.config/Code/User/settings.json`; Windows `%APPDATA%\Code\User\settings.json`. Profiles add `profiles/<profile ID>/settings.json`. This is VS Code stable, not Cursor, Insiders, or a remote server path.

| Category | Documented contract and limits | macOS | Linux | Windows |
| --- | --- | --- | --- | --- |
| Instructions and precedence | Copilot harness: `~/.copilot/copilot-instructions.md` and `~/.copilot/instructions/`; repository `.github/copilot-instructions.md`, `AGENTS.md`, scoped `.instructions.md`. General conflict precedence **UNKNOWN**. [V1][V1] | DOC | DOC | UNKNOWN |
| Skills | User `~/.copilot/skills/`, `~/.claude/skills/`, `~/.agents/skills/`; project `.github/skills/`, `.claude/skills/`, `.agents/skills/`; `SKILL.md`. Harness selection matters. [V2][V2] | DOC | DOC | UNKNOWN |
| Commands / prompts | Local harness: `.github/prompts/*.prompt.md` or user-profile prompt storage. Copilot harness does not run `.prompt.md` directly; use a skill. [V3][V3] | DOC | DOC | UNKNOWN |
| Subagents | Copilot harness user `~/.copilot/agents/`, project `.github/agents/*.agent.md`; YAML + Markdown. Local harness uses profile storage for user agents. [V4][V4] | DOC | DOC | UNKNOWN |
| Hooks | Copilot harness reads `.github/hooks/*.json` and inline Copilot settings hooks; matching sources accumulate. Local harness discovery/settings differ. [V5][V5] | DOC | DOC | UNKNOWN |
| Rules / memories | Scoped `.github/instructions/**/*.instructions.md` with YAML `applyTo`; Local harness profile instructions can sync, harness home folders do not roam through Settings Sync. [V1][V1] | DOC | DOC | UNKNOWN |
| MCP path and format | Copilot Global: `$COPILOT_HOME/mcp-config.json`, JSON `mcpServers`. Local compatibility: `.vscode/mcp.json` or user-profile `mcp.json`, JSON `servers`. Formats are distinct. [V6][V6] | DOC | DOC | UNKNOWN |
| Settings path and format | VS Code user `settings.json` is JSONC; workspace `.vscode/settings.json`; non-default profiles have their own user settings. See OS roots below. [V7][V7] | DOC | DOC | DOC |
| Relocation variables / flags | `COPILOT_HOME` for Copilot MCP home. Other native configuration-root environment variables **UNKNOWN**; remote harness files belong to the runtime host. [V6][V6] [V5][V5] | DOC | DOC | UNKNOWN |
| Instruction symlinks | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Skill symlinks | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |

<a id="windsurf"></a>

### Windsurf

Documentation identity caveat: on 2026-10-24 official `docs.windsurf.com/windsurf/cascade/{memories,skills,workflows,hooks,mcp}.md` redirected to `docs.devin.ai/desktop/cascade/...`. DOC cells describe the returned current documentation only. Compatibility with a named installed Windsurf release is **UNKNOWN**; do not silently migrate an existing instance to Devin paths.

| Category | Documented contract and limits | macOS | Linux | Windows |
| --- | --- | --- | --- | --- |
| Instructions and precedence | Redirected docs retain global `~/.codeium/windsurf/memories/global_rules.md`; project `AGENTS.md` and workspace rules contribute. Conflict precedence **UNKNOWN**. [W1][W1] | DOC | DOC | UNKNOWN |
| Skills | User `~/.codeium/windsurf/skills/` or `~/.config/devin/skills/`; project `.devin/skills/` preferred, `.windsurf/skills/` legacy; `SKILL.md`. [W2][W2] | DOC | DOC | UNKNOWN |
| Commands / prompts | User `~/.codeium/windsurf/global_workflows/*.md`; project `.devin/workflows/*.md` or legacy `.windsurf/workflows/*.md`. System > workspace > global > built-in on name conflict. [W3][W3] | DOC | DOC | UNKNOWN |
| Subagents | **UNKNOWN** — Built-in specialized agents are not evidence of a user-configurable subagent directory.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Hooks | User `~/.codeium/windsurf/hooks.json`; project `.devin/hooks.json`, legacy `.windsurf/hooks.json` fallback; JSON hooks. [W4][W4] | DOC | DOC | UNKNOWN |
| Rules / memories | Global rule file above; workspace `.devin/rules/*.md` preferred to `.windsurf/rules/*.md`; generated `~/.codeium/windsurf/memories/` is workspace-associated local state. [W1][W1] | DOC | DOC | UNKNOWN |
| MCP path and format | Current redirected docs: macOS/Linux `~/.config/devin/mcp_config.json`, Windows `%APPDATA%\devin\mcp_config.json`; JSON `mcpServers`. Legacy installed Windsurf MCP path **UNKNOWN**. [W5][W5] | DOC | DOC | DOC |
| Settings path and format | **UNKNOWN** — Desktop preference settings path/format not established.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Relocation variables / flags | `XDG_CONFIG_HOME` relocates the documented Devin MCP root to `$XDG_CONFIG_HOME/devin`; relocation of legacy Windsurf roots **UNKNOWN**. [W5][W5] | DOC | DOC | DOC |
| Instruction symlinks | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Skill symlinks | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |

<a id="amp"></a>

### Amp (generic)

Managed guidance paths are OS-specific [A1]: Linux `/etc/ampcode/AGENTS.md`, macOS `/Library/Application Support/ampcode/AGENTS.md`, Windows `%ProgramData%\ampcode\AGENTS.md`. The instruction-file format is Markdown.

| Category | Documented contract and limits | macOS | Linux | Windows |
| --- | --- | --- | --- | --- |
| Instructions and precedence | `~/.config/amp/AGENTS.md` and `~/.config/AGENTS.md`; cwd/parents and lazy subtree guidance. System-wide files also contribute; workspace guidance precedes personal/repository guidance. [A1][A1] | DOC | DOC | DOC |
| Skills | User roots include `~/.config/agents/skills/`, `~/.agents/skills/`, `~/.config/amp/skills/`; project `.agents/skills/`; Claude compatibility roots also searched. First frontmatter name wins in documented search order. [A3][A3] | DOC | DOC | UNKNOWN |
| Commands / prompts | Skills are documented reusable workflows; a separate standalone prompt-directory contract **UNKNOWN**. [A3][A3] | DOC | DOC | UNKNOWN |
| Subagents | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Hooks | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Rules / memories | AGENTS-style files provide persistent prose; disable matching guidance with `AMP_IGNORE_GUIDANCE_FILES`. Portable generated-memory layout **UNKNOWN**. [A1][A1] | DOC | DOC | UNKNOWN |
| MCP path and format | Skills may carry sibling `mcp.json` or frontmatter `mcpServers` (frontmatter wins). Global MCP uses `amp.mcpServers` in the documented JSON/JSONC settings file. [A3][A3] [A2][A2] | DOC | DOC | UNKNOWN |
| Settings path and format | `~/.config/amp/settings.json` or `.jsonc`; workspace nearest `.amp/settings.json` / `.jsonc`; workspace overrides user settings. Windows `%USERPROFILE%\.config\amp\settings.json[c]`. [A2][A2] | DOC | DOC | DOC |
| Relocation variables / flags | `--settings-file <path>` selects user settings. Whole-root relocation env var **UNKNOWN**; `AMP_IGNORE_GUIDANCE_FILES` controls loading, not relocation. [A2][A2] [A1][A1] | DOC | DOC | UNKNOWN |
| Instruction symlinks | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Skill symlinks | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |

<a id="cline"></a>

### Cline (generic)

| Category | Documented contract and limits | macOS | Linux | Windows |
| --- | --- | --- | --- | --- |
| Instructions and precedence | Cross-tool user `~/.agents/AGENTS.md`; global rules roots plus project `AGENTS.md`, `.cline/rules/` or `.clinerules/`. Workspace rules override conflicting global rules. [L1][L1] [L2][L2] | DOC | DOC | UNKNOWN |
| Skills | User `~/.cline/skills/`; project `.cline/skills/`, legacy `.clinerules/skills/`, compatible `.agents/skills/`; `SKILL.md`. Global wins same-name workspace skill. Windows user root `C:\Users\USERNAME\.cline\skills\`. [L3][L3] [L1][L1] | DOC | DOC | DOC |
| Commands / prompts | `~/.cline/workflows/`, project `.cline/workflows/`; additional `~/Documents/Cline/Workflows`. Exact prompt file schema **UNKNOWN**. [L1][L1] | DOC | DOC | UNKNOWN |
| Subagents | `~/.cline/agents/`, project `.cline/agents/`; definition schema/precedence **UNKNOWN**. [L1][L1] | DOC | DOC | UNKNOWN |
| Hooks | `~/.cline/hooks/`, project `.cline/hooks/`; compatibility `~/Documents/Cline/Hooks`; schema **UNKNOWN**. [L1][L1] | DOC | DOC | UNKNOWN |
| Rules / memories | Global Markdown rules: `~/Documents/Cline/Rules` on macOS/Linux; `Documents\Cline\Rules` on Windows, including documented OneDrive variants; `~/.cline/rules` also searched. [L2][L2] | DOC | DOC | DOC |
| MCP path and format | `~/.cline/data/settings/cline_mcp_settings.json`; JSON filename documented; top-level server schema **UNKNOWN** in reviewed layout. [L1][L1] | DOC | DOC | UNKNOWN |
| Settings path and format | `~/.cline/data/settings/global-settings.json`; precise schema/merge rules **UNKNOWN**. Do not copy adjacent provider credentials. [L1][L1] | DOC | DOC | UNKNOWN |
| Relocation variables / flags | `CLINE_DIR` or `--config <path>` selects a custom config directory. [L1][L1] | DOC | DOC | UNKNOWN |
| Instruction symlinks | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Skill symlinks | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |

<a id="roo"></a>

### Roo Code (generic)

Generic inventory covers Roo Code custom-instruction documentation returned by the official documentation redirect. Other extension storage, settings, MCP, skills, and link behavior remain **UNKNOWN**.

| Category | Documented contract and limits | macOS | Linux | Windows |
| --- | --- | --- | --- | --- |
| Instructions and precedence | Global `~/.roo/rules/`, `rules-<modeSlug>/`; workspace `.roo/rules/` and mode rules. Workspace takes precedence on conflict; directories precede legacy `.roorules` files. [R1][R1] | DOC | DOC | DOC |
| Skills | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Commands / prompts | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Subagents | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Hooks | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Rules / memories | Rule files can be Markdown/text; global Windows roots `%USERPROFILE%\.roo\rules\` and `rules-<modeSlug>\`. [R1][R1] | DOC | DOC | DOC |
| MCP path and format | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Settings path and format | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Relocation variables / flags | Docs explicitly call global rules directory fixed/non-customizable. This is not evidence that every other Roo setting lacks relocation. [R1][R1] | DOC | DOC | DOC |
| Instruction symlinks | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Skill symlinks | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |

<a id="kilo"></a>

### Kilo Code (generic)

The official documentation corpus has a “Custom Rules” section with separate VS Code/CLI tabs. Older Kilo roots are not evidence for the current native Windows settings root.

| Category | Documented contract and limits | macOS | Linux | Windows |
| --- | --- | --- | --- | --- |
| Instructions and precedence | Global `kilo.jsonc` `instructions` paths/globs, typically `~/.config/kilo/kilo.jsonc`; project instructions follow and win conflicts. [K1][K1] | DOC | DOC | UNKNOWN |
| Skills | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Commands / prompts | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Subagents | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Hooks | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Rules / memories | Project `.kilo/rules/`; legacy `.kilocode/rules/` remains compatible. Explicit `instructions` entries are ordered; globs use filesystem order. [K1][K1] | DOC | DOC | UNKNOWN |
| MCP path and format | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Settings path and format | Global `~/.config/kilo/kilo.jsonc`, project `kilo.jsonc`; JSONC. Source distinguishes VS Code and CLI surfaces. [K1][K1] | DOC | DOC | UNKNOWN |
| Relocation variables / flags | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Instruction symlinks | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Skill symlinks | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |

<a id="continue"></a>

### Continue (generic)

| Category | Documented contract and limits | macOS | Linux | Windows |
| --- | --- | --- | --- | --- |
| Instructions and precedence | `rules` in YAML config and workspace `.continue/rules/*.md`. Rules join in toolbar order. Separate global AGENTS-style file **UNKNOWN**. [N2][N2] [N3][N3] | DOC | DOC | UNKNOWN |
| Skills | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Commands / prompts | YAML `prompts` is a documented config property; a separately discovered global prompts directory **UNKNOWN**. [N2][N2] | DOC | DOC | UNKNOWN |
| Subagents | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Hooks | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Rules / memories | Workspace `.continue/rules/*.md` supports frontmatter; rules apply in Agent, Chat, Edit, not autocomplete/apply. [N3][N3] | DOC | DOC | UNKNOWN |
| MCP path and format | `mcpServers` in `config.yaml`; YAML list, not a shared JSON `mcpServers` map. [N2][N2] [N1][N1] | DOC | DOC | DOC |
| Settings path and format | macOS/Linux `~/.continue/config.yaml`; Windows `%USERPROFILE%\.continue\config.yaml`; YAML. `config.json` is deprecated. [N1][N1] | DOC | DOC | DOC |
| Relocation variables / flags | **UNKNOWN** — Whole-root relocation env var not established; do not confuse model environment values with directory selection.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Instruction symlinks | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Skill symlinks | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |

<a id="openhands"></a>

### OpenHands (generic)

OpenHands Cloud, SDK, and local runtime are separate surfaces. This entry does not turn cloud user/organization skills into an invented host-global file path.

| Category | Documented contract and limits | macOS | Linux | Windows |
| --- | --- | --- | --- | --- |
| Instructions and precedence | Repository `AGENTS.md` supplies general guidelines. Global user instructions path/precedence **UNKNOWN**. [E1][E1] | DOC | DOC | UNKNOWN |
| Skills | General skills are always-loaded Markdown context with optional frontmatter. Exact user and repository skill-directory contracts **UNKNOWN** in the reviewed page. [E1][E1] | DOC | DOC | UNKNOWN |
| Commands / prompts | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Subagents | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Hooks | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Rules / memories | General repository skills provide persistent project context; global generated memories layout **UNKNOWN**. [E1][E1] | DOC | DOC | UNKNOWN |
| MCP path and format | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Settings path and format | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Relocation variables / flags | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Instruction symlinks | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Skill symlinks | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |

<a id="goose"></a>

### Goose (generic)

| Category | Documented contract and limits | macOS | Linux | Windows |
| --- | --- | --- | --- | --- |
| Instructions and precedence | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Skills | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Commands / prompts | Config-root `prompts/` for customized templates; YAML `slash_commands` entries map a command to a recipe path. Template discovery/precedence beyond that **UNKNOWN**. [B1][B1] | DOC | DOC | UNKNOWN |
| Subagents | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Hooks | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Rules / memories | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| MCP path and format | `config.yaml` under config root; YAML `extensions` map with `stdio` / `streamable_http` server definitions. [B1][B1] | DOC | DOC | DOC |
| Settings path and format | macOS/Linux `~/.config/goose/config.yaml`; Windows `%APPDATA%\Block\goose\config\config.yaml`. Environment values override file values, then defaults. [B1][B1] | DOC | DOC | DOC |
| Relocation variables / flags | **UNKNOWN** — Relocation env var not established by the reviewed configuration guide.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Instruction symlinks | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Skill symlinks | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |

<a id="pi"></a>

### Pi (generic)

| Category | Documented contract and limits | macOS | Linux | Windows |
| --- | --- | --- | --- | --- |
| Instructions and precedence | Agent root `~/.pi/agent`; `AGENTS.override.md` replaces same-directory AGENTS/CLAUDE alternatives. Agent-root and ancestor/cwd context files combine. Context discovery does not require project trust. [I1][I1] | DOC | DOC | UNKNOWN |
| Skills | `~/.pi/agent/skills/`, project `.pi/skills/`; `SKILL.md` resource directories. [I1][I1] [I2][I2] | DOC | DOC | UNKNOWN |
| Commands / prompts | `~/.pi/agent/prompts/`, project `.pi/prompts/`; Markdown prompt templates exposed as slash commands. [I1][I1] | DOC | DOC | UNKNOWN |
| Subagents | **UNKNOWN** — No user subagent file contract established; extensibility is not evidence for one.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Hooks | Extensions live in `~/.pi/agent/extensions/` / `.pi/extensions/`. Dedicated lifecycle-hook config path/format **UNKNOWN**. [I1][I1] | DOC | DOC | UNKNOWN |
| Rules / memories | `SYSTEM.md` replaces, `APPEND_SYSTEM.md` appends to system prompt in agent/project roots. Trusted project counterpart wins; generated memory layout **UNKNOWN**. [I1][I1] | DOC | DOC | UNKNOWN |
| MCP path and format | `~/.pi/agent/mcp.json`, project `.pi/mcp.json`; JSON `mcpServers`; trusted project entry replaces same-name user entry. [I3][I3] | DOC | DOC | UNKNOWN |
| Settings path and format | `~/.pi/agent/settings.json`, project `.pi/settings.json`; JSON. Project configuration requires trust (with documented session-directory exception). [I1][I1] | DOC | DOC | UNKNOWN |
| Relocation variables / flags | `PI_CODING_AGENT_DIR` selects agent directory; SDK `agentDir` also available. [I1][I1] | DOC | DOC | UNKNOWN |
| Instruction symlinks | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Skill symlinks | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |

<a id="antigravity"></a>

### Antigravity (generic)

This generic entry deliberately identifies CLI-only evidence where a shared configuration page exposes it. Do not apply those cells as an Antigravity IDE adapter contract.

| Category | Documented contract and limits | macOS | Linux | Windows |
| --- | --- | --- | --- | --- |
| Instructions and precedence | Global `~/.gemini/AGENTS.md`, `~/.gemini/GEMINI.md`, modular `~/.gemini/config/rules/*.md`; workspace `AGENTS.md` / `.agents/rules/`. Surface-specific discovery/order matters. [T1][T1] | DOC | DOC | UNKNOWN |
| Skills | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Commands / prompts | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Subagents | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Hooks | CLI docs name `~/.gemini/config/hooks.json` and workspace `.agents/hooks.json`; exact IDE-only contract **UNKNOWN**. [T2][T2] | DOC | DOC | UNKNOWN |
| Rules / memories | Markdown global/workspace rules, optional activation frontmatter; portable generated knowledge/memory contract **UNKNOWN**. [T1][T1] | DOC | DOC | UNKNOWN |
| MCP path and format | CLI docs name `~/.gemini/config/mcp_config.json`, workspace `.agents/mcp_config.json`; IDE-specific server schema/path **UNKNOWN**. [T2][T2] | DOC | DOC | UNKNOWN |
| Settings path and format | **UNKNOWN** — Reviewed settings page mixes App, IDE, and CLI; no universal settings-file path established.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Relocation variables / flags | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Instruction symlinks | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Skill symlinks | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |

<a id="factory"></a>

### Factory Droid (generic)

| Category | Documented contract and limits | macOS | Linux | Windows |
| --- | --- | --- | --- | --- |
| Instructions and precedence | `~/.factory/AGENTS.md` and compatible personal roots; project `AGENTS.md`. Project guidance refines personal defaults; nested files refine root guidance. Same-scope filename collision order **UNKNOWN**. [F2][F2] | DOC | DOC | UNKNOWN |
| Skills | User `~/.factory/skills/<name>/SKILL.md`, project `.factory/skills/`; compatible user/project `.agents/skills/`, `.agent/skills/`. YAML + Markdown. Project skills precede personal skills; mission/automation scopes differ. [F3][F3] | DOC | DOC | UNKNOWN |
| Commands / prompts | Project `.factory/commands/*` supports prompt/executable shortcuts; user commands path/schema **UNKNOWN**. [F2][F2] | DOC | DOC | UNKNOWN |
| Subagents | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Hooks | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Rules / memories | AGENTS-style files are coding guidelines, not parsed settings. Portable generated-memory layout **UNKNOWN**. [F2][F2] | DOC | DOC | UNKNOWN |
| MCP path and format | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Settings path and format | macOS/Linux `~/.factory/settings.json`; Windows `%USERPROFILE%\.factory\settings.json`; JSON. User/project `settings.local.json` merges over same-scope settings. [F1][F1] | DOC | DOC | DOC |
| Relocation variables / flags | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Instruction symlinks | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Skill symlinks | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |

<a id="aider"></a>

### Aider (generic)

| Category | Documented contract and limits | macOS | Linux | Windows |
| --- | --- | --- | --- | --- |
| Instructions and precedence | No automatic global instructions filename established. Use `read` config / `--read CONVENTIONS.md` for read-only conventions context; loaded config determines paths. [Y2][Y2] | DOC | DOC | UNKNOWN |
| Skills | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Commands / prompts | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Subagents | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Hooks | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Rules / memories | Conventions are user-selected read-only context, not evidence of automatically discovered skill/rules folders. Generated memory contract **UNKNOWN**. [Y2][Y2] | DOC | DOC | UNKNOWN |
| MCP path and format | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Settings path and format | `.aider.conf.yml` YAML searched home → Git root → cwd, later values win. `--config <filename>` loads only that file. Native Windows home expansion **UNKNOWN**. [Y1][Y1] | DOC | DOC | UNKNOWN |
| Relocation variables / flags | `--config <filename>` selects a file, not a configuration directory. Whole-root relocation env var **UNKNOWN**. [Y1][Y1] | DOC | DOC | UNKNOWN |
| Instruction symlinks | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |
| Skill symlinks | **UNKNOWN** — Not established by the reviewed sources.  | UNKNOWN | UNKNOWN | UNKNOWN |

## Verification limits and follow-up evidence

All 19 clients × 11 categories × 3 OS columns are present (627 status cells).
DOC cells certify source review only. **No runtime compatibility claim was tested.**
UNKNOWN remains especially significant for instruction symlinks (all except
Claude's documented example), skill symlinks (all except Claude, Codex, Gemini
Unix documentation), native Windows mappings, generated memories, and generic
agents' less-documented primitives. Even a DOC link cell has no observed named
version/OS result. Agent upgrades, remote runtimes, trust/policy controls, plugins,
and IDE profiles can change effective loading.

Before implementing link mode for an unresolved primitive, obtain an explicit
upstream guarantee or run a separately authorized isolated fixture against a
named version/OS. Do not probe real user configs or infer support from ordinary
filesystem read behavior. Preserve unknowns until that evidence exists.

Research used official documentation pages, official documentation indexes, and
upstream documentation source. Pages that returned errors, sign-in redirects,
or unrelated content were not used as verification. The unavailable Linear
shared-context endpoint supplied no evidence; the unchanged KIT-1 and KIT-4 issue
descriptions were retrieved through connected Linear. No private configuration
content or private host paths are included.

## Sources

Every source below was fetched and reviewed on **2026-10-24**. They are official
product documentation or documentation in the upstream product repository.
OpenAI URLs are canonical destinations returned by the former
`developers.openai.com/codex/...` documentation URLs. Windsurf redirect details
are preserved in its entry. Gemini and Kilo feature text was also reviewed in
their official documentation corpus/index, not inferred from search snippets.

- **C1**: [Claude instructions and memory](https://code.claude.com/docs/en/memory) — 2026-10-24; documentation, unpinned.
- **C2**: [Claude settings](https://code.claude.com/docs/en/settings) — 2026-10-24; documentation, unpinned.
- **C3**: [Claude skills and legacy commands](https://code.claude.com/docs/en/skills) — 2026-10-24; documentation, unpinned.
- **C4**: [Claude subagents](https://code.claude.com/docs/en/sub-agents) — 2026-10-24; documentation, unpinned.
- **C5**: [Claude hooks](https://code.claude.com/docs/en/hooks) — 2026-10-24; documentation, unpinned.
- **C6**: [Claude MCP](https://code.claude.com/docs/en/mcp) — 2026-10-24; documentation, unpinned.
- **O1**: [Codex instruction discovery](https://learn.chatgpt.com/docs/agent-configuration/agents-md) — 2026-10-24; documentation, unpinned.
- **O2**: [Codex skills](https://learn.chatgpt.com/docs/build-skills) — 2026-10-24; documentation, unpinned.
- **O3**: [Codex settings](https://learn.chatgpt.com/docs/config-file/config-basic) — 2026-10-24; documentation, unpinned.
- **O4**: [Codex custom prompts](https://learn.chatgpt.com/docs/custom-prompts) — 2026-10-24; documentation, unpinned.
- **O5**: [Codex subagents](https://learn.chatgpt.com/docs/agent-configuration/subagents) — 2026-10-24; documentation, unpinned.
- **O6**: [Codex hooks](https://learn.chatgpt.com/docs/hooks) — 2026-10-24; documentation, unpinned.
- **O7**: [Codex execution rules](https://learn.chatgpt.com/docs/agent-configuration/rules) — 2026-10-24; documentation, unpinned.
- **O8**: [Codex MCP](https://learn.chatgpt.com/docs/extend/mcp) — 2026-10-24; documentation, unpinned.
- **U1**: [Cursor rules](https://cursor.com/docs/rules) — 2026-10-24; documentation, unpinned.
- **U2**: [Cursor skills](https://cursor.com/docs/skills) — 2026-10-24; documentation, unpinned.
- **U3**: [Cursor subagents](https://cursor.com/docs/subagents) — 2026-10-24; documentation, unpinned.
- **U4**: [Cursor hooks](https://cursor.com/docs/hooks) — 2026-10-24; documentation, unpinned.
- **U5**: [Cursor MCP](https://cursor.com/docs/mcp) — 2026-10-24; documentation, unpinned.
- **G1**: [Gemini instruction hierarchy](https://geminicli.com/docs/cli/gemini-md) — 2026-10-24; documentation, unpinned.
- **G2**: [Gemini skills](https://geminicli.com/docs/cli/skills) — 2026-10-24; documentation, unpinned.
- **G3**: [Gemini commands](https://geminicli.com/docs/cli/custom-commands) — 2026-10-24; documentation, unpinned.
- **G4**: [Gemini subagents](https://geminicli.com/docs/core/subagents) — 2026-10-24; documentation, unpinned.
- **G5**: [Gemini hooks](https://geminicli.com/docs/hooks) — 2026-10-24; documentation, unpinned.
- **G6**: [Gemini settings and system paths](https://geminicli.com/docs/reference/configuration) — 2026-10-24; documentation, unpinned.
- **G7**: [Gemini enterprise home isolation](https://geminicli.com/docs/cli/enterprise) — 2026-10-24; documentation, unpinned.
- **G8**: [Gemini MCP](https://geminicli.com/docs/tools/mcp-server) — 2026-10-24; documentation, unpinned.
- **G9**: [Gemini skill link command](https://geminicli.com/docs/cli/cli-reference) — 2026-10-24; documentation, unpinned.
- **P1**: [OpenCode config](https://opencode.ai/docs/config/) — 2026-10-24; documentation, unpinned.
- **P2**: [OpenCode rules](https://opencode.ai/docs/rules/) — 2026-10-24; documentation, unpinned.
- **P3**: [OpenCode skills](https://opencode.ai/docs/skills/) — 2026-10-24; documentation, unpinned.
- **P4**: [OpenCode commands](https://opencode.ai/docs/commands/) — 2026-10-24; documentation, unpinned.
- **P5**: [OpenCode agents](https://opencode.ai/docs/agents/) — 2026-10-24; documentation, unpinned.
- **P6**: [OpenCode plugins](https://opencode.ai/docs/plugins/) — 2026-10-24; documentation, unpinned.
- **P7**: [OpenCode MCP](https://opencode.ai/docs/mcp-servers/) — 2026-10-24; documentation, unpinned.
- **H1**: [Copilot CLI directory reference](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-config-dir-reference) — 2026-10-24; documentation, unpinned.
- **H2**: [Copilot CLI instructions](https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/add-custom-instructions) — 2026-10-24; documentation, unpinned.
- **H3**: [Copilot CLI skills](https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/add-skills) — 2026-10-24; documentation, unpinned.
- **H4**: [Copilot CLI hooks](https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/use-hooks) — 2026-10-24; documentation, unpinned.
- **H5**: [Copilot CLI MCP](https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/add-mcp-servers) — 2026-10-24; documentation, unpinned.
- **H6**: [Copilot CLI native home paths](https://docs.github.com/en/copilot/how-tos/copilot-cli/set-up-copilot-cli/configure-copilot-cli) — 2026-10-24; documentation, unpinned.
- **V1**: [VS Code instructions by harness](https://code.visualstudio.com/docs/agent-customization/custom-instructions) — 2026-10-24; documentation, unpinned.
- **V2**: [VS Code skills](https://code.visualstudio.com/docs/agent-customization/agent-skills) — 2026-10-24; documentation, unpinned.
- **V3**: [VS Code prompt files](https://code.visualstudio.com/docs/agent-customization/prompt-files) — 2026-10-24; documentation, unpinned.
- **V4**: [VS Code custom agents](https://code.visualstudio.com/docs/agent-customization/custom-agents) — 2026-10-24; documentation, unpinned.
- **V5**: [VS Code hooks](https://code.visualstudio.com/docs/agent-customization/hooks) — 2026-10-24; documentation, unpinned.
- **V6**: [VS Code MCP destinations](https://code.visualstudio.com/docs/agent-customization/mcp-servers) — 2026-10-24; documentation, unpinned.
- **V7**: [VS Code settings and profiles](https://code.visualstudio.com/docs/configure/settings) — 2026-10-24; documentation, unpinned.
- **W1**: [Windsurf redirected rules documentation](https://docs.devin.ai/desktop/cascade/memories) — 2026-10-24; documentation, unpinned.
- **W2**: [Windsurf redirected skills documentation](https://docs.devin.ai/desktop/cascade/skills) — 2026-10-24; documentation, unpinned.
- **W3**: [Windsurf redirected workflows documentation](https://docs.devin.ai/desktop/cascade/workflows) — 2026-10-24; documentation, unpinned.
- **W4**: [Windsurf redirected hooks documentation](https://docs.devin.ai/desktop/cascade/hooks) — 2026-10-24; documentation, unpinned.
- **W5**: [Windsurf redirected MCP documentation](https://docs.devin.ai/desktop/cascade/mcp) — 2026-10-24; documentation, unpinned.
- **A1**: [Amp instructions](https://ampcode.com/docs/customize/agents-md) — 2026-10-24; documentation, unpinned.
- **A2**: [Amp settings](https://ampcode.com/docs/cli/settings) — 2026-10-24; documentation, unpinned.
- **A3**: [Amp skills](https://ampcode.com/docs/customize/skills) — 2026-10-24; documentation, unpinned.
- **L1**: [Cline configuration layout](https://docs.cline.bot/getting-started/config) — 2026-10-24; documentation, unpinned.
- **L2**: [Cline rules](https://docs.cline.bot/customization/cline-rules) — 2026-10-24; documentation, unpinned.
- **L3**: [Cline skills](https://docs.cline.bot/customization/skills) — 2026-10-24; documentation, unpinned.
- **R1**: [Roo custom instructions](https://roocodeinc.github.io/Roo-Code/features/custom-instructions/) — 2026-10-24; documentation, unpinned.
- **K1**: [Kilo official documentation, Custom Rules section](https://kilo.ai/docs/llms.txt) — 2026-10-24; documentation, unpinned.
- **N1**: [Continue config paths](https://docs.continue.dev/customize/deep-dives/configuration) — 2026-10-24; documentation, unpinned.
- **N2**: [Continue YAML reference](https://docs.continue.dev/reference) — 2026-10-24; documentation, unpinned.
- **N3**: [Continue rules](https://docs.continue.dev/customize/deep-dives/rules) — 2026-10-24; documentation, unpinned.
- **E1**: [OpenHands general repository skills](https://docs.openhands.dev/overview/skills/repo) — 2026-10-24; documentation, unpinned.
- **B1**: [Goose official configuration documentation source](https://github.com/block/goose/blob/main/documentation/docs/guides/config-files.md) — 2026-10-24; documentation, unpinned.
- **I1**: [Pi official configuration documentation source](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/configuration.md) — 2026-10-24; documentation, unpinned.
- **I2**: [Pi official skills documentation source](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/skills.md) — 2026-10-24; documentation, unpinned.
- **I3**: [Pi official MCP documentation source](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/mcp.md) — 2026-10-24; documentation, unpinned.
- **T1**: [Antigravity rules](https://antigravity.google/docs/rules) — 2026-10-24; documentation, unpinned.
- **T2**: [Antigravity settings by surface](https://antigravity.google/docs/settings?tab=ide) — 2026-10-24; documentation, unpinned.
- **F1**: [Factory Droid settings](https://docs.factory.com/droid-cli/settings) — 2026-10-24; documentation, unpinned.
- **F2**: [Factory Droid instruction discovery](https://docs.factory.com/harness/agents-md) — 2026-10-24; documentation, unpinned.
- **F3**: [Factory Droid skills](https://docs.factory.com/harness/skills) — 2026-10-24; documentation, unpinned.
- **Y1**: [Aider YAML configuration](https://aider.chat/docs/config/aider_conf.html) — 2026-10-24; documentation, unpinned.
- **Y2**: [Aider coding conventions](https://aider.chat/docs/usage/conventions.html) — 2026-10-24; documentation, unpinned.

[C1]: https://code.claude.com/docs/en/memory
[C2]: https://code.claude.com/docs/en/settings
[C3]: https://code.claude.com/docs/en/skills
[C4]: https://code.claude.com/docs/en/sub-agents
[C5]: https://code.claude.com/docs/en/hooks
[C6]: https://code.claude.com/docs/en/mcp
[O1]: https://learn.chatgpt.com/docs/agent-configuration/agents-md
[O2]: https://learn.chatgpt.com/docs/build-skills
[O3]: https://learn.chatgpt.com/docs/config-file/config-basic
[O4]: https://learn.chatgpt.com/docs/custom-prompts
[O5]: https://learn.chatgpt.com/docs/agent-configuration/subagents
[O6]: https://learn.chatgpt.com/docs/hooks
[O7]: https://learn.chatgpt.com/docs/agent-configuration/rules
[O8]: https://learn.chatgpt.com/docs/extend/mcp
[U1]: https://cursor.com/docs/rules
[U2]: https://cursor.com/docs/skills
[U3]: https://cursor.com/docs/subagents
[U4]: https://cursor.com/docs/hooks
[U5]: https://cursor.com/docs/mcp
[G1]: https://geminicli.com/docs/cli/gemini-md
[G2]: https://geminicli.com/docs/cli/skills
[G3]: https://geminicli.com/docs/cli/custom-commands
[G4]: https://geminicli.com/docs/core/subagents
[G5]: https://geminicli.com/docs/hooks
[G6]: https://geminicli.com/docs/reference/configuration
[G7]: https://geminicli.com/docs/cli/enterprise
[G8]: https://geminicli.com/docs/tools/mcp-server
[G9]: https://geminicli.com/docs/cli/cli-reference
[P1]: https://opencode.ai/docs/config/
[P2]: https://opencode.ai/docs/rules/
[P3]: https://opencode.ai/docs/skills/
[P4]: https://opencode.ai/docs/commands/
[P5]: https://opencode.ai/docs/agents/
[P6]: https://opencode.ai/docs/plugins/
[P7]: https://opencode.ai/docs/mcp-servers/
[H1]: https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-config-dir-reference
[H2]: https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/add-custom-instructions
[H3]: https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/add-skills
[H4]: https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/use-hooks
[H5]: https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/add-mcp-servers
[H6]: https://docs.github.com/en/copilot/how-tos/copilot-cli/set-up-copilot-cli/configure-copilot-cli
[V1]: https://code.visualstudio.com/docs/agent-customization/custom-instructions
[V2]: https://code.visualstudio.com/docs/agent-customization/agent-skills
[V3]: https://code.visualstudio.com/docs/agent-customization/prompt-files
[V4]: https://code.visualstudio.com/docs/agent-customization/custom-agents
[V5]: https://code.visualstudio.com/docs/agent-customization/hooks
[V6]: https://code.visualstudio.com/docs/agent-customization/mcp-servers
[V7]: https://code.visualstudio.com/docs/configure/settings
[W1]: https://docs.devin.ai/desktop/cascade/memories
[W2]: https://docs.devin.ai/desktop/cascade/skills
[W3]: https://docs.devin.ai/desktop/cascade/workflows
[W4]: https://docs.devin.ai/desktop/cascade/hooks
[W5]: https://docs.devin.ai/desktop/cascade/mcp
[A1]: https://ampcode.com/docs/customize/agents-md
[A2]: https://ampcode.com/docs/cli/settings
[A3]: https://ampcode.com/docs/customize/skills
[L1]: https://docs.cline.bot/getting-started/config
[L2]: https://docs.cline.bot/customization/cline-rules
[L3]: https://docs.cline.bot/customization/skills
[R1]: https://roocodeinc.github.io/Roo-Code/features/custom-instructions/
[K1]: https://kilo.ai/docs/llms.txt
[N1]: https://docs.continue.dev/customize/deep-dives/configuration
[N2]: https://docs.continue.dev/reference
[N3]: https://docs.continue.dev/customize/deep-dives/rules
[E1]: https://docs.openhands.dev/overview/skills/repo
[B1]: https://github.com/block/goose/blob/main/documentation/docs/guides/config-files.md
[I1]: https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/configuration.md
[I2]: https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/skills.md
[I3]: https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/mcp.md
[T1]: https://antigravity.google/docs/rules
[T2]: https://antigravity.google/docs/settings?tab=ide
[F1]: https://docs.factory.com/droid-cli/settings
[F2]: https://docs.factory.com/harness/agents-md
[F3]: https://docs.factory.com/harness/skills
[Y1]: https://aider.chat/docs/config/aider_conf.html
[Y2]: https://aider.chat/docs/usage/conventions.html
