# Working on KitBag

## Setup and checks

- Install the pinned tools with `mise install` (trust `mise.toml` when prompted).
- Run `mise run hooks:install` after setup to enable pre-commit checks.
- Run `mise run check` before every PR: read-only formatting, lint, vet, race
  tests with shuffle, govulncheck and typos.
- CI checks: `check (ubuntu-latest)`, `check (macos-latest)`, `windows`,
  `modules`, `security`; `ci-success` aggregates them for branch protection.
- Use `mise run format` to fix Go formatting; `mise run format:check` only checks.
- Hooks format staged Go files, lint changed packages, scan staged secrets and
  check spelling; lefthook preserves partially staged edits. Bash is required.
- Run `mise exec -- go run ./cmd/kitbag version` to check the CLI.
- Keep every tool pinned in `mise.toml`; do not install global workarounds.

## Architecture

- `cmd/kitbag`: CLI entry point; orchestration belongs in `internal/core`.
- `internal/core`: synchronization policy, independent of the OS and UI.
- `internal/platform`: host contracts; `unix`, `windows` and `fake` implement them.
  OS-specific filesystem, services and credential operations belong here.
- `internal/adapters/<agent>`: agent paths, instruction and settings formats.
- `internal/vcs`: Git contracts and backends.
- `internal/ui/model`: shared view models, independent of UI renderers.
- `pkg/manifest`: the only public library package; treat its API as a contract.
- `internal/e2e`: test-only testscript harness; prove CLI behaviour with scripts
  in `cmd/kitbag/testdata/script` (see README), never against the real HOME.
- Record architecture decisions in [docs/adr](docs/adr/). Read relevant ADRs
  before changing package responsibilities. Update `.golangci.yml` allow-lists
  when adding dependencies or agents; adapters may import only their own subtree.

## Pull requests

- Never push to main. Use a feature branch and a reviewed PR against `main`.
- Use Conventional Commits for commit messages and PR titles.
- Keep one focused change per PR; include tests for meaningful behavior changes.
- Explain the outcome, rationale, checks and anything not verified in the PR.
- Squash is the only allowed merge policy.
- Before pushing workflow changes, run `actionlint` and `zizmor` when installed.
- Before committing files that might hold secrets, run `gitleaks git --staged`.

## Workspace hygiene

- Put scratch files under `/tmp/kitbag-*` or an ignored directory; clean them up.
- Never commit logs, coverage, screenshots or browser/test tool artifacts.
- Stop servers, watchers and browser sessions you start.
