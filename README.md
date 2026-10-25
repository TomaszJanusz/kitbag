# kitbag

[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/TomaszJanusz/kitbag/badge)](https://scorecard.dev/viewer/?uri=github.com/TomaszJanusz/kitbag)

kitbag is a planned tool for synchronizing AI coding agents' global configuration
from a Git repository across machines. The initial target platforms are macOS and
Linux; Windows support follows later.

The project is in discovery. See the [architecture decision records](docs/adr/README.md)
for the accepted planning decisions and their rationale.

KitBag is being designed for developers who want one Git repository to carry
agents' global instructions, skills, commands, MCP definitions and settings
across their machines. [APM](https://github.com/microsoft/apm) focuses on agent
package resolution, deployment and audit; [agentsync](https://github.com/dallay/agentsync)
focuses on shared configuration and per-agent rendering. KitBag's intended focus
is the ongoing workflow between machines: detecting local changes, reviewing
publication through Git and PRs, and exposing sync state through a daemon and
shared UI. These capabilities are planned; see the [prior-art review](docs/prior-art.md)
for overlap, reuse recommendations and limits.

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
