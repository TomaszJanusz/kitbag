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
| `Settings` | One field per `internal/settingskey` registry entry with description, allowed values, default, effective value and source. Every edit is validated with the registry's rules; invalid or unchanged drafts are never saved. Sensitive values are redacted and never prefilled. Unset is offered only for values stored in `config.toml`; environment or flag overrides are shown. |
| `Log` | A bounded ring buffer (`DefaultLogCapacity` 256) of hints, sync phases and setting writes; the oldest entry is dropped and counted. |

Every load carries a sequence number. A result older than the last applied one,
or one that was never requested, is discarded, so a slow response cannot
overwrite newer state. Hints are not a journal: a connection, `sync.finished` or
`drift.detected` reloads the snapshots. A `sync.started` hint from elsewhere is
cleared by its finish hint or by any successful load issued after it.

## Texts

All user-visible text is in `internal/ui/model/text`: typed `Key` constants and
the English catalog `text.English()`. Views contain `text.Message` values;
arguments are messages too, so nested phrases stay translatable, and
`text.Plain` carries data such as paths, numbers and core warnings. Setting
descriptions come from the registry, the same source as
[machine settings](config.md). Tests require every key to have an English
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
