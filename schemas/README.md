# Schemas

JSON Schema (draft 2020-12) for TOML configuration and command output. The contract
they encode is written out in [docs/manifest.md](../docs/manifest.md).

| File | `$id` | Describes |
| --- | --- | --- |
| `kitbag.schema.json` | `https://kitbag.sh/schemas/kitbag.schema.json` | `kitbag.toml`, committed in a configuration repository: schema 1 and 2, as the current reader accepts them |
| `kitbag.v1.schema.json` | `https://kitbag.sh/schemas/kitbag.v1.schema.json` | `kitbag.toml` schema 1 alone, for editors of files not yet upgraded |
| `agents.schema.json` | `https://kitbag.sh/schemas/agents.schema.json` | `kitbag agent list --json`; see [agent discovery](../docs/agents.md#discover-agent-directories-and-instances) |
| `manifest-upgrade.schema.json` | `https://kitbag.sh/schemas/manifest-upgrade.schema.json` | `kitbag manifest upgrade --json`; see [upgrading to schema 2](../docs/manifest.md#upgrading-to-schema-2) |
| `agent-change.schema.json` | `https://kitbag.sh/schemas/agent-change.schema.json` | `kitbag agent add/edit/remove --json`; see [agent edits](../docs/agents.md#add-edit-and-remove-instances) |
| `config.schema.json` | `https://kitbag.sh/schemas/config.schema.json` | `config.toml`, machine-local tool configuration |

The `$id` values are stable identifiers; nothing is fetched from them.

## Examples

`examples/<kind>/valid/*.toml` must validate; `examples/<kind>/invalid/*.toml`
must fail (manifest examples sit one level deeper, per schema version, in
`examples/kitbag/v1` and `examples/kitbag/v2`), and each starts with a line `# expect: <fragment>` naming the error
it is meant to produce. `internal/schemacheck` enforces both as part of
`mise run check`, together with the rule that TOML blocks quoted in
`docs/manifest.md` under an `<!-- example: path -->` marker are byte-identical
to the fixture at that path.

`examples/kitbag/v1/valid/host-instances.toml` uses schema-1 host replacement
sets and `examples/kitbag/v2/valid/host-instances.toml` the schema-2 `hosts`
filter with host patches.
`examples/kitbag/v1/valid/agent-config.toml` reproduces today's
`TomaszJanusz/agent-config` layout; `full.toml` exercises every key once and is
a fixture, not a recommendation.
