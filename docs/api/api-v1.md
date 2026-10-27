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

## `agents.list`

List agent kinds, declared instances and undeclared directories (agents_schema 1) with the caller's forwarded relocation variables; probe adds the explicit HOME name probe. Read-only.

Request: `$defs/agents_listRequest`; result data: `$defs/agents`.

Example method arguments (add `token` and `api_version`):

```json
{"probe":false,"env":{"CLAUDE_CONFIG_DIR":""}}
```

## `agents.add`

Declare an instance for all hosts. With dry_run, preview the manifest diff, commit title and route; otherwise publish like kitbag agent add --yes (revision guards a confirmed preview) and sync.

Request: `$defs/agents_addRequest`; result data: `$defs/agentChangeResult`.

Example method arguments (add `token` and `api_version`):

```json
{"agent":{"kind":"claude","name":"third","dir":"~/.claude_3"},"dry_run":true}
```

## `agents.edit`

Change an instance's directory, settings switch or hosts; dry_run previews, otherwise publishes and syncs like kitbag agent edit --yes.

Request: `$defs/agents_editRequest`; result data: `$defs/agentChangeResult`.

Example method arguments (add `token` and `api_version`):

```json
{"agent":{"target":"claude","name":"second","dir":"~/.claude-work"},"dry_run":true}
```

## `agents.remove`

Remove an instance declaration; dry_run previews, otherwise publishes, hands the directory back by unmanage rules and syncs like kitbag agent remove --yes.

Request: `$defs/agents_removeRequest`; result data: `$defs/agentChangeResult`.

Example method arguments (add `token` and `api_version`):

```json
{"agent":{"target":"claude","name":"second"},"dry_run":true}
```

## `manifest.upgrade`

Convert kitbag.toml from schema 1 to schema 2 losslessly (manifest_upgrade_schema 1); dry_run previews the conversions and diff, otherwise publishes and syncs like kitbag manifest upgrade --yes. A manifest already on schema 2 returns already_current.

Request: `$defs/manifest_upgradeRequest`; result data: `$defs/manifestUpgradeResult`.

Example method arguments (add `token` and `api_version`):

```json
{"dry_run":true}
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
- `poll.finished`: Subscribers: remote check completed, including failure or no change; refresh status.get. Initial sync also produces a check. No raw errors or repository data in the hint.
