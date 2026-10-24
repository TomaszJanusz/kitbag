# 4. Prefer system Git with an embedded fallback

- Status: Accepted
- Date: 2026-10-24

## Context and Problem Statement

Developer machines already have Git configured with credentials, signing, hooks,
and merge tools. Other machines only consume configuration and should not need
Git or GitHub CLI installed before they can synchronize.

## Decision Drivers

- Preserve the user's Git behavior when available.
- Support basic synchronization without external Git installation.
- Make backend choice and capability limits explicit.
- Avoid requiring `gh` or bundling a separate Git binary.

## Considered Options

- Prefer system Git, with go-git embedded as fallback.
- Require system Git everywhere.
- Use only go-git.
- Bundle Git or use libgit2 bindings.

## Decision Outcome

Provide `system` and `embedded` implementations behind the Git backend boundary.
In automatic mode, prefer system Git when a suitable version (at least 2.30) is
available; otherwise use embedded go-git. Select explicitly through
`--git-backend=system|embedded`, then `KITBAG_GIT_BACKEND`, then the tool's
`git.backend` configuration (`auto`, `system`, or `embedded`), in that precedence.
Forcing system Git when it is missing is an error, not a silent fallback.
Diagnostics show the active backend and where the selection came from.

System Git preserves credential helpers, SSH configuration, signing, hooks,
`rerere`, and merge drivers. Embedded go-git covers basic clone, fetch,
fast-forward, status, commit, and push. It must disclose unsupported operations
and direct users to system Git or the working copy rather than pretending to
provide full parity. Embedded commits are unsigned in v1.

kitbag owns embedded credential discovery: SSH agent/configuration/keys and
HTTPS credential sources, with host verification and keyring storage for saved
tokens. It must not assume go-git discovers the user's credentials automatically
or save tokens in plaintext. `gh` may supply credentials when installed and
logged in, but is never required. GitHub features use APIs directly, with a
built-in authentication path; ordinary Git sync is not GitHub-specific.

### Consequences

- Shared basic operations need contract tests against both implementations.
- Embedded mode has a deliberately narrower capability set and additional
  credential-discovery work.
- Bundled Git adds size and a separate update burden; libgit2 adds CGO/build
  complexity. Neither is needed for the intended fallback scope.

## Links

- Planning source: plan §15.1, §15.1a, §15.1b, and §15.2; see the
  [source and revision notes](README.md#planning-sources-and-revisions).
