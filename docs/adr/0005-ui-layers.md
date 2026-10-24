# 5. Layer the UI over shared view models and a local API

- Status: Accepted
- Date: 2026-10-24

## Context and Problem Statement

kitbag must work in terminals and on headless machines while eventually offering
accessible desktop status and settings. Maintaining separate native clients per
OS duplicates work; an always-resident webview adds runtime and system dependency
costs disproportionate to a small tray application.

## Decision Drivers

- A useful CLI and SSH-friendly TUI first.
- Low idle memory and dependency cost for the tray.
- Shared behavior and validation across interfaces.
- A replaceable desktop renderer with no changes to the core.

## Considered Options

- CLI/TUI, menu-only tray, then one lightweight non-webview window.
- A desktop web UI wrapped in Wails or another webview.
- Separate native clients for macOS, Windows, and Linux.

## Decision Outcome

Build CLI and TUI first, using Bubble Tea, huh, and Lip Gloss for the terminal
interface. Add the menu-only tray in M5, with sync status and actions such as
Sync now, Open repo, Show plan, Pause, Quit, and access to the TUI. Follow it with
a lightweight settings/status window after the M0 toolkit benchmark.

Share Go view models in `internal/ui/model` for status, settings, drift, catalog,
and publication, fed by the daemon's versioned local API. The TUI and window are
renderers of those models; validation, behavior, and text belong in the shared
layer. The tray/window communicates through the local API, keeping GUI and CGO
dependencies outside the CLI and daemon. Create the window lazily and destroy it
when closed so idle residency stays focused on the tray.

The window must not use a webview. Fyne and Gio are benchmark candidates; Fyne is
a provisional candidate, not an accepted toolkit selection. Measure startup and
memory on macOS and Linux before deciding. Planning memory figures are estimates,
not verified budgets. A separate optional headless web UI may share view models,
but does not determine the desktop renderer.

### Consequences

- Models can be tested without a terminal or GUI renderer.
- Desktop builds may need native dependencies even though the core does not.
- API compatibility matters across independently running components.
- This replaces the earlier desktop webview direction and separate per-OS
  native-client plan. It does not settle the GUI toolkit.

## Links

- Planning source: plan §2.6, §3, §17, and the superseding §18.2; see the
  [source and revision notes](README.md#planning-sources-and-revisions).
