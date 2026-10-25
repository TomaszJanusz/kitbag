# First-run setup

`kitbag init <git-url>` prepares a machine without applying agent files. It
clones through the selected Git backend into `<data>/repos/<name>`, detects
existing supported agent directories, builds the ordinary dry plan, asks for
confirmation, and atomically creates `<config>/config.toml` with mode 0600.
Run the printed `kitbag --config '<file>' sync` command to apply the plan.
`--config` and `KITBAG_CONFIG` select another configuration file.

```sh
kitbag init https://example.com/agent-config.git
kitbag init https://example.com/agent-config.git --yes --git-backend=embedded --mode=copy
```

Every prompt has a flag. `--yes` accepts defaults and confirmation and never
reads terminal input. Without it, input must be a terminal. The wizard uses
huh v1.0.0 (the stable `github.com/charmbracelet/huh` module line); exact direct
and transitive versions are recorded in go.mod/go.sum. Huh stays in cmd/kitbag,
with a depguard boundary, following ADR 5. Core setup policy uses injected
platform, VCS and configuration contracts and has no renderer dependency.
Keyboard model tests drive the actual huh forms without starting a TTY.

| Question or choice | Flag | Default |
| --- | --- | --- |
| Repository URL | positional URL or `--repo` | Required; prompted interactively |
| Git backend | `--git-backend` | Environment, then `auto` |
| Materialization mode | `--mode` | Environment, then `link` |
| Commit name, only when Git lacks an identity | `--identity-name` | Headless consumers may leave unset |
| Commit email, only when Git lacks an identity | `--identity-email` | Headless consumers may leave unset |
| Confirm writing machine configuration | `--yes` | Decline |
| Scaffold agents | `--agents=claude,codex` | Detected kinds, else Claude Code |
| Scaffold named instances | repeat `--instance=claude.main=~/.claude` | Adapter defaults |
| Confirm creating scaffold files and Git metadata | `--yes` | Decline |

`--branch` selects the clone branch; the default is the remote's default branch.
`--host` persists a machine host override. Other global precedence rules are
unchanged. Both identity flags must be supplied together. They configure only
fallback unsigned embedded commits; system Git keeps its own identity and
signing configuration. Neither setup path creates a commit. Identity is not
required for clone or sync; `--yes` prints guidance when it is missing.

## Detection and existing repositories

Detection checks only whether `.claude` and `.codex` are real directories,
through platform FS/Env. `CLAUDE_CONFIG_DIR` and `CODEX_HOME` replace their
respective defaults; the override must be absolute. Missing directories,
regular files and final symlinks are not detected. Detection never lists HOME
or reads agent settings, credentials, sessions or instance files, and it never
runs an agent executable. Directory presence is a conservative hint, not proof
that an agent executable is installed. Additional instances are explicit.

For an imported repository, targets and instances remain the policy declared
in `kitbag.toml`, including host overrides. The wizard displays detected
directories for review; it does not rewrite the manifest or invent machine
selection fields absent from config schema 1. `--agents` and `--instance` apply
only to scaffold. To select agents for an existing repository, edit its targets
or host overrides before running sync. Dry planning can read managed destination
contents as part of its normal drift/settings comparison; that is separate from
directory-only detection.

Any existing configuration entry wins, even invalid TOML, a directory or a
dangling link. Repeating init exits successfully and explains that setup already
exists, without reading its contents, cloning again or changing it. If setup
was cancelled after cloning, the clone remains available for another attempt.
An existing clone is reused only when it is a real directory with the exact
requested origin and branch. A different origin, different requested branch or
non-repository is an error; init never resets or fetches an existing clone.
A failed clone may leave partial files; inspect them before retrying.

Configuration creation stages a complete validated TOML file and installs it
with the platform's no-replace rename. A competing creator's configuration
wins. A reusable `.init.lock` file serializes writers; completed writes remove
their `.init-new` staging entry. A process killed before cleanup can leave a
staging entry; init refuses to overwrite it, so inspect it before removing it.
Planning can create the ordinary state lock directory/file but never saves state
or modifies targets, backups or Git includes.

## Starting a new repository

```sh
kitbag init --scaffold ./agent-config
kitbag init --scaffold ./agent-config --yes --git-backend=embedded \
  --agents=claude,codex --instance=claude.main=~/.claude \
  --instance=codex.main=~/.codex
```

The scaffold matches `schemas/examples/kitbag/valid/minimal.toml`: schema 1,
`instructions.source = "AGENTS.md"`, and selected supported targets. It creates:

- `kitbag.toml`, with optional named instances using the manifest home-path grammar.
- `AGENTS.md`, a shared instructions skeleton.
- `README.md`, with commit/push and first-sync guidance.
- `.git`, with an unborn `main` branch, through the selected VCS initializer.

It writes no machine configuration and creates no agent directories. Existing
template files are never overwritten. A complete existing template is reported
as already scaffolded and exits successfully; a partial existing template is
refused before any template file is written. Existing Git metadata is refused.
The scaffold is parsed and planned with the real adapters in core tests; CLI
scripts also check the generated files with the ordinary plan command.

## Service installation

Init prints the manual sync command and `kitbag --config <path> service install`
for optional background synchronization. Run that command after reviewing the
dry plan; init itself does not start a background process. See
[service management](service.md) for daemon/timer modes and headless Linux setup.
