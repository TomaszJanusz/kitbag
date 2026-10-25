# 10. Version and authenticate the local API independently

- Status: Accepted for KIT-29
- Date: 2026-10-25

## Context and Problem Statement

ADR 5 requires independently shipped CLI, TUI and desktop clients to share
synchronization behavior. KIT-29 introduces their transport before KIT-30 adds
the daemon. Go package compatibility alone cannot protect an older running
daemon from a newer client binary.

## Decision Drivers

- One core workflow with independently versioned clients.
- Owner-only local access on Linux and macOS, with Windows isolated behind the
  platform contract.
- Reviewable, executable wire documentation and bounded notification delivery.

## Decision Outcome

Use newline-delimited JSON-RPC 2.0 over a platform-owned Unix listener. Require
an owned real 0700 directory, a 0600 socket, same-effective-UID kernel peer
credentials on both ends, and a random 0600 state-directory token on every
request. Per-request authentication avoids a second connection-authentication
state machine. Windows returns unsupported until named pipes are implemented.

Publish method inputs/results, errors and lifecycle notifications as JSON
Schema. Generate the Markdown reference and real-socket contract cases from it;
check their freshness in the existing quality gate. Keep policy in core and
configuration validation in the existing config package.

Negotiate major.minor per request, advertise a supported range with version.get,
and report the selected minor in each result. A newer v1 client can request an
older supported minor; optional unknown fields are ignored and unavailable
methods return method-not-found. Breaking meanings require a new major.

Treat notifications as hints: fixed-size queues drop newest on overflow without
blocking publishers. Clients refresh snapshots after sync or reconnecting.
Progress reports coarse workflow phases, without changing apply semantics.

## Consequences

- The API serves core operations without becoming a daemon implementation.
- Ordinary status/plan use the daemon when available and otherwise execute
  locally. Authentication, permission and protocol failures fail closed.
- Verbose inspection resolves locally to preserve backend diagnostics. Other
  CLI mutations retain their existing execution path.
- Errors expose stable classifications, without raw causes or request bodies.
- No replay or exactly-once notification guarantee is offered. Crash-stale
  socket recovery and drift/update detection remain daemon responsibilities.

## Links

- [Wire contract and transport](../api/local-api.md).
- Plan §3, §17.4 and §18.2; KIT-29.
