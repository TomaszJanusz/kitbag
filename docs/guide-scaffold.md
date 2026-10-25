# Guide: your own configuration repository from scratch

This guide creates a new Git repository that carries your agents' shared
instructions and one skill, syncs it to this machine, and publishes it so a second
machine can clone it. It needs a [built or installed `kitbag`](../README.md#install)
and Git. Every command here runs against your real agent directories, so read the
`kitbag plan` output before `kitbag sync`.

## 1. Scaffold the repository

```sh
kitbag init --scaffold ~/agent-config
```

Without `--yes` the command asks before writing. It detects `~/.claude` and
`~/.codex` (or `CLAUDE_CONFIG_DIR` / `CODEX_HOME`) and scaffolds a target for each
detected agent, else Claude Code. Choose explicitly with `--agents=claude,codex`
and name instances with `--instance=claude.main=~/.claude`; see
[first-run setup](init.md#starting-a-new-repository). The scaffold writes
`kitbag.toml`, `AGENTS.md`, a `README.md` and an empty Git repository on `main`. It
writes no machine configuration and creates no agent directory.

## 2. Edit `kitbag.toml` and your instructions

`kitbag.toml` starts as the [minimal manifest](manifest.md#example-minimal-manifest):

```toml
schema = 1

[instructions]
source = "AGENTS.md"

[targets.claude]
agent = "claude"
```

`AGENTS.md` becomes each agent's global instructions (Claude Code imports it from
a generated `CLAUDE.md`; Codex reads it as `AGENTS.md`). Replace its placeholder
with your real guidance. Add settings, commands, MCP definitions or per-host
instructions as described in the [manifest reference](manifest.md).

## 3. Add a skill

A skill is a directory containing `SKILL.md`. Declare where skills live with a
`[skills]` table in `kitbag.toml`:

```toml
[skills]
dirs = ["skills"]
```

```sh
mkdir -p ~/agent-config/skills/hello
cat > ~/agent-config/skills/hello/SKILL.md <<'SKILL'
---
name: hello
description: Say hello.
---
Say hello to the user.
SKILL
```

Each skill is linked into every enabled agent's skills directory
(`~/.claude/skills/<name>`, and the shared `~/.agents/skills/<name>` for Codex).

## 4. Commit, publish and connect

kitbag syncs from a Git remote, so the repository needs one. Create an empty
repository on your Git host (or a bare one, `git init --bare ~/agent-config.git`),
then:

```sh
cd ~/agent-config
git add -A && git commit -m "feat: initial agent configuration"
git remote add origin <your-remote-url>
git push -u origin main
kitbag init <your-remote-url>
```

`kitbag init <url>` clones into the platform data directory, prints a dry plan
and, after you confirm, writes the machine configuration. Your scaffold directory
is only the place where you authored the first commit; kitbag works from its own
clone, so edit and push from either place and use `kitbag sync` to pull.

## 5. Sync

```sh
kitbag plan     # review what will change; nothing is written
kitbag sync     # pull, then apply once
kitbag sync     # a second run prints nothing: everything is in sync
```

Default agent directories are created when needed; an explicitly listed instance
directory (`instances.<name>.dir`) that is missing is skipped with a warning. A pre-existing file at a managed path is moved to a one-time backup next to it
(`<name>.before-kitbag.<time>`) before kitbag writes its own. Check the result with
`kitbag status` and `kitbag doctor`.

## 6. Change, publish, sync elsewhere

Edit files in the working copy (`repo.path`, shown by `kitbag status`), commit and
push through Git or a PR as usual. On every other machine, `kitbag sync` pulls
and applies; `kitbag service install` does that in the background (see
[background synchronization](service.md)).

## See also

[Everyday commands](commands.md), [manifest reference](manifest.md),
[generated command reference](cli/kitbag_init.md), [ADR 2: Git as the source of
truth](adr/0002-git-source-of-truth.md).
