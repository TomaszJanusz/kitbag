<!-- Generated from api-v1.schema.json; DO NOT EDIT. -->
# Local API v1.0 reference

See [transport and compatibility](local-api.md). Every method uses the schema's
`params` envelope and returns `{api_version, data}`. Unknown optional request
fields are ignored. Method result types are in `$defs` of the schema.

## `status.get`

Report local Git status, last sync and drift counts.

Request: `$defs/status_getRequest`; result data: `$defs/status`.

Example method arguments (add `token` and `api_version`):

```json
{}
```

## `plan.get`

Build a safe read-only plan without file contents.

Request: `$defs/plan_getRequest`; result data: `$defs/plan`.

Example method arguments (add `token` and `api_version`):

```json
{}
```

## `sync.run`

Run the existing pull, plan and apply workflow; progress is coarse phases.

Request: `$defs/sync_runRequest`; result data: `$defs/sync`.

Example method arguments (add `token` and `api_version`):

```json
{"no_pull":true}
```

## `drift.list`

List observations other than in-sync targets.

Request: `$defs/drift_listRequest`; result data: `$defs/drift`.

Example method arguments (add `token` and `api_version`):

```json
{}
```

## `config.get`

Read validated machine TOML; never reads agent credentials.

Request: `$defs/config_getRequest`; result data: `$defs/config`.

Example method arguments (add `token` and `api_version`):

```json
{}
```

## `config.set`

Validate complete machine TOML before atomic replacement; next operation reloads it.

Request: `$defs/config_setRequest`; result data: `$defs/config`.

Example method arguments (add `token` and `api_version`):

```json
{"config":"schema = 1\n[repo]\nurl = \"https://example.invalid/config.git\"\npath = \"~/repo\"\n"}
```

## `events.subscribe`

Subscribe this connection once; repeat replaces its subscription.

Request: `$defs/events_subscribeRequest`; result data: `$defs/subscription`.

Example method arguments (add `token` and `api_version`):

```json
{}
```

## `version.get`

Advertise API range, tool version and method names.

Request: `$defs/version_getRequest`; result data: `$defs/version`.

Example method arguments (add `token` and `api_version`):

```json
{}
```

## Errors

| JSON-RPC code | Meaning | CLI exit |
| --- | --- | --- |
| -32700 | Parse error | 2 |
| -32600 | Invalid request or unsupported batch | 2 |
| -32601 | Unknown method; optional extension unavailable | 2 |
| -32602 | Invalid parameter types | 2 |
| -32603 | Internal serialization error | 1 |
| -32000 | Missing or wrong token | 2 |
| -32001 | Unsupported API major/version syntax | 2 |
| -32002 | Core/config operation failed; exit_code classifies the cause | 1–5 |
| -32003 | State/config/token lock busy | 1 |
| -32004 | Request frame exceeds the limit | 2 |

## Notifications

- `sync.progress`: Request connection: started, pull, plan, apply, includes, finished.
- `sync.started`: Subscribers: a sync was requested.
- `sync.finished`: Subscribers: sync completed, including failures.
- `drift.detected`: Subscribers: daemon detected drift; refresh snapshots.
- `update.available`: Subscribers: daemon detected a tool update.
