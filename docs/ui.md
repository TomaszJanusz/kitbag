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
- **No agent or instance view.** Detected and declared agent instances are not
  shown, and adding one requires editing `kitbag.toml`; the proposal follows.

## Proposed: agents and instances (ADR 12)

Not implemented. [ADR 12](adr/0012-agent-instance-discovery.md) (Proposed)
recommends the following models; names may change during review.

`Client` gains these methods, backed by local API v1.1 `agents.list`,
`agents.preview`, `agents.apply`, `publish.status`, `publish.flush` and
`publish.drop`, with the usual in-process fallback:

```go
Agents(ctx context.Context, probe bool) (AgentsSnapshot, error)
PreviewAgentChange(ctx context.Context, change AgentChange) (AgentPreview, error)
ApplyAgentChange(ctx context.Context, change AgentChange, revision string, keepLocal bool) (AgentApplied, error)
Publication(ctx context.Context) (Publication, error)
FlushPublication(ctx context.Context, ids []string) (Publication, error)
DropPublication(ctx context.Context, ids []string) (Publication, error)
```

Changes never ask for a commit message: KitBag queues them, applies them here
at once and publishes them per the manifest's `[publish]` policy
([ADR 12, section 3](adr/0012-agent-instance-discovery.md#3-automatic-publication)).

The adapter sends only the relocation variables of known adapters from this
process's environment, as settings resolution does.

| Type | Fields |
| --- | --- |
| `AgentKind` | `Kind`, `Variable` (relocation variable or empty), `DefaultDir` (`~/` form), `Settings` (adapter merges a settings fragment), `Wizard` (offered by the wizard; false for `generic`) |
| `Presence` | `present`, `missing`, `not_directory`, `symlink`, `outside_home`, `unreadable` |
| `CandidateSource` | `default`, `environment`, `declared`, `probe` |
| `Instance` | `Kind`, `Target`, `Name`, `Dir`, `Hosts` (empty means all), `DisabledOn` (hosts with an `enabled = false` patch), `Implicit` (adapter default, not listed), `Enabled`, `Settings`, `ActiveHere`, `Presence`, `Pending` (queue entry ID or empty) |
| `Candidate` | `Kind`, `Dir`, `Source`, `Variable`, `Presence`; undeclared only |
| `AgentsSnapshot` | `Host`, `KnownHosts` (declared host names), `Schema` (manifest schema), `Kinds`, `Instances`, `Candidates`, `Revision`, `Publication`, `Warnings`, `Daemon` |
| `AgentChange` | `Op` (`add`, `edit`, `remove`), `Kind`, `Target`, `Name`, `Dir`, `Hosts` (empty means all hosts), `Here` (`enable`, `disable` or unset: a host patch for this host), `Settings` |
| `AgentPreview` | `Change`, `Diff` (added and removed manifest lines), `Items` (the `DriftItem` shape for affected destinations), `MaterializedDefault`, `NeedsSchema2`, `Route` (`direct`, `pull_request`, `held`, with branch), `Title` (generated commit title), `Warnings`, `Revision` |
| `AgentApplied` | `Revision`, `Publication`, `Warnings` |
| `PublishEntry` | `ID`, `Summary`, `Created`, `State` (`pending`, `held`, `published`, `pull_request`, `conflict`, `blocked`, `rejected`), `Reason`, `Commit`, `URL` |
| `Publication` | `Entries`, `Held` (machine-wide hold), `Mode` (this host's policy) |

| Model | States and rules |
| --- | --- |
| `Agents` | Loading, ready, failed. Declared instances grouped by kind, then undeclared candidates. Selection follows (kind, dir) across reloads. `ProbeAgents` reloads with the name probe; `sync.finished` reloads. |
| `AgentWizard` | Closed, then steps kind, directory, name and target, hosts, settings, preview, confirm, result. On a schema-1 manifest the hosts step offers only all hosts, plus a manifest-upgrade preview action. Every draft change is validated; **Next** is disabled while the draft is invalid. Entering preview issues `PreviewAgentChange` with a sequence number; a draft change discards the preview. Confirm sends the preview's revision; a conflict returns to preview with the stale message. Esc goes back; Esc on the first step with a draft asks before closing. |

Validation in the model, repeated by the server: instance name with
`manifest.CheckName`, directory with `manifest.CheckHomePath` (`~/` only),
host names with `manifest.CheckHostName`, duplicate (kind, dir), name already
used in the target, `outside_home`, a host filter on a schema-1 manifest, and
settings only where the target declares a fragment and the adapter merges it.

New text keys, English templates:

| Key | Template |
| --- | --- |
| `agents.title` | `Agents` |
| `agents.empty` | `No agents are declared or found on this machine.` |
| `agents.source.default` | `documented default` |
| `agents.source.environment` | `from %s` |
| `agents.source.declared` | `declared in kitbag.toml` |
| `agents.source.probe` | `found by name` |
| `agents.presence.present` | `present` |
| `agents.presence.missing` | `missing` |
| `agents.presence.not_directory` | `not a directory` |
| `agents.presence.symlink` | `symbolic link, not followed` |
| `agents.presence.outside_home` | `outside the home directory; cannot be declared` |
| `agents.presence.unreadable` | `cannot be inspected` |
| `agents.scope.all` | `all hosts` |
| `agents.scope.hosts` | `only on %s` |
| `agents.no_instance_here` | `Target %s has no instance on this host.` |
| `agents.pending` | `waiting to be published` |
| `wizard.step.kind` | `Agent` |
| `wizard.step.dir` | `Configuration directory` |
| `wizard.step.name` | `Instance name` |
| `wizard.step.scope` | `Where to use it` |
| `wizard.scope.here` | `This machine (%s)` |
| `wizard.scope.all` | `All machines` |
| `wizard.scope.choose` | `Choose machines…` |
| `wizard.apply` | `Apply` |
| `wizard.apply_local` | `Apply here only` |
| `wizard.step.settings` | `Merge shared settings` |
| `wizard.step.preview` | `What will change` |
| `wizard.step.confirm` | `Confirm` |
| `wizard.generic_cli` | `Generic agents need explicit roots; use kitbag agent add generic.` |
| `agent.invalid.name` | `Use up to 64 lowercase letters, digits, dots, underscores or hyphens, starting with a letter or digit.` |
| `agent.invalid.dir` | `Use a path that starts with ~/; other paths cannot be shared between machines.` |
| `agent.invalid.duplicate` | `%s is already declared as %s.` |
| `agent.invalid.name_taken` | `Target %s already has an instance named %s.` |
| `agent.invalid.no_settings` | `Target %s has no settings fragment to merge.` |
| `agent.warn.missing_dir` | `%s does not exist. Sync warns and skips it until the agent creates it.` |
| `agent.warn.default_declared` | `The default directory %s is declared as instance %s so it stays managed.` |
| `agent.invalid.host` | `Use up to 64 letters, digits, dots, underscores or hyphens for a host name.` |
| `agent.invalid.needs_schema2` | `Choosing machines needs manifest schema 2. Preview the upgrade first.` |
| `agent.warn.unknown_host` | `%s is not this machine and not declared under [hosts]; check the spelling.` |
| `publish.route.direct` | `Published directly to %s.` |
| `publish.route.pull_request` | `Proposed as a pull request against %s.` |
| `publish.route.held` | `Applied on this machine only; not published yet.` |
| `publish.state.pending` | `Waiting to publish; it is already applied here.` |
| `publish.state.conflict` | `The repository changed in a way that contradicts this change. Drop it, edit it or keep it here only.` |
| `publish.state.blocked` | `Not published: %s` |
| `publish.state.rejected` | `The pull request was closed without merging.` |
| `agent.error.stale` | `kitbag.toml changed since the preview. Review the new preview.` |
| `agent.error.layout` | `kitbag.toml defines %s in a form KitBag cannot edit safely. Edit it by hand.` |

`agent.error.stale` and `publish.state.conflict` are `conflict` (exit 3);
validation and `agent.error.layout` are `invalid` (exit 2). Renderer behavior for the Agents
tab and wizard is in ADR 12, section 5; [tui.md](tui.md) changes when it is
implemented.
