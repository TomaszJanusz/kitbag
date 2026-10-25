# Contributing to KitBag

Read [AGENTS.md](AGENTS.md) for setup, architecture boundaries, checks and workspace
hygiene. The same guidance applies to human and automated contributions.

Install the pinned tools with `mise install`, enable hooks with
`mise run hooks:install`, and run `mise run check` before opening a pull request.
For workflow changes also run `mise run check:workflows`. Add tests for meaningful
behavior changes; CLI scenarios use the isolated HOME testscript harness described
in [README.md](README.md).

Use a feature branch and one focused PR against `main`. PR titles must follow
Conventional Commits: `type(scope): description`, with an optional scope and an
optional `!` for breaking changes. Allowed types are `feat`, `fix`, `docs`, `style`,
`refactor`, `perf`, `test`, `build`, `ci`, `chore` and `revert`. For example,
`feat(manifest): validate source names`. Scopes use letters, numbers, `.`, `_`,
`/` or `-`, starting with a letter or number. Descriptions must not be empty.

Complete the PR template with the outcome, rationale, checks and anything not
verified. PRs merge by squash only; the PR title becomes the commit subject, so
keep it accurate after revisions. See [repository governance](docs/governance.md)
for required checks, the solo-maintainer review policy and maintainer setup.

Report vulnerabilities using [SECURITY.md](SECURITY.md).

Documentation links are checked offline by `mise run check` (or separately with
`mise run docs:links:check`). The check covers README.md, CONTRIBUTING.md,
AGENTS.md, SECURITY.md and every Markdown file under `docs/`. Relative file and
directory links must exist; fragments must match a GitHub-style heading anchor
or an explicit HTML `<a id="...">` anchor. Inline and reference-style links are
checked, including images; code examples and external URLs are excluded. Escape
or percent-encode parentheses in link destinations for the Markdown parser.
