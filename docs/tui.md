# Terminal interface (`kitbag tui`)

`kitbag tui` is a full-screen terminal interface for status, drift, settings and
activity. It renders the [shared view models](ui.md) and owns no behavior of its
own: keys become model intents, every text comes from the catalog in
`internal/ui/model/text`, and validation stays in the settings registry
([ADR 5](adr/0005-ui-layers.md)). The renderer is `internal/ui/tui` (Bubble Tea,
Lip Gloss and the Bubbles line editor); the `terminal-renderer` depguard rule
keeps these libraries out of every other package.

## Visual design v2

The home screen is a quiet starting point: a compact pixel wordmark, one status
headline and one menu. Repository identity, version and help sit on the edges.
Details appear only after opening a screen or dialog. Space is intentional;
there are no dashboard cards or persistent tab bar. This section is the design
contract for the staged implementation below.

### Principles and palette

- One warm accent identifies the primary action and selection. Normal status is
  muted grey; existing warning and failure semantics remain explicit in words.
- Buttons have a filled rectangular surface (primary) or muted outline
  (secondary), a visible focus marker, and an explicit unavailable label.
- Dialog titles align left, a small `esc` hint aligns right. Body text is muted;
  buttons align right at the bottom. Colour panels have no heavy frame.
- Keyboard and mouse use the same geometry and model intents. Resizing preserves
  selection, drafts and dialog focus. Nothing can draw beyond the viewport.
- All labels, hints and state summaries come from the catalog and shared view
  models. The renderer owns layout, focus, scrolling and decorative logo pixels.

| Role | Truecolor | 256 colours | 16 colours | NO_COLOR / monochrome |
| --- | --- | --- | --- | --- |
| Screen | `#181818` | 234 | black (0) | terminal background |
| Dialog panel | `#242424` | 235 | black (0) | ASCII frame `+--+`, vertical sides |
| Text | `#dedbd4` | 253 | white (7) | plain text |
| Muted | `#99958e` | 246 | bright black (8) | plain text, positional hierarchy |
| Accent / primary | `#e9b872` | 179 | yellow (3) | `[ Label ]` |
| Logo side | `#8e6338` | 95 | bright black (8) | omitted |
| Logo lower edge | `#50391f` | 58 | bright black (8) | omitted |
| Primary text | `#181818` | 234 | black (0) | plain text |
| Warning | `#e9b872` | 179 | yellow (3) | explicit warning text |
| Failure | terminal red | red (1) | red (1) | explicit error text |
| Focus | `> ` + emphasis | same | same | `> [ Label ]` |
| Disabled | muted + unavailable label | same | same | `[ Label (unavailable) ]` |

Profiles are selected from TERM/COLORTERM without terminal queries. NO_COLOR
and ui.color=never suppress **all** ANSI styling and use the plain wordmark.
ANSI fallback colours use the terminal palette; the two dark surfaces can be
identical on 16-colour terminals, so spacing and focus markers still carry the
hierarchy. No capability depends on colour alone.

### Wordmark and extrusion

The colour wordmark is a seven-subpixel-high pixel mask. Each terminal cell
contains two vertical subpixels, drawn with `▀`, `▄` and `█`; mixed cells use
foreground and background colours to preserve both shades. The accent front
face takes precedence over every shadow pixel. Sweeping the same mask three
subpixels right and down forms joined side faces; exposed lower shadow edges
receive the darkest shade, suggesting light from the top left.

The complete footprint is 28 columns by five terminal rows, including the
extrusion. It occupies the original five-row logo area, so the status and menu
do not move. The pure mask-to-cells function clips every layer to its rectangle.
Truecolor uses the two dark shades above; 256-colour mode uses their nearest
palette entries and 16-colour mode uses one dark shade for both. NO_COLOR and
ui.color=never use catalog text `kitbag`, without ANSI. Below 80 columns or 24
rows the logo is also plain text; any viewport narrower than the mask plus its
extrusion must use that fallback. Resizing never adds a shadow beyond the edge.

The shade-index golden below shows one line per **subpixel** row (two per
terminal row): `█` front, `▓` side, `▒` lower edge, space background. Raw-ANSI
and stripped home goldens cover 120×35, 80×24, 60×20 and 40×10 in truecolor,
256 and 16 colours, alongside NO_COLOR goldens.

```text
█  █ █  █  █
█▓█ ▒ ▒ █▓ █▓
██▓▓ █ ███▓███   ███  ███
█▓█▓▒█▓ █▓▓█▓▓█ █ ▒█▓█ ▓█▓
█▓█▓ █▓▓█▓▓█▓▓█▓█▓ █▓▓███▓▓
█▓▓█▓█▓▓█▓▓█▓▓█▓█▓▓█▓▓▓▓█▓▓▓
█▓▓█▓█▓▓██▓███▓▒▓███▓███▓▓▓▓
 ▒▓▓▒▓▓▓▓▒▓▓▒▓▓ ▒▓▒▓▓▓▓▓▓▓▒▓
  ▒▓ ▒▓▒▓ ▒▓ ▒▓▓ ▒ ▒▓▓▓▒▓▓ ▒
   ▒  ▒ ▒  ▒▒ ▒▒▒   ▒▒▒ ▒▒▒
```

### Home mockups

These ASCII layouts show placement in the stated viewport (blank space is
abbreviated above and below the centered content). The implementation uses
block pixels for the wordmark with its darker extrusion when colour is available
and the viewport has space; ASCII/plain mode uses `kitbag`. Values are examples, never renderer text.
The status line is focusable above the menu; its Enter action opens Status.

120×35:

```text

                                                 kitbag [pixel wordmark]

                                    Everything is in sync · checked 2 min ago

                                         +------------------------------------+
                                         | > Sync now                       s |
                                         |   Agents                 4       a |
                                         |   Drift                          d |
                                         |   Settings                       c |
                                         |   Activity                       l |
                                         +------------------------------------+

 /repo/kitbag · main                                  ? help · q quit                                      kitbag 0.1.2
```

80×24:

```text

                             kitbag [pixel wordmark]

                  Everything is in sync · checked 2 min ago

                     +------------------------------------+
                     | > Sync now                       s |
                     |   Agents                 4       a |
                     |   Drift                          d |
                     |   Settings                       c |
                     |   Activity                       l |
                     +------------------------------------+

 /repo/kitbag · main              ? help · q quit                     kitbag 0.1.2
```

60×20 (plain logo below the pixel breakpoint):

```text

                           kitbag

        Everything is in sync · checked 2 min ago

           +------------------------------------+
           | > Sync now                       s |
           |   Agents                 4       a |
           |   Drift                          d |
           |   Settings                       c |
           |   Activity                       l |
           +------------------------------------+

 /repo · main         ? help · q quit          kitbag 0.1.2
```

At 40×10 the plain logo, one truncated headline and five menu rows fit; the
secondary status line and decorative menu padding disappear. Below 40×10 only
the catalog's enlarge-terminal notice appears. Edge fields truncate independently
and never overwrite help. A problem replaces the normal headline; at most one
secondary line shows stale-check/service/version warnings or the latest outcome.
Status retains the complete repository, branch/upstream, working copy, last
sync/check, next check, targets, connection and warning details.

### Dialog presets

The following ASCII frames illustrate the monochrome fallback. In colour, a
slightly lighter panel replaces the frame, with the same padding and geometry.
`[ Yes ]`, `[ Next ]`, `[ Finish ]` and `[ OK ]` are filled accent blocks;
No/Back are secondary. The focused control also carries `> `.

Confirmation (Yes/No, No initially focused):

```text
+------------------------------------------------------+
| Remove stored setting?                           esc |
|                                                      |
| Remove host from config.toml? The effective value     |
| will be resolved again using the normal precedence.  |
|                                                      |
|                                 [ Yes ]   > [ No ]   |
+------------------------------------------------------+
```

Form (Back/Next; final step uses Finish):

```text
+------------------------------------------------------+
| Add agent                                        esc |
| Step 2/5                                             |
|                                                      |
| Directory: > ~/.agent                                |
| Choose a directory for this instance.                |
|                                                      |
|                                  [ Back ] [ Next ]   |
+------------------------------------------------------+
```

Scrollable information (Help, Status or target details):

```text
+------------------------------------------------------+
| Keyboard                                         esc |
|                                                      |
| Up/Down or k/j: move the selection                    |
| Enter: open the selected item                        |
| ...                                                  |
|                                                      |
| scroll ↑/↓                              > [ OK ]     |
+------------------------------------------------------+
```

Long confirm messages and forms use the same scroll hint and fixed button row.
Standard panels cap at 72 columns (wide Agent previews retain their 104-column
request, clamped to the terminal minus four columns). At 80×24 a standard panel
has 72 columns; at 60×20 it has 56 columns; at the minimum
40×10 it has 36 columns. The body scrolls, long labels truncate, and focus brings
a field into view. Many buttons wrap into right-aligned rows; the focused row
stays visible. Esc and outside click cancel only the top dialog. Scroll wheel,
PgUp/PgDn and Ctrl-Up/Ctrl-Down scroll the body; Up/Down also scroll read-only
bodies, while Tab/Shift-Tab traverse enabled controls. No disabled button acts.

### Navigation and key map

Home opens Screens, each with a small top-edge breadcrumb `kitbag › Settings`.
Esc or q returns home; Esc first closes a dialog, editor or target detail.
Ctrl-C quits everywhere; q quits on home. A dialog traps input until dismissed.
Settings retains responsive master/detail panes, Drift shows list/detail and
Activity retains its bounded scroll/follow log. The merged Agents screen uses
the same master/detail panes and fixed bottom action buttons; its wizard stays
in the shared dialog stack.

| Context | Keys / mouse | Result |
| --- | --- | --- |
| Home | ↑/↓, k/j, Tab/Shift-Tab | Select status or menu row |
| Home | Enter or click row | Open selected screen; Sync now starts sync |
| Home | s / a / d / c / l | Sync / Agents / Drift / Settings / Activity |
| Anywhere outside an editor/dialog | i / 1 | Status details |
| Anywhere outside an editor/dialog | 2 / 3 / 4 / 5 | Drift / Settings / Activity / Agents |
| Screen | Esc / q; click breadcrumb | Return home |
| Everywhere outside an editor/dialog | r / s / ? | Refresh / sync / help |
| Settings | a / Enter / w / u | Advanced / edit / save draft / confirm unset |
| Editor | Enter / Esc; ←/→/Space or option click | Save / cancel / choose |
| Activity | f / ↑/↓ / PgUp/PgDn; wheel | Follow / scroll |
| Dialog | Tab/Shift-Tab, arrows; click button | Focus / activate enabled button |
| Dialog | Enter / Esc; outside click | Activate / cancel top dialog |
| Scrollable dialog | wheel / PgUp/PgDn / Ctrl-↑/↓ | Scroll body |
| Home / anywhere | q / Ctrl-C | Quit / quit |

Delivery is three focused PRs: (1) this design and isolated buttons/dialogs,
(2) home and screen navigation, (3) post-merge Agents navigation/style migration.
No model-side dialog contract from PR #85 changes.

## Connection

Like `kitbag status`, the interface uses the [local API](api/README.md) when the
daemon runs and reads this machine in-process when none listens. The Status
screen's Connection row says which one answered:

- **Daemon, live updates**: the event subscription is open; the Activity screen
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
Status screen's last sync adds a relative hint such as `(3 min ago)` and the
Activity screen shows `15:04:05` local time of day. The view models format them
(`internal/timefmt` with a clock injected by `cmd/kitbag`), so renderers never
touch zones. The API, state files and JSON keep UTC RFC 3339.

## Screens

Home shows the wordmark, a compact status summary and one menu: Sync now,
Agents, Drift (with its nonzero count), Settings and Activity. Repository path
and branch sit at the lower left, version at the lower right, and help/quit
at the bottom centre. Up/Down, k/j and Tab/Shift-Tab move focus between the
status line and menu; Enter activates it, or click directly. Shortcuts on home
are s/a/d/c/l. Sync stays on home and respects the model's unavailable state.

Screens have a small `kitbag › Settings` breadcrumb instead of a tab bar.
Click the breadcrumb or press Esc/q to return home. Esc first leaves a setting
editor or target detail; Ctrl-C quits everywhere. The deep keys remain
1 Status, 2 Drift, 3 Settings, 4 Activity and 5 Agents; i opens Status.
Tab/Shift-Tab also retain their screen-cycling behavior outside home/dialogs.
The merged Agents renderer uses the same screen navigation and button row.

| Screen | Shows | Keys |
| --- | --- | --- |
| Status | Full repository/branch/upstream, working copy, last sync/check and next check, target counts, connection, service/version warnings and actions | s sync, r refresh, h repair, arrows/page keys or wheel scroll |
| Drift | Target list and selected target details side by side on wide screens; Enter opens details on any size | arrows/k/j, Enter or Details button, Esc back |
| Settings | Common and Advanced settings in responsive master/detail panes, registry validation and redaction | arrows/k/j, Enter edit/save, Esc cancel, a Advanced, w save, u confirm removal |
| Activity | Bounded log, dropped count and follow state | f or Follow/pause button, arrows/page keys or wheel |

While editing, typed keys go to the editor; Ctrl-C still quits.
Left/Right/Space cycle picker options; letters cannot change an enum or toggle.
An invalid draft is never saved; failed saves retain the editor and its draft.
Save/Remove/Revert buttons emit the same intents as their keys, and Remove
always asks for confirmation. At short sizes unavailable screen actions are
omitted to reserve room for the editor; dialog disabled controls stay explicit.

Help opens a scrollable dialog with the title left, Esc right and a primary
OK button. An open dialog clears the visible background while preserving the
underlying screen state, so clipped background text does not compete with it.
Page keys, control arrows and the wheel scroll the body; arrows also scroll
read-only bodies. Tab/Shift-Tab traverse enabled fields/buttons. Esc/outside
click cancels only the top window. Dialogs consume all other screen shortcuts.

## Layout and windows

Settings uses a master/detail layout. At **100 columns and wider**, the list
occupies about three fifths of the screen, with the selected field's description,
effective value, source and default in the right pane. A dedicated **Help**
section in that pane shows field hints and editor keys. An editor that fits is
rendered **in the selected row beside the field name**; longer controls use the
right pane. Below 100 columns the same panes stack vertically and the list
scrolls to keep the selection visible. Very short screens shorten the details
to reserve room for the editor and help. Below 40×10 the existing size notice
replaces the content, preserving open windows and drafts until the next resize.

Modal windows are centered over a cleared background, with a left title,
right Esc hint, muted scrollable body and bottom button row. Colour uses a
slightly lighter panel; NO_COLOR uses an ASCII frame. Only
the top window receives input. `Tab`/`Shift-Tab`, `↑`/`↓` and arrows on buttons
cycle through enabled fields and buttons. Left/Right within a text field move
the cursor. `Enter` activates the focused button, or Next/Finish from a field;
a disabled Next/Finish never activates Back. `Esc` and a primary click outside
cancel only the top window. Clicking a field focuses it; clicking a button
activates it. Wheel events inside the panel scroll its body. Releases and drags are ignored.
Global screen, refresh, sync, help
and quit keys stay trapped; `Ctrl-C` still exits the application. Text markers
and borders carry focus and selection with `NO_COLOR` too.

Removing a stored setting with `u` opens a Yes/No confirmation, initially on
**No**. Yes sends the existing unset intent; No, Esc and outside clicks leave
configuration untouched. Unset remains unavailable for values supplied by flags,
environment or defaults. Saving, reverting drafts and sync keep their existing
behavior; this adds no new persistence semantics.

### Component API for future tabs

The shared contract is in `internal/ui/model/dialog.go`, with all labels in
`internal/ui/model/text`:

- `DialogView` carries a stable ID, title/message, fields, body lines, options,
  enabled `Control` buttons, cancel intent, optional step/step-count, initial
  focus index and a `Wide` request (up to 104 columns instead of 72).
  Focus indexes count fields, then options, then buttons. `DialogLine` has a
  `Tone` (normal, heading, hint, success, warning, problem, code); warnings,
  problems and successes carry `!`, `✗` and `✓` markers so they survive
  `NO_COLOR`, and code lines (diffs, commands) are truncated, not wrapped.
  `DialogOption` carries a label, detail, `Selected` (drawn `(*)`) and the intent
  a choice dispatches.
  `DialogField` carries a stable field ID, draft, label/hint, enabled state and
  sensitivity. Supply only editable drafts, never a stored secret.
- `ConfirmDialog(id, title, message, confirm, cancel)` supplies Yes/No and
  closes on an action. `FormDialog(id, title, step, steps, fields, back, next,
  cancel)` supplies **Step 2/5**, Back and Next (Finish on the last step).
  A form stays open after a button intent while its model validates or loads
  a preview. Disabled controls are skipped by keyboard and mouse.
- The owning model receives `EditDialogField{Dialog, Field, Value}` on each
  changed draft and the supplied button/cancel intents. It owns validation,
  wizard steps, dirty-close policy, asynchronous work and preview revisions.
  Reproject its view after handling an intent; the renderer never advances a
  wizard or writes a field itself.
- `tui.Model.ShowDialog(view)` (or a `DialogView` message on the Bubble Tea
  loop) opens/replaces a projection by ID; `HideDialog(id)` removes a
  model-closed window. Call these only on the renderer loop. Replacements
  preserve focus and the text cursor of unchanged drafts. Distinct IDs stack;
  closing the top restores the window beneath it. A replacement keeps the
  focus only if the user moved it on the same step; a new step, or a step the
  user has not touched, starts at its initial focus. A disabled initial button
  falls back to the one before it (Back), never around to Cancel. PgUp/PgDn and
  the wheel scroll a window whose content does not fit while a button has the
  focus; a focused field or option keeps itself visible.
- `WindowStack` implements Push/Replace/Pop/Remove/Len and Key-to-intent
  translation for renderer composition. `MasterDetail(width, height,
  masterWidth, stackedRows)` returns `PaneLayout` with cell `Rect`s for drawing
  and hit testing. A zero master width uses the proportion; a nonzero width
  requests a fixed list width, clamped to leave a usable detail pane. Call it
  after the minimum-size guard. No toolkit types enter shared models.

The Agents screen does exactly this: `AgentForm.Dialog()` projects the wizard into
`FormDialog` under one ID at each step, and `AgentForm.DiscardDialog()` stacks
the discard confirmation above it under another.

## Agents screen and wizard

The Agents screen renders the shared `Agents` and `AgentForm` models
([agents and instances](ui.md#agents-and-instances)); the list and the wizard
read and write through the same API methods as `kitbag agent list/add/edit/remove`,
with the daemon's dry-run preview and revision check. Open it from the home menu (`a`) or key `5`;
Activity retains key `4`.

**List.** The left pane groups rows by agent kind. Each kind shows its declared
instances (`target/name`, or the implicit default directory), then a
**[found, not declared]** section with directories found from the environment,
the documented default or the name probe. Each row has a second line with the
directory state, where it comes from, the host scope and whether it is active on
this host. `p` (or the action button) runs the explicit HOME name probe, which
can add more found directories. The right pane describes the selected row:
directory, source (where it is declared), directory state, hosts, whether it is
active here, the settings merge, and what the **next sync** does with it
(installs, skips on this host, warns and skips a missing directory, or leaves an
undeclared or outside-HOME directory alone). Actions live in the fixed bottom
button row: Add is the primary warm filled button; Edit/Remove/Probe are muted
secondary buttons. They use the same intents
as a/e/d/p. The detail keeps its key hint and warnings. Below 100 columns the
panes stack; short screens reserve space for the selected row and its states,
with actions still at the bottom. Only the declared instances of a
target can be edited or removed; the implicit default and found directories offer
Add, which a found directory prefills with its kind and directory.

**Wizard.** `a`, `e` (or Enter, or a second click on the selected row) and `d`
open the model's form as a wide modal window titled with the operation and the
step, for example **Add an agent instance: Instance name**, with **Step n/N**:

- add: agent, instance name, directory, machines (schema 2 only), settings
  merge, preview, confirm, result;
- edit: directory (empty keeps the current one, which the field's hint shows),
  machines (schema 2 only), settings merge, preview, confirm, result;
- remove: preview, confirm, result.

Text steps validate every keystroke with the model's catalog messages (name
grammar and taken names, `~/` or absolute directories, outside HOME,
directories another instance declares, `generic` refused); an invalid step keeps
**Next** unavailable. Choice steps (agent, settings, and the directory step's
**Directories found on this machine**) are option lists marked `(*)`: Space,
Enter or a click chooses, `←`/`→` step to the previous/next choice, and Enter on
an option moves the focus to Next, so Enter, Enter accepts a choice and goes on.
The preview step shows the commit title, how it will be published (directly to
the branch, or for `pr` publish mode that nothing is written and the commands to
propose it follow in the result), a directory that is handed back, warnings and
the `kitbag.toml` diff; PgUp/PgDn or the wheel scroll a long preview. A refused
preview shows the server's detail and focuses Back. The confirm step starts on
**Back**, so a stray Enter never applies; **Apply** must be chosen. While the
daemon applies, the window says so, every button is unavailable and Esc or an
outside click is refused. A preview made stale by another change to
`kitbag.toml` is not applied: the window returns to the preview, says it changed
and shows the new one. The result says published (with the short commit),
committed but not published (with the reason and the `git pull --rebase` and
`git push` commands), not written in `pr` mode (with the commands to propose it)
or not applied; it reports handed-back outputs and the sync that ran, and notes
that the list was reloaded. Close (or Cancel) returns to the refreshed list.

**Machines.** On a schema-2 manifest the **Machines** step offers **This
machine only (host)** (the default for add), **All machines**, **Choose
machines…** (a comma-separated text entry; the option names the machines
`kitbag.toml` declares) and, when editing, **Keep the current machines**.
Invalid or repeated names keep Next unavailable; unknown names warn. On a
schema-1 manifest the step is absent, the selected instance's Hosts row says
choosing machines needs manifest schema 2, and the button row adds **Upgrade
the manifest**. It opens a window that previews the upgrade (conversions, the
warning that every machine must run the release that reads schema 2, the
checked machines, commit, route and diff) with focus on **Cancel**; **Upgrade
and publish** applies exactly that preview, and the result reloads the list.

Cancel, Esc and an outside click close the wizard at once while nothing was
chosen or typed; otherwise a **Discard this change?** window asks first,
initially on **No**. No, Esc or an outside click returns to the draft; Yes
discards it. Nothing is written before Apply.

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

- Click a home menu row or status line to open it, or click a screen breadcrumb
  to return home. Click action buttons to use the same intent as the key.
- Click a Drift row to select it; click it again for details, like Enter.
  Settings rows only select; Enter starts editing.
- The wheel moves Drift selection and scrolls Activity/Status. On home it moves
  menu focus. Dialog wheels scroll only inside the panel.
- While editing a setting, click a choice or a bottom action button. Outside
  option/button clicks are ignored. Releases, drags and other buttons never
  act. Text selection/copying uses the terminal override (usually Shift-drag).

The renderer asks for cell-motion reporting with SGR coordinates and releases
both when the program ends: on quit, `Ctrl-C`, `SIGINT`/`SIGTERM` and recovered
panics, like the alternate screen. The `?` help describes mouse actions; the footer hints are clickable.

## Terminal behavior

- **Interactive only.** stdin and stdout must be terminals; otherwise the command
  exits 2 with `kitbag tui needs an interactive terminal on stdin and stdout`.
  Use `kitbag status`, `plan` and `config list` in scripts.
- **Colour.** `NO_COLOR` (any value, even empty) and `ui.color = "never"` disable
  colour and all other styling. No state depends on styling: buttons are
  bracketed, the selection is marked `>`, a pending draft `*` and an unavailable
  action says so. The profile comes from `TERM`/`COLORTERM`; the
  terminal is never queried, which keeps SSH sessions free of stray replies.
- **Size.** The layout follows resizes. Below 40×10 the screen asks for a larger
  terminal; narrow terminals wrap values, stack panes and use a plain logo.
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
scripted `model.Client`: home keyboard/mouse navigation, screen flows, sync
progress, drift detail,
settings pickers/toggles, custom validation, failed saves, Advanced visibility,
secret masking, the bounded log, help, mouse clicks and wheel
(with a negative control for each stray event), resize, `NO_COLOR` output
and subscription shutdown. Golden screens in `internal/ui/tui/testdata` are
ANSI-stripped at fixed sizes; after reviewing a change, rewrite them with
`go test ./internal/ui/tui -args -update-screens`. A testscript covers the
non-terminal refusal.

Pane and window flows additionally cover the 99/100-column breakpoint, inline
steppers, side-panel help, selection hit testing, modal focus trapping, disabled
Next, nested windows, outside cancellation, dialog buttons/fields, secret
masking and resizes with a draft open. Settings and dialog golden screens cover
120×35, 100×30, 60×20 and 40×10, plus the below-minimum notice. Tests use scripted
clients and do not run KitBag against a real home directory.


## Remote check freshness

The Status screen keeps separate `Last check`, `Next check` and `Last sync` rows.
A check looked at the remote; a sync applied changes. The help overlay repeats
this distinction. `poll.finished` refreshes the status snapshot even when
nothing changed. Check failure uses the shared error text catalog and appears
as a warning with its age. While the daemon is running, an age greater than
three poll intervals warns “daemon is not checking”. A lightweight clock tick
updates ages and warnings even when no daemon events arrive. With no daemon,
the persisted check says “stale; daemon not running” and no next check is shown.

Status shows **daemon X, binary Y** and warns about version mismatch or a stale
installed service definition. Press **h** on Status to request repair: a
**Repair service** confirmation window opens, initially on **No**. Press **y**
(or activate Yes) to confirm; **n**, **Esc**, No or an outside click cancels.
The shared view model owns the warning, the `ConfirmDialog` projection
(`StatusView.Dialog`) and the refresh intent; the terminal renderer draws them
and forwards keys. Refresh runs locally through the service core rather than an RPC inside
the daemon being restarted, so older API v1 daemons can be repaired too. Matching
or uninstalled services have no repair action. No repair runs before confirmation;
ordinary **r** only reloads views. A failed repair remains visible and retryable.
