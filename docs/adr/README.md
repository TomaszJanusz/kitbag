# Architecture decision records

These numbered records use Markdown Architectural Decision Records (MADR) to
capture decisions accepted during planning on 2026-10-24. They describe the intended
architecture, not features already implemented.

1. [Use Go as the implementation language](0001-use-go.md) — Accepted
2. [Keep Git as the source of truth and version the tool separately](0002-git-source-of-truth.md) — Accepted
3. [Default to Unix symlinks and isolate platform differences](0003-materialization-and-platform.md) — Accepted
4. [Prefer system Git with an embedded fallback](0004-git-backends.md) — Accepted
5. [Layer the UI over shared view models and a local API](0005-ui-layers.md) — Accepted
6. [Distribute v1 through macOS and Linux channels](0006-distribution.md) — Accepted
7. [Name the project kitbag](0007-name.md) — Accepted
8. [Use the MIT License](0008-license.md) — Accepted

Follow-up recommendations remain open until accepted:

9. [Preserve skills lock interchange and separate immutable provenance](0009-skill-lock-interchange.md) — Proposed (KIT-2)

Implementation decisions:

10. [Version and authenticate the local API independently](0010-local-api.md) — Accepted (KIT-29)
11. [Keep daemon scheduling independent of host mechanisms](0011-foreground-daemon.md) — Accepted (KIT-30)

## Planning sources and revisions

The source is [KIT-3](https://linear.app/privacybrand/issue/KIT-3/adrs-for-the-decisions-already-taken-plus-licence)
and the [planning document](https://app.notion.com/p/40319f2ad838818587aad3205e6928d3),
with the accepted scope summarized in the
[KitBag project](https://linear.app/privacybrand/project/kitbag-07201812cab1).
Each record cites the relevant plan sections. The records stand alone; reading
those planning sources is not required to understand the decisions.

The plan preserves earlier alternatives. Its later revisions take precedence:

- §12 settles Go over Rust, which §2.5 initially left provisional.
- §0 and revised §2.3 settle Unix symlinks and defer Windows to M6.
- §14.3 replaces §2.2's automatic update-and-retry on an incompatible manifest
  with a fail-safe stop and an explicit tool update.
- §18.2 replaces §17.2's desktop webview direction and §17.4's separate native
  clients with shared Go view models, a menu-only tray, and a later lightweight
  window. The GUI toolkit remains subject to the M0 benchmark.
- §0 settles the name and MIT licence and defers Apple/Azure code signing;
  earlier signing requirements are future goals, not release blockers.
- §19.2 keeps the npm wrapper outside v1. Skill lock compatibility remains
  separate work; these records do not select a lock schema.

Change an accepted decision through a subsequent ADR that explicitly supersedes
it, preserving the original rationale.
