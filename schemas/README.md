# Schemas

JSON Schema (draft 2020-12) for TOML configuration and command output. The contract
they encode is written out in [docs/manifest.md](../docs/manifest.md).

| File | `$id` | Describes |
| --- | --- | --- |
| `kitbag.schema.json` | `https://kitbag.sh/schemas/kitbag.schema.json` | `kitbag.toml`, committed in a configuration repository |
| `agents.schema.json` | `https://kitbag.sh/schemas/agents.schema.json` | `kitbag agent list --json`; see [agent discovery](../docs/agents.md#discover-agent-directories-and-instances) |
| `config.schema.json` | `https://kitbag.sh/schemas/config.schema.json` | `config.toml`, machine-local tool configuration |

The `$id` values are stable identifiers; nothing is fetched from them.

## Examples

`examples/<kind>/valid/*.toml` must validate; `examples/<kind>/invalid/*.toml`
must fail, and each starts with a line `# expect: <fragment>` naming the error
it is meant to produce. `internal/schemacheck` enforces both as part of
`mise run check`, together with the rule that TOML blocks quoted in
`docs/manifest.md` under an `<!-- example: path -->` marker are byte-identical
to the fixture at that path.

`examples/kitbag/valid/agent-config.toml` reproduces today's
`TomaszJanusz/agent-config` layout; `full.toml` exercises every key once and is
a fixture, not a recommendation.
