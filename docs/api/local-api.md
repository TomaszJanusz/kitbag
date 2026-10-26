# Local API transport and compatibility

`internal/api.Serve(ctx, listener, handlers)` supplies the server boundary for
KIT-30. It owns the listener and active connections until cancellation, waits
for handlers to stop, and starts no service, timer, tray or daemon process.
`cmd/kitbag` supplies handlers backed by the **same** `workflow.Service` that
runs CLI status, plan and sync. A fresh service loads configuration for every
request. Handler factories must be concurrency-safe and honor cancellation.

The source of truth is [api-v1.schema.json](api-v1.schema.json). It describes
requests, method params/results, errors and notifications, including executable
valid/invalid cases in `x-methods`. `mise run docs:api` generates the
[reference](api-v1.md) and Go contract tests. `TestAPIArtifacts`, included in
`mise run check`, checks both artifacts byte for byte. Generated cases validate
the request against its schema, round-trip through a real temporary Unix socket
and the production handlers, validate the actual result, and assert the invalid
case's error. The plan projection reuses the existing plan schema.

## Framing and limits

Each UTF-8 JSON-RPC 2.0 object occupies one newline-terminated frame. Clients
escape embedded newlines in strings. Batches are unsupported (`-32600`). Requests
on a connection execute serially; separate connections run independently. The
server permits 32 active connections; excess connections close immediately.
Frames including their newline must fit within 1 MiB. An oversized frame receives
`-32004` and its connection closes. Before closing, the server discards through
that frame's newline, bounded to one additional MiB and two seconds, so a client
finishing a bounded write can read the error. Clients sending beyond those bounds
may observe a reset or broken pipe instead. Malformed JSON receives `-32700`; a later
valid frame on the same connection can succeed. IDs may be strings, numbers or
null; omitting an ID makes a notification, which receives no response. Invalid
JSON/envelopes receive an error with null ID.

Responses and notifications can be interleaved. Match responses by ID and ignore
unknown optional result/notification fields. The Go client bounds response
frames to 8 MiB and ordinary calls to 30 seconds; `Stream` drops the call bound
for long methods such as `sync.run`, and custom streaming clients can use the
wire contract with their own deadline. `Subscribe` holds a subscription open
with a `version.get` keepalive every minute; [view models](../ui.md) use both. The server closes connections
idle on reads for two minutes, or blocked on writes for two seconds. Reconnect
and resubscribe after a disconnect; snapshots remain authoritative.

## Endpoint and authorization

`platform.Env.RuntimeDir()` resolves `$XDG_RUNTIME_DIR/kitbag`, ignoring unset,
empty or relative values and otherwise falling back to `<state>/run`. The socket
is `api-v1.sock` there. On macOS the same XDG layout applies. Configure a shorter
absolute XDG runtime path if the OS socket path limit is exceeded. The fallback
can retain a stale socket after a crash: KIT-30 must explicitly verify that no
daemon owns it before removing it. Listening never unlinks an existing entry.
Normal listener Close removes its socket.

Unix `ListenLocal` creates the runtime directory with **0700** and the socket
with **0600**, independent of umask. An existing directory must be a real,
owner-owned directory with exactly 0700; a symlink or broader permissions fail
closed. The directory protects socket creation before chmod. `DialLocal` also
checks directory/socket type, ownership and exact modes. Both ends verify the
kernel peer UID against the effective UID: Linux `SO_PEERCRED`, macOS
`LOCAL_PEERCRED`. A foreign or unverifiable server is rejected; foreign client
connections close without disrupting the accept loop. Root and other processes
with the same UID are outside this isolation boundary. The fake models the same
permissions and an explicit foreign-peer decision. Windows returns
`platform.ErrUnsupported`; named pipes belong to its later implementation.

`api.EnsureToken(host)` creates `<state>/api-token` once with 256 random bits,
encoded as 64 lowercase hex characters, mode **0600**. Creation shares a
nonblocking lock; concurrent creation returns busy rather than replacing a
token. Existing malformed, nonregular, symlinked or insecure-mode tokens fail
closed. The directory is created as 0700 if absent. Every request must include
`params.token`; authorization compares fixed-size SHA-256 digests with
`subtle.ConstantTimeCompare`. Authentication is checked before version/method
dispatch. The server never logs requests or causes, and errors contain only
fixed public messages, API range and CLI exit code. Method data deliberately
includes requested machine TOML but never agent settings/credentials or rendered
file contents. Machine config must follow its existing no-secrets contract.
Token rotation requires restarting the server with the new token.

## Negotiation

Every request includes `params.api_version` as `major.minor`, for example
`"1.0"`. Call `version.get` to discover `min_version`, `max_version`, the tool
version and available methods. This server advertises **1.0–1.0**. A client
requesting 1.99 selects **1.0**, reported in every successful result's
`api_version`. A client then uses features in that selected minor or checks the
advertised methods before using an extension. Unknown methods return `-32601`
without closing the connection. Unknown optional request fields are ignored.
Malformed versions and other majors return `-32001` with the supported range.

Minor versions may add optional fields/methods; they must retain existing
required inputs, result meanings and errors. New required inputs, removed
methods and changed meanings need a new major and schema. The negotiation and
unknown-extension integration test exercises a newer minor client against this
older daemon. JSON-RPC IDs and authentication are independent of API version.

## Lifecycle and progress

`sync.run` accepts optional `no_pull` and `force`, with exactly the existing
workflow semantics. It sends `sync.progress` notifications on the calling
connection, correlated by `request_id`: `started`, optional `pull`, `plan`,
`apply`, `includes`, `finished`. These are phase boundaries rather than per-file
percentages; an error can omit later phases. The final response reports the
shared CLI exit classification and changed count, or the sanitized operation
error. Sync remains serialized by the existing state lock.

`events.subscribe` registers the current connection. Repeating it replaces the
subscription. Subscribers receive `sync.started`/`sync.finished`; the daemon can
publish `drift.detected` and `update.available` through `Handlers.Events` after
its detectors run. KIT-30 supplies [polling/detection](../daemon.md).

Every broker subscriber queue and connection output queue holds at most **32**
messages. Full notification queues **drop the newest hint** without blocking;
no goroutine or overflow list is created per event. A slow subscriber cannot
block sync or another subscriber. Responses wait only on their own connection
and are never deliberately dropped. Progress/lifecycle hints, including finish
hints, may be lost; clients refresh `status.get`, `plan.get` or `drift.list` after
sync and after reconnecting. Notifications are not a replayable event journal.

## CLI and configuration

Ordinary `kitbag status` and `kitbag plan` try the local API. Explicit config,
host, mode and Git backend selections are forwarded. Only a missing endpoint or
connection refusal allows in-process fallback. Bad permissions, token failures,
protocol errors and timeouts remain errors. The same renderers produce text and
JSON on both paths; a testscript compares both streams byte for byte. `--verbose`
runs core inspection in-process so backend resolution diagnostics retain their
existing wording; status still reads live check metadata from the API when
available.
CLI sync and mutation semantics remain unchanged. `kitbag daemon` owns polling,
detection and endpoint lifetime; see [daemon operation](../daemon.md).

`config.get` returns validated machine TOML. `config.set` accepts a complete
TOML string through `settings.Service.Replace`, validating with `config.Parse`
before taking the CLI settings lock and replacing it atomically with 0600. The
same regular-file checks apply as for CLI settings changes. Invalid input leaves the old bytes
intact. Running operations retain their loaded config; subsequent operations
reload it. This is whole-file replacement, without a read-modify-write CAS.

Error `data.exit_code` uses the shared CLI table: 1 operational/warnings, 2
usage/configuration, 3 conflict, 4 tool too old, 5 unsupported OS. Transport and
protocol errors use 2; lock contention uses 1. Synchronization warnings/conflicts
can instead be successful JSON-RPC results with nonzero `exit_code`.

## Remote check status (additive v1 fields)

`status.get` and `kitbag status --json` add optional `last_check` and
`daemon_running` fields without changing `status_schema: 1` or API 1.0.
`last_check` is null until a check completes, otherwise the versioned record
documented in [state](../state.md#last-remote-check). Its `at` and `next_at`
are UTC RFC 3339; `head` is a short hex identifier, not a remote URL.
`outcome: failed` always carries a safe catalog `category`, never a raw cause.
The in-memory daemon snapshot is authoritative while it runs. The local
fallback reads the atomic persisted snapshot, sets `daemon_running: false`,
and treats the record as historical/stale. `next_at` is then historical, not
a promised next check. Older v1 servers omit these fields; clients accept that
as unknown check status. A repository override never receives another
repository's check.

`poll.finished` is a content-free lifecycle hint after every completed check,
including startup, failure and no change; subscribers refresh `status.get`.
The existing bounded, lossy delivery contract still applies. No-change checks
do not emit a sync event or alter the last sync. A failed check or a running
daemon's check older than three `interval_seconds` is a presentation warning.
The machine status stays successful when only these warnings occur.
