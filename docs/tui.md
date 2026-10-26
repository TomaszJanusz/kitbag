# Terminal interface (`kitbag tui`)

`kitbag tui` is a full-screen terminal interface for status, drift, settings and
activity. It renders the [shared view models](ui.md) and owns no behavior of its
own: keys become model intents, every text comes from the catalog in
`internal/ui/model/text`, and validation stays in the settings registry
([ADR 5](adr/0005-ui-layers.md)). The renderer is `internal/ui/tui` (Bubble Tea,
Lip Gloss and the Bubbles line editor); the `terminal-renderer` depguard rule
keeps these libraries out of every other package.

## Connection

Like `kitbag status`, the interface uses the [local API](api/README.md) when the
daemon runs and reads this machine in-process when none listens. The Status
tab's Connection row says which one answered:

- **Daemon, live updates**: the event subscription is open; the Activity tab
  receives lifecycle hints and the views reload on `sync.finished` and
  `drift.detected`.
- **Daemon, live updates unavailable**: answers come from the daemon but the
  subscription is down; it is retried with backoff from 1s to 30s.
- **No daemon; reading this machine directly**: every load and sync runs in this
  process. There are no live hints; press `r` to reload.

Global flags apply as for other commands (`--config`, `--host`, `--mode`,
`--git-backend`). `--verbose` diagnostics are not written: stderr output would
corrupt the screen.

## Tabs

| Tab | Shows | Keys |
| --- | --- | --- |
| 1 Status | Headline (in sync, drift, conflicts, syncing with the current phase, another sync running, tool too old with required and running version), last known status when a reload fails, update available, result of the last sync started here, repository, branch and upstream, working copy, last sync, target counts, connection, warnings | `s` sync now, `r` refresh |
| 2 Drift | Targets other than in sync with their classification; Enter opens the selected target's planned actions (source, backup path, warnings) | `↑`/`↓`, `k`/`j`, Enter, Esc |
| 3 Settings | Every registry setting with effective value, source, override note, default, allowed values and the description; sensitive values are redacted and never prefilled | `↑`/`↓`, Enter edit, Enter save, Esc cancel, `w` save a kept draft, `u` remove from `config.toml` |
| 4 Activity | The bounded activity log (256 entries; dropped entries are counted): daemon hints, sync phases and setting writes | `f` follow, `↑`/`↓`, PgUp/PgDn |

`Tab`, `Shift-Tab` and `1`–`4` switch tabs, `?` shows the key help, `q` or
`Ctrl-C` quits. While a setting is being edited, typed keys go to the editor;
`Ctrl-C` still quits. `←`/`→` cycle the allowed values of a closed set. Every
keystroke is validated with the registry rules: an invalid draft shows its
constraint and is never saved; a failed save keeps the draft with the reason.

## Mouse

The mouse is optional; every action keeps its key. The terminal reports clicks
and the wheel, and the renderer turns them into the same intents as the keys:

- Click a tab title to switch to it (the digits in the narrow tab bar too).
- Click a row on the Drift tab to select it; click the selected row again to
  open its details, like Enter. Click a row on the Settings tab to select it;
  editing stays on Enter, so a stray click never starts a change.
- The wheel moves the Drift selection and scrolls the Activity log one line per
  notch, like `↑`/`↓`. Scrolling back to the end resumes following.
- While a setting is edited, in the help overlay and in an open drift detail
  the mouse is ignored, like keys that do not apply. Other buttons, releases
  and drags do nothing. Text selection and copying need the terminal's
  override (usually Shift-drag).

The renderer asks for cell-motion reporting with SGR coordinates and releases
both when the program ends: on quit, `Ctrl-C`, `SIGINT`/`SIGTERM` and recovered
panics, like the alternate screen. The footer and the `?` help mention the mouse.

## Terminal behavior

- **Interactive only.** stdin and stdout must be terminals; otherwise the command
  exits 2 with `kitbag tui needs an interactive terminal on stdin and stdout`.
  Use `kitbag status`, `plan` and `config list` in scripts.
- **Colour.** `NO_COLOR` (any value, even empty) and `ui.color = "never"` disable
  colour and all other styling. No state depends on styling: the active tab is
  bracketed, the selection is marked `>`, a pending draft `*` and an unavailable
  action says so. The profile comes from `TERM`/`COLORTERM`; the
  terminal is never queried, which keeps SSH sessions free of stray replies.
- **Size.** The layout follows resizes. Below 40×10 the screen asks for a larger
  terminal; narrow terminals wrap values and shorten the tab bar.
- **Screen.** The alternate screen is used and restored on quit, `Ctrl-C`,
  `SIGINT`/`SIGTERM` and recovered panics. The cursor does not blink, so an idle
  screen is not redrawn.
- **One operation at a time.** Loads, syncs and setting writes run in order on
  one worker, because the in-process services and the daemon's per-request
  services share the state lock; a newer queued reload replaces an older one. A
  long sync therefore delays a refresh or save until it finishes.
- **Shutdown.** Quitting cancels the worker and the event subscription and waits
  for both before the command returns.

## Limitations

- No content diffs: the API omits file contents, so drift shows classification
  and planned actions only ([API gaps](ui.md#api-gaps)).
- Update notices carry no version; tool-too-old through the daemon has no
  required version (the in-process fallback has it).
- Problems show the catalog's classified message; core's diagnostic detail is not
  displayed. Use the CLI command for the full error.
- Settings writes through the daemon have no compare-and-swap.
- Skills and catalog tabs follow with the skill catalog (KIT-52, KIT-46).
- Windows is refused before any command runs, like every other command.

## Tests

`internal/ui/tui` tests drive the program with
[teatest](https://github.com/charmbracelet/x/tree/main/exp/teatest) against a
scripted `model.Client`: each tab's main flow, sync progress, drift detail,
settings validation and save, the bounded log, help, mouse clicks and wheel
(with a negative control for each stray event), resize, `NO_COLOR` output
and subscription shutdown. Golden screens in `internal/ui/tui/testdata` are
ANSI-stripped at fixed sizes; after reviewing a change, rewrite them with
`go test ./internal/ui/tui -args -update-screens`. A testscript covers the
non-terminal refusal.
