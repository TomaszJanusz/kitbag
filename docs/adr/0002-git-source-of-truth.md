# 2. Keep Git as the source of truth and version the tool separately

- Status: Accepted
- Date: 2026-10-24

## Context and Problem Statement

Configuration needs history, review, offline work, and conflict handling across
machines. Bundling the synchronization script with the data it applies creates a
bootstrap problem: a machine first fetches new data with old code, then needs a
second synchronization to apply the new layout correctly.

## Decision Drivers

- Reviewable changes and recoverable history.
- Independent evolution of tool code and configuration data.
- Prompt propagation without replacing the authoritative transport.

## Considered Options

- Git-backed configuration with a separately released tool.
- Pure peer-to-peer file synchronization.
- A dedicated hosted synchronization service.
- Keep synchronization code inside the configuration repository.

## Decision Outcome

Use Git as the authoritative store and data transport. Release the `kitbag` tool
separately from the configuration repository, which contains data and a
`kitbag.toml` manifest. Track managed destinations and their source and hash in
local state so moves and removals become plan differences rather than requiring
a new tool release for each repository layout change.

Fetch the new tree, read its manifest, check `min_tool_version`, and only then
plan and apply. If the installed tool is too old, stop without applying changes,
report the incompatibility, and require an explicit update. This follows the
later fail-safe decision in §14.3 rather than §2.2's automatic update-and-retry.
Tool versions and manifest schema versions are separate concerns; this record
does not define the manifest or skill lock schema.

Polling and explicit sync provide the baseline. Optional P2P messages only say
“fetch now”; all configuration data still travels through Git. The notification
technology is a separate choice.

### Consequences

- Git supplies history, PR review, offline work, and text merging without a new
  authoritative server.
- P2P availability is unnecessary for correctness; it only affects latency.
- File synchronization alone cannot provide the same review and merge workflow.
- Package-managed tool installations must be updated through their package
  manager; an incompatible manifest cannot silently replace the binary.

## Links

- Planning source: plan §2.1–§2.4 and §14.2–§14.3; see the
  [source and revision notes](README.md#planning-sources-and-revisions).
