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

## Times

Times are shown in the machine's local zone as `2006-01-02 15:04:05 MST`; the
Status tab's last sync adds a relative hint such as `(3 min ago)` and the
Activity tab shows `15:04:05` local time of day. The view models format them
(`internal/timefmt` with a clock injected by `cmd/kitbag`), so renderers never
touch zones. The API, state files and JSON keep UTC RFC 3339.

## Tabs

| Tab | Shows | Keys |
| --- | --- | --- |
| 1 Status | Headline (in sync, drift, conflicts, syncing with the current phase, another sync running, tool too old with required and running version), last known status when a reload fails, update available, result of the last sync started here, repository, branch and upstream, working copy, last sync, target counts, connection, warnings | `s` sync now, `r` refresh |
| 2 Drift | Targets other than in sync with their classification; Enter opens the selected target's planned actions (source, backup path, warnings) | `↑`/`↓`, `k`/`j`, Enter, Esc |
| 3 Settings | Common settings with value, source, override note, default and plain-language description; sensitive values are redacted and never prefilled | `↑`/`↓`, Enter edit/save, Esc cancel, `←`/`→`/Space choose, `a` Advanced, `w` save a kept draft, `u` remove from `config.toml` |
| 4 Activity | The bounded activity log (256 entries; dropped entries are counted): daemon hints, sync phases and setting writes | `f` follow, `↑`/`↓`, PgUp/PgDn |

`Tab`, `Shift-Tab` and `1`–`4` switch tabs, `?` shows the key help, `q` or
`Ctrl-C` quits. While a setting is being edited, typed keys go to the editor;
`Ctrl-C` still quits. `←`/`→`/Space cycle picker options; letters cannot change an enum or toggle. Every
keystroke is validated with the registry rules: an invalid draft shows its
constraint and is never saved; a failed save keeps the editor open with its draft and the reason. The editor closes only after a successful save (or Enter on an unchanged value).

## Settings controls

The main list contains `host`, `repo.url`, `repo.path`, `git.identity.name/email`,
`sync.poll_interval`, `daemon.update_check` and `sync.mode`. Press `a` to expand
or collapse **Advanced**: `repo.branch`, `git.backend`, `daemon.debounce` and
`ui.color`. Advanced starts collapsed and keeps its state across refreshes.
The footer and `?` help include the shortcut.

- **Choices:** `sync.mode` offers link/copy; `git.backend` offers
  auto/system/embedded; `ui.color` offers auto/always/never.
- **Toggle:** `daemon.update_check` offers On/Off.
- **Durations:** polling offers 30s, 1m, 5m, 15m and 1h; debounce offers
  100ms, 250ms, 500ms, 1s, 5s, 30s and 1m. Equivalent durations match the
  same preset (60s selects 1m). Choose **Custom…** to type another value.
  The registry still requires polling from 1s to 24h and debounce from 100ms
  to 1m. Left/Right return to the presets; Space enters a space in custom text.
- **Host:** choices come from regular `hosts/*.md` files in the effective
  working copy; symlinks are excluded. **Other…** accepts a validated name.
  An existing name without a matching file opens Other without replacing it.
- **Text:** repository URL/path/branch and Git identity remain line editors.
  Identity details show the effective Git config value as a hint when readable,
  or say it is unavailable. These hints never prefill an input. The existing
  identity name/email pair rule still applies; use `kitbag config edit` to set
  both together. URL and stored identity values remain masked.

`schema` and `daemon.release_url` are internal settings. `ui.notifications` and
`ui.theme` currently have no effect and are hidden. Colour has a real CLI/TUI
consumer, so `ui.color` stays in Advanced. `migration.legacy_roots` appears only
when configured or when `~/src/agent-config` or `~/.local/bin/agent-config-sync`
exists. Its detail points to the CLI for editing the TOML path list; the TUI
does not guess or add trusted migration roots. Detection is deliberately limited
to these conventional locations and does not scan agent contents.

Every key remains available through `kitbag config list/get/set/unset/edit`.
Saving writes configuration; restart the daemon for polling/debounce/update
changes, and reopen the TUI for its startup colour preference.

## Mouse

The mouse is optional; every action keeps its key. The terminal reports clicks
and the wheel, and the renderer turns them into the same intents as the keys:

- Click a tab title to switch to it (the digits in the narrow tab bar too).
- Click a row on the Drift tab to select it; click the selected row again to
  open its details, like Enter. Click a row on the Settings tab to select it;
  editing stays on Enter, so a stray click never starts a change.
- The wheel moves the Drift selection and scrolls the Activity log one line per
  notch, like `↑`/`↓`. Scrolling back to the end resumes following.
- While a setting is edited, click an option to change the draft, including
  Custom/Other. Enter still saves; Esc cancels. Clicks outside the options
  are ignored. In help and open drift details the mouse is ignored. Other
  buttons, releases and drags do nothing. Text selection and copying need the terminal's
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
settings pickers/toggles, custom validation, failed saves, Advanced visibility,
secret masking, the bounded log, help, mouse clicks and wheel
(with a negative control for each stray event), resize, `NO_COLOR` output
and subscription shutdown. Golden screens in `internal/ui/tui/testdata` are
ANSI-stripped at fixed sizes; after reviewing a change, rewrite them with
`go test ./internal/ui/tui -args -update-screens`. A testscript covers the
non-terminal refusal.

## Remote check freshness

The Status tab keeps separate `Last check`, `Next check` and `Last sync` rows.
A check looked at the remote; a sync applied changes. The help overlay repeats
this distinction. `poll.finished` refreshes the status snapshot even when
nothing changed. Check failure uses the shared error text catalog and appears
as a warning with its age. While the daemon is running, an age greater than
three poll intervals warns “daemon is not checking”. A lightweight clock tick
updates ages and warnings even when no daemon events arrive. With no daemon,
the persisted check says “stale; daemon not running” and no next check is shown.

Status shows **daemon X, binary Y** and warns about version mismatch or a stale
installed service definition. Press **h** on Status to request repair, then **y**
to confirm or **n/Esc** to cancel. The shared view model owns the warning,
confirmation and refresh intent; the terminal renderer draws them and forwards
keys. Refresh runs locally through the service core rather than an RPC inside
the daemon being restarted, so older API v1 daemons can be repaired too. Matching
or uninstalled services have no repair action. No repair runs before confirmation;
ordinary **r** only reloads views. A failed repair remains visible and retryable.
