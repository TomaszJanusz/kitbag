# 1. Use Go as the implementation language

- Status: Accepted
- Date: 2026-10-24

## Context and Problem Statement

kitbag primarily coordinates files, Git, structured configuration, and processes.
It needs a maintainable CLI, daemon, and TUI, with straightforward distribution
across operating systems. The implementation language should support that work
without making toolchain complexity the central concern.

## Decision Drivers

- Fast implementation and readable, maintainable I/O code.
- Mature libraries for Git, terminal interfaces, services, and CLI tests.
- A portable core that builds without CGO.

## Considered Options

- Go.
- Rust.
- TypeScript with Bun.

## Decision Outcome

Choose Go. Its ecosystem fits the work: go-git, Bubble Tea, systray, service
libraries, and testscript. The CLI, daemon, and TUI remain free of CGO; GUI
components may have separate native build requirements.

Rust is a credible alternative with stronger compiler guarantees around errors,
exhaustiveness, and absence of null values. For this I/O-focused tool, those
benefits do not outweigh Go's implementation speed, readability, and simpler
cross-compilation of the core. TypeScript/Bun offers quick development but a less
suitable embedded Git and tray ecosystem and a larger packaged runtime.

### Consequences

- Strict formatting, linting, error handling, and dependency boundaries are a
  necessary complement to Go's compiler guarantees.
- Cross-compiling the core is practical; it does not prove native behavior or
  remove GUI-specific build and verification needs.
- Rust remains a documented alternative rather than a second implementation.

## Links

- Planning source: plan §2.5 and §12, with quality requirements in §13; see the
  [source and revision notes](README.md#planning-sources-and-revisions).
