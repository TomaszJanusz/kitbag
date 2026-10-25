# kitbag

[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/TomaszJanusz/kitbag/badge)](https://scorecard.dev/viewer/?uri=github.com/TomaszJanusz/kitbag)

kitbag keeps your AI coding agents' global instructions, skills, commands and
settings in one Git repository and syncs them to every machine you use (macOS and
Linux; Windows is not supported before 1.x).

## Install

There is **no release yet**, so today you build from source with the pinned
toolchain ([mise](https://mise.jdx.dev) and Git are the only prerequisites):

```sh
git clone https://github.com/TomaszJanusz/kitbag.git && cd kitbag
mise trust && mise install
mise exec -- go build -ldflags "-X main.version=0.0.0-src" -o kitbag ./cmd/kitbag
./kitbag version   # then put ./kitbag on your PATH, e.g. mv kitbag ~/.local/bin/
```

**From v0.1.0** (not available yet; nothing below is published):
`brew install TomaszJanusz/tap/kitbag`, the
`curl -fsSL https://github.com/TomaszJanusz/kitbag/releases/latest/download/install.sh | sh`
script, `.deb`/`.rpm` files from the release page, or `yay -S kitbag-bin`. See
[installing kitbag](docs/install.md) for details; the channel mechanics are
checked in CI against snapshot builds, not against a published release.

## Quick start

1. **Install** as above, so `kitbag version` works.
2. **Connect a configuration repository.** If you already have one, clone it
   through kitbag:

   ```sh
   kitbag init https://example.com/you/agent-config.git
   ```

   For a new repository, scaffold it, commit and push it to an empty remote
   (needs a Git identity: `git config --global user.name`/`user.email`), then
   connect it ([details](docs/guide-scaffold.md)):

   ```sh
   kitbag init --scaffold ~/agent-config
   git -C ~/agent-config add -A
   git -C ~/agent-config commit -m "Initial agent configuration"
   git -C ~/agent-config remote add origin https://example.com/you/agent-config.git
   git -C ~/agent-config push -u origin main
   kitbag init https://example.com/you/agent-config.git
   ```

   `init` shows a dry plan and asks before writing machine configuration; it
   applies nothing yet. Add `--yes` to skip the questions.
3. **Sync.** Apply the plan:

   ```sh
   kitbag sync
   ```

   Run it again: a second run prints nothing when everything is in sync. Optionally
   keep the machine in sync in the background with `kitbag service install` (see
   [background synchronization](docs/service.md)), and diagnose problems with
   `kitbag doctor`.

   For an interactive view, run `kitbag tui`: Status, Drift, Settings and
   Activity tabs (press `?` for keys). It uses the daemon when one runs and reads
   this machine directly otherwise, honours `NO_COLOR`, and exits 2 when stdin or
   stdout is not a terminal. See the [terminal UI](docs/tui.md).

Coming from the `agent-config-sync` script? Follow the
[migration guide](docs/guide-migrate-agent-config-sync.md). Every command and flag
is in the [generated command reference](docs/cli/kitbag.md).

## Supported agents

| Agent | Status |
| --- | --- |
| Claude Code | First-class: verified in tests and in runs against a real user account on Linux ([adapter](docs/claude.md)) |
| Codex CLI | First-class: verified in tests and in runs against a real user account on Linux ([adapter](docs/adapters-codex.md)) |
| Gemini CLI | Adapter implemented, **not verified** against the real agent ([adapter](docs/adapters-gemini.md)) |
| OpenCode | Adapter implemented, **not verified** against the real agent ([adapter](docs/adapters-opencode.md)) |
| Other agents | Declare directories and roots with the [generic adapter](docs/generic.md) |
| Cursor, Windsurf, GitHub Copilot | Not supported yet |

The [agent inventory](docs/agents.md) records the documented locations per agent.
macOS behaviour is exercised by CI on macOS, but not by real-agent runs there.

## How it differs

[APM](https://github.com/microsoft/apm) focuses on agent package resolution,
deployment and audit; [agentsync](https://github.com/dallay/agentsync) focuses on
shared configuration and per-agent rendering. kitbag's focus is the ongoing
workflow between machines: detecting local changes, reviewing publication through
Git and PRs, and exposing sync state through a daemon and local API. The
[prior-art review](docs/prior-art.md) covers overlap, reuse and limits.

## Privacy

kitbag collects no telemetry and has no analytics or crash reporting. Its only
network traffic is Git talking to your own configuration remote, plus, while the
[daemon](docs/daemon.md) runs, one daily request for the latest GitHub release
metadata (notification only; it never downloads or installs). Turn that off with
`kitbag config set daemon.update_check false`. `kitbag self-update` contacts the
release host only when you run it.

## Documentation

[Everyday commands](docs/commands.md), [first-run setup](docs/init.md),
[manifest reference](docs/manifest.md), [machine settings](docs/config.md),
[doctor](docs/doctor.md), [architecture decision records](docs/adr/README.md) and the
[release process](docs/release.md).

## Development

For local development, install the pinned tools with `mise trust` and
`mise install`, then enable pre-commit checks with `mise run hooks:install`.
Run `mise run check` before opening a PR; it checks formatting without changing
files, lints, vets, runs race tests with shuffled order, checks vulnerabilities
and checks spelling. Use `mise run format` to apply Go formatting. Local hook
scripts require Bash and Git; see [AGENTS.md](AGENTS.md) for architecture rules.

## Writing end-to-end scripts

Every CLI behaviour is proven by a [testscript](https://pkg.go.dev/github.com/rogpeppe/go-internal/testscript)
script that runs the real `kitbag` main function as a separate process against
a throwaway HOME. Scripts live in `cmd/kitbag/testdata/script/*.txtar` and run
in `mise run check` (so on Linux and macOS CI); run only them with
`mise exec -- go test ./cmd/kitbag -run TestScripts/<name>`. Start from
[`version.txtar`](cmd/kitbag/testdata/script/version.txtar) and the
`testscript` command reference; start the binary with `exec kitbag ...`.

The harness (`internal/e2e`, test-only) prepares each script:

- `HOME` is `$WORK/home`, empty unless the archive has `home/...` sections.
- Git sees only `$WORK/.gitconfig`: no system configuration, fixed author,
  committer and dates, no signing, no hooks, local file remotes only.

It adds these commands; the assertions accept `!` to negate them:

| Command | Purpose |
| --- | --- |
| `agentdirs dir...` | create agent directories below `$HOME`, e.g. `.claude .codex` |
| `gitremote bare [seed-dir]` | create a bare repository on `main`, seeded with a commit of `seed-dir` |
| `linktarget path [target]` | assert `path` is a symlink, to exactly `target` if given |
| `content file text` | assert `file` holds exactly `text` and a newline (reads through symlinks) |
| `nooutput` | assert the last command wrote nothing to stdout or stderr |
| `hidecmd program...` | remove `PATH` entries holding `program`, e.g. `hidecmd git` to run without Git |

Use the built-in `cmp` for multi-line content and `exists` for presence.

**Mandatory scenarios.** Each feature ticket copies
[`mandatory_scenarios.txtar`](internal/e2e/testdata/script/mandatory_scenarios.txtar)
into `cmd/kitbag/testdata/script/<feature>.txtar`, replaces the `stubsync`
stand-in with `exec kitbag <command>` and keeps all four scenarios: first run,
repeat run prints nothing, a listed instance directory is missing, and the
case the change is about.

Real-adapter acceptance scripts run public `sync`, `plan`, `status`, `adopt`,
`unmanage` and `revert`; see
[everyday commands](docs/commands.md) and the [generated reference](docs/cli/kitbag.md).
Fixture-only apply cases retain the test-binary-only `__e2e_apply` entry.
`execstatus code command ...` asserts exact process exits.
`planschema file` validates actual `plan --json` output against its JSON Schema
(`adoptschema` and `historyschema` likewise). `homefixture dir` installs a
recorded HOME and `treesnap root file` fingerprints a tree, including
modification times, to prove that a command changed nothing.

**HOME guard.** The harness refuses to run, and `kitbag` started by a script
exits with status 125, when `HOME` is the real home directory or is not a
directory inside `$WORK`, including after `env HOME=...` in a script. Never
point a script at real agent directories.

Licensed under the [MIT License](LICENSE).
