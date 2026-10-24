# Working on KitBag

## Setup and checks

- Install the pinned tools with `mise install` (trust `mise.toml` when prompted).
- Use `mise exec -- go build ./...` and `mise exec -- go test ./...` now.
- Run `mise exec -- go vet ./...` and `mise exec -- gofumpt -d .` before a PR.
- Run `mise exec -- go run ./cmd/kitbag version` to check the CLI.
- The required `mise run check` gate arrives in KIT-8; this bootstrap does not
  define it. Once available, run it before every PR. CI arrives in KIT-9.
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
- Record architecture decisions in [docs/adr](docs/adr/). Read relevant ADRs
  before changing package responsibilities. Dependency linting arrives in KIT-8.

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
