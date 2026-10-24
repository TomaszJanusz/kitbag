# kitbag

kitbag is a planned tool for synchronizing AI coding agents' global configuration
from a Git repository across machines. The initial target platforms are macOS and
Linux; Windows support follows later.

The project is in discovery. See the [architecture decision records](docs/adr/README.md)
for the accepted planning decisions and their rationale.

For local development, install the pinned tools with `mise trust` and
`mise install`, then enable pre-commit checks with `mise run hooks:install`.
Run `mise run check` before opening a PR; it checks formatting without changing
files, lints, vets, runs race tests with shuffled order, checks vulnerabilities
and checks spelling. Use `mise run format` to apply Go formatting. Local hook
scripts require Bash and Git; see [AGENTS.md](AGENTS.md) for architecture rules.

Licensed under the [MIT License](LICENSE).
