# Shared view models

`internal/ui/model` holds the models that the terminal UI and the later
settings window both render ([ADR 5](adr/0005-ui-layers.md)). The terminal
renderer is `internal/ui/tui` ([`kitbag tui`](tui.md)). Behavior,
validation and text live there once; a renderer only draws views and forwards
user input. The package is pure Go: no UI toolkit, no `os`, `net` or process
access, no `internal/api`, `internal/core` or `internal/platform` import. The
`ui-model` depguard rule in `.golangci.yml` enforces this.

## State machines

Each model is a plain state machine. `Update(Msg) []Cmd` applies a user intent
(`Refresh`, `SyncNow`, `EditSetting`, `SaveSetting`, ...) or a command result
(`StatusLoaded`, `SyncFinished`, `EventReceived`, ...) and returns commands
(`LoadStatus`, `RunSync`, `WriteSetting`, `Subscribe`, ...). `View()` returns a
plain struct for rendering. Models start no goroutines, read no clock and are
not safe for concurrent use: one renderer loop owns them.

`Runner.Run(ctx, cmd, emit)` executes a command against a `model.Client` and
emits its result (sync also emits each progress phase first). A renderer runs it
on its own worker and feeds the emitted messages back into its loop. `App`
composes the models, routes every message to all of them, loads every snapshot
on start and resubscribes after a lost event stream with backoff from 1s to 30s.
A canceled subscription (renderer shutdown) is not retried.

| Model | States and rules |
| --- | --- |
| `Status` | Loading, ready, failed. Shows Git state, last sync, drift counts, warnings, tool-too-old, update available, daemon/live connection and the result of a sync started here. Only one sync runs from here; sync is disabled while another sync runs or a newer tool is required. |
| `Drift` | Targets other than in sync with their planned actions; the selection follows its path across reloads. |
| `Settings` | A curated projection of the registry with common fields and collapsed Advanced fields. `SettingEditor`, labelled options, custom-input state, hints and visibility belong to this shared model. Every edit uses the registry validation; invalid or unchanged drafts are never saved. Sensitive values are redacted and never prefilled. Unset is offered only for stored values; environment/flag overrides remain visible. |
| `Agents` | Agent kinds with their declared instances, then undeclared candidates; the selection follows (kind, directory) across reloads. Reloads on connect, `sync.finished`, an applied change and `ProbeAgents` (the explicit HOME name probe). See [agents and instances](#agents-and-instances). |
| `AgentForm` | The add, edit and remove wizard with per-keystroke validation, sequence-numbered previews and a revision-guarded apply. See [agents and instances](#agents-and-instances). |
| `Log` | A bounded ring buffer (`DefaultLogCapacity` 256) of hints, sync phases and setting writes; the oldest entry is dropped and counted. |

Every load carries a sequence number. A result older than the last applied one,
or one that was never requested, is discarded, so a slow response cannot
overwrite newer state. Hints are not a journal: a connection, `sync.finished` or
`drift.detected` reloads the snapshots. A `sync.started` hint from elsewhere is
cleared by its finish hint or by any successful load issued after it.

## Home projection

`App.Home(version)` supplies one headline, at most one secondary message,
severity, repository/branch/version edge labels and five compact menu items.
It reuses Status and Agents snapshots without loading another source. Counts,
check-time text and service/check-warning priorities belong here; the terminal
owns menu focus and screen routing. `RelativeText` shares catalog-backed time
hints with `WhenText`. Full status rows remain in `StatusView`.

## Settings presentation

`ToggleAdvancedSettings` controls the shared view's expanded state.
`BeginSetting`, `StepSetting` and `ChooseSetting` select an editor/option;
`EditSetting` validates text drafts, and `SaveSetting` requests persistence.
Renderers translate keys and mouse coordinates into those intents. Presets,
boolean labels, custom host/duration options and visibility are shared policy,
not renderer logic. The renderer owns focus and geometry; it keeps the editor
open until `SettingWritten` succeeds so full-file validation failures can be
corrected without losing the draft.

The normal and Advanced field lists, duration limits and hidden-key decisions
are documented in [the terminal settings guide](tui.md#settings-controls).
`migration.legacy_roots` is conditional and read-only in the form, with a CLI
editing hint for its TOML array. Hidden settings remain in the registry and
CLI. Changes to this presentation do not change config or API contracts.

`SettingValue` can carry optional host names, a Git config identity hint and a
legacy-detection flag. The API adapter enriches snapshots on the same local
host using injected platform and Git contracts. It reads the effective working
copy selected by the newly loaded settings, for both daemon and local fallback
responses. Missing files, unavailable Git or unsupported Git config leave the
hints unavailable without blocking the form. Host files are regular
`hosts/*.md` files; identity reads only user.name/email through the optional
`vcs.IdentityReader` capability. No signing keys or credentials are returned.

## Dialog presentation contract

`DialogView`, `DialogField`, `ConfirmDialog` and `FormDialog` describe reusable
confirmation and wizard windows without a terminal dependency. They carry
`text.Message` labels, enabled model controls, stable dialog/field IDs,
optional step counts, toned body lines (`DialogLine`), choice lists
(`DialogOption`, each with the intent its choice dispatches) and a wide-window
request. `EditDialogField` forwards draft changes to the owning
model. The owner validates, updates its projection and decides when the window
closes; a form action does not automatically advance a step or persist a value.
Cancel is an explicit intent, so a wizard can ask about a dirty draft by
projecting another confirmation window. Focus, geometry, text cursors, stacking
and hit testing belong to the renderer. See the [TUI component API](tui.md#component-api-for-future-tabs).

Settings projects an optional `SettingsView.Dialog` after
`RequestUnsetSetting`. Only a stored, writable value can open it. Its Yes
control carries the existing `UnsetSetting`; cancel carries `DismissDialog`.
The settings model and command runner retain their existing unset validation
and write behavior. Status projects `StatusView.Dialog` after
`RequestServiceRefresh` for an installed stale or mismatched service; Yes
carries `ConfirmServiceRefresh{Accept: true}` and cancel carries
`ConfirmServiceRefresh{}`. Shared models know nothing about terminal panes or cells.

## Texts

All user-visible text is in `internal/ui/model/text`: typed `Key` constants and
the English catalog `text.English()`. Views contain `text.Message` values;
arguments are messages too, so nested phrases stay translatable, and
`text.Plain` carries data such as paths, numbers and core warnings. Plain-language setting
descriptions live in the text catalog. Types, defaults, constraints and
validation still come from the registry behind [machine settings](config.md). Tests require every key to have an English
template using only `%s`, and every message a model emits to match its
template's argument count.

## Local API adapter

`internal/ui/apiclient.Client` implements `model.Client` over the local API.
Like `kitbag status` and `plan`, it falls back to the same in-process core
services only when no daemon listens (`platform.ErrNoDaemon`); authentication,
protocol and lock failures are reported. Errors become `*model.Error` with a
kind and the shared exit code.

- Status uses `status.get`; drift uses `plan.get`, keeping `drift.list`'s
  selection and adding each path's planned operations.
- Sync uses `sync.run` through `client.Stream`, which has no 30-second call
  bound, and forwards `sync.progress` phases.
- Settings resolve `config.get` bytes with this process's environment and flags
  through `settings.Entries`, exactly like `kitbag config list`. A change is
  `config.get`, `settings.Changed` (the same validation, comment refusal and
  encoding as `kitbag config set/unset`) and `config.set`.
- Events use `client.Subscribe`, which keeps one connection open with a
  `version.get` keepalive below the server's two-minute idle limit.

## API gaps

- **No content diffs.** The API deliberately omits file contents, so drift shows
  classification and planned actions, not a textual diff. A diff needs a new
  method and its own redaction decision.
- **No compare-and-swap for settings.** `config.set` replaces the whole file; a
  concurrent edit between the adapter's `config.get` and `config.set` is lost.
- **No update details.** `update.available` carries no version and status does
  not report it, so the model shows only that an update exists.
- **Tool-too-old through the daemon** is a sanitized error (exit 4) without the
  required version; the in-process fallback includes it.
- Catalog and publication models wait for their APIs (KIT-46, KIT-53).
- **Agents: no per-target settings flag.** `agents.list` does not say which
  targets declare a settings fragment, so the form always offers the settings
  step and the server refuses it at preview time.

## Agents and instances

The [v1 slice of ADR 12](adr/0012-agent-instance-discovery.md#v1-slice-shipped-first):
changes apply to all hosts, there is no publication queue, and publication
follows the host's publish mode ([agent edits](agents.md#add-edit-and-remove-instances)).

`Client` methods, backed by the additive local API methods `agents.list`,
`agents.add`, `agents.edit` and `agents.remove`
([local API](api/local-api.md#agent-instances-additive-v1-methods)); the adapter
falls back in-process when no daemon listens or an older daemon answers
method-not-found:

```go
Agents(ctx context.Context, probe bool) (AgentsSnapshot, error)
PreviewAgentChange(ctx context.Context, change AgentChange) (AgentPreview, error)
ApplyAgentChange(ctx context.Context, change AgentChange, revision string) (AgentApplied, error)
```

The adapter forwards only the known agents' relocation variables
(`apiclient.Client.AgentVariables`) from this process's environment to
`agents.list`, so the list matches what this shell's agents use. Changes use the
daemon's environment, like sync. Commands are `LoadAgents{Seq, Probe}`,
`PreviewAgent{Change, Seq}` and `ApplyAgent{Change, Revision}`; results are
`AgentsLoaded`, `AgentPreviewed` and `AgentChangeApplied`. All types are in
`internal/ui/model/agent_types.go`.

| Type | Fields |
| --- | --- |
| `AgentCandidate` | `Kind`, `Dir` (absolute), `Variable`, `Source` (`default`, `environment`, `declared`, `probe`), `Presence` (`present`, `missing`, `not_directory`, `symlink`, `outside_home`, `unreadable`) |
| `AgentInstance` | `AgentCandidate` plus `Target`, `Name`, `Hosts`, `ExcludedHosts`, `Implicit`, `Enabled`, `Settings`, `ActiveHere` |
| `AgentsSnapshot` | `Host`, `KnownHosts`, `Kinds`, `Instances`, `Candidates`, `Warnings`, `Schema`, `Daemon` |
| `AgentChange` | `Op` (`add`, `edit`, `remove`), `Kind`, `Target`, `Name`, `Dir`, `Settings` (nil: default or unchanged) |
| `AgentPreview` | `Change` (normalized), `Revision`, `Title`, `Message`, `Dropped`, `Diff`, `Warnings`, `Publication` (`State`, `Mode`, `Branch`, `Commit`, `Reason`, `Next`, `Attempts`) |
| `AgentApplied` | `AgentPreview` plus `Released` (dropped directory: `Dir`, `Changed`, `Warnings`), `Sync` (`ExitCode`, `Changed`, `Warnings`), `ExitCode` |

**`Agents`** (`NewAgents`, `Init`, `Update`, `Snapshot`, `View`). `AgentsView`
has `Title`, `Headline`, `Problem`, `Phase`, `Rows`, `Warnings`, `Detail` and
`Actions`. Rows are ordered by kind, declared instances first, each with
`Kind`, `Target`, `Name`, `Label`, `Dir`, `Source`, `Presence`, `Scope`,
`Declared`, `Implicit`, `ActiveHere` and `Selected`; source and presence are
words, never colour alone. `SelectAgent{Index}` and `MoveAgent{Delta}` select.
`Actions` are, in order: **Add**, or **Add this directory** for a selected candidate,
**Edit** and **Remove** (shown only for a declared, non-implicit instance),
**Look for more directories** (`ProbeAgents`) and **Refresh**.

**`AgentForm`** (`NewAgentForm`, `Open`, `Update`, `View`). `OpenAgentForm{Op,
Kind, Target, Name, Dir, Adopt}` starts it. Adoption retains Kind and Dir and
skips their steps when both pass validation; it starts at name, then settings,
preview, confirm and result. Back follows this shortened sequence and retains
the draft. Invalid prefilled paths retain the full sequence for correction. Steps: add — kind, name, directory,
settings, preview, confirm, result; edit — directory, settings, preview,
confirm, result; remove — preview, confirm, result.

- `EditAgentField{Value}` edits the name or directory; `ChooseAgentOption{Index}`
  picks a kind, a directory hint (undeclared candidates of the kind with their
  source and presence) or a settings choice (0 unchanged or default, 1 on,
  2 off).
- Every draft change is validated at once with `manifest.CheckName` and
  `manifest.CheckHomePath` (`~/...` or an absolute path), the name not taken in
  the target, the directory not declared by another instance of the kind, and
  not outside HOME; `generic` is refused (it needs explicit roots). A missing
  directory is a warning. `AgentFormNext{}` does nothing while the step is
  invalid. The server validates again.
- Entering preview issues `PreviewAgent` with a new sequence number; a stale
  result or any draft change discards the preview. Confirm ("Apply") issues
  `ApplyAgent` with the preview's revision. A conflict (exit 3: `kitbag.toml`
  changed, or the change no longer applies upstream) returns to the preview
  with a stale message and previews again; other failures stay on confirm with
  the problem and the server's detail.
- `AgentFormBack{}` goes back; on the first step, and `CancelAgentForm{}`, close
  the form, asking first (`Discarding`) when a draft exists; a second cancel
  discards.
- `ResumeAgentForm{}` answers the discard question with no and keeps the
  draft. The form also accepts `EditDialogField` from its own window
  (`AgentFormDialog`) as `EditAgentField`.
- `AgentForm.Dialog()` projects the open form as a `FormDialog`
  (`AgentFormDialog`): title "operation: step", Step n/N, the explanation as the
  message, the name or directory as a field (the edited instance's directory as
  its hint), options for kinds, found directories and settings, body lines for
  validation, warnings, problems, the busy state, the preview (commit, route,
  commands to propose a `pr`-mode change, handed-back directory, warnings and,
  on the preview step, the diff) and the result, then Cancel, Back and
  Next/Apply/Close. Every ready valid step initially focuses its primary button
  (Next, Apply, Close), including prefilled fields and known kinds. Invalid
  text or an unselected kind starts at the field/option with disabled Next.
  Explicit typing and cursor movement preserve editor focus on reprojection.
  Applying shows busy text without buttons and still refuses cancellation;
  the result has only Close.
  `AgentForm.DiscardDialog()` is a `ConfirmDialog` (`AgentDiscardDialog`) while
  `Discarding`: Yes cancels again (discarding), No resumes.
- `AgentFormView` carries `Op`, `Step`, `Steps`, `StepIndex`, `Title`,
  `Explanation`, `Field` and `TextEntry` for text steps, `Options`, `Invalid`,
  `Warnings`, `Problem`, `Busy`, `Preview` (title, commit, route, diff lines,
  dropped directory, warnings), `Result` (headline, commit, next commands,
  released outputs, sync line, warnings) and the `Back`, `Next`, `Cancel` and
  (on confirm) `Confirm` controls.

The selected row's detail ends with **Next sync**: what a sync does with the
directory (installs the shared configuration, skips it on this host, warns and
skips a missing or unusable directory, or leaves an undeclared or outside-HOME
directory alone).

All texts are in `internal/ui/model/text` (`agents.*`, `wizard.*`, `agent.*`,
`publish.*` keys). The terminal renders them in the
[Agents screen](tui.md#agents-screen-and-wizard).

Action rows omit operations with no effect: Settings hides unchanged Save/Revert
and Remove for inherited/default values, Agents hides Edit/Remove without a
declared instance, and empty Drift has no Details. Settings loading/saving has
no action row. Disabled Save for an invalid changed draft and disabled wizard
Next for validation/preview loading explain their restrictions. Status and Home retain
disabled Sync during sync/service refresh activity or an unmet tool version.
Status retains disabled service refresh during sync/refresh activity, with
status messages explaining availability. Confirmation and information dialogs
keep their usable Yes/No or OK actions.

Agent previews retain the response's exact `manifest` path and expanded
`directory`. Optional `effects` list the current machine's proposed sync actions
and dry-run unmanage actions for dropped outputs (kind, path, existing entry,
backup path and warning). The workflow plans against the proposed manifest
without writing it; release planning uses the same ownership and backup rules
as apply. The adapter forwards these facts to the form's sentence summary,
above the raw diff. Older daemons omitting effects are labelled unavailable.
Other hosts plan from their own files; Apply rechecks the local filesystem.
