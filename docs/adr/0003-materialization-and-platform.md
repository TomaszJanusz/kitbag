# 3. Default to Unix symlinks and isolate platform differences

- Status: Accepted
- Date: 2026-10-24

## Context and Problem Statement

Users may edit configuration through an agent's directory or directly in the
repository. Unix symlinks preserve that connection, while some agents and future
Windows installations need copies. Platform differences must not spread through
the synchronization core.

## Decision Drivers

- Direct editing of the working copy on macOS and Linux.
- Copy support for targets that do not follow links.
- Future Windows support without rewriting core logic.
- Protection of local edits when copies are used.

## Considered Options

- Symlinks by default on Unix, with an explicit copy option.
- Copies by default on every operating system.
- Platform-specific behavior scattered throughout the core.

## Decision Outcome

Ship v1 for macOS and Linux with `link` as the default mode, creating symlinks to
the working copy. Offer `--mode copy` for incompatible targets. Windows follows
in M6 with copy as its default; optional link behavior must account for junctions,
symlink permissions, and file targets.

Copy mode compares the source, last applied state, and current destination to
detect drift. Local edits must not be silently overwritten: status reports drift,
explicit force can discard it, and adoption can bring it into the repository.
In link mode, edits at the destination are working-copy changes.

Put every OS difference behind `internal/platform`: materialization and atomic
replacement, paths, service management, keyrings, opening files or URLs,
permissions, and filesystem naming constraints. Keep the core independent of OS
implementations. Plan semantics are shared by link and copy; execution differs.
Windows must compile from M1, with unsupported operations reported explicitly
until implemented. Compilation alone is not a claim of Windows support.

### Consequences

- Unix users retain edit-in-place behavior without copying changes back.
- Copy mode requires state and conflict detection rather than blind overwrites.
- Platform-specific tests and eventual real Windows testing remain necessary.
- This supersedes the earlier proposal to default to copies on Unix and to
  support all three systems equally from the first release.

## Links

- Planning source: plan §0, revised §2.3, and §18.1; see the
  [source and revision notes](README.md#planning-sources-and-revisions).
