# SchemaStore submission handoff

Prepared only: no SchemaStore PR has been submitted. The maintainer must approve
submission after resolving the hosting and compatibility questions below.

The ready-to-copy entries are in
[schemastore/catalog-entries.json](schemastore/catalog-entries.json). Append those
objects to `schemas` in SchemaStore's `src/api/json/catalog.json`, not as a nested
array. `kitbag.toml` matches the manifest; `**/kitbag/config.toml` scopes machine
settings to KitBag's directory. A bare `config.toml` match would incorrectly
validate unrelated applications. An arbitrarily named `--config` file needs a
local association instead. Confirm the scoped glob against each target client.

Schema identifiers and planned catalog URLs are already consistent:

- `schemas/kitbag.schema.json`: `https://kitbag.sh/schemas/kitbag.schema.json`
- `schemas/config.schema.json`: `https://kitbag.sh/schemas/config.schema.json`

After this KitBag PR merges, raw GitHub distribution alternatives are:

- `https://raw.githubusercontent.com/TomaszJanusz/kitbag/main/schemas/kitbag.schema.json`
- `https://raw.githubusercontent.com/TomaszJanusz/kitbag/main/schemas/config.schema.json`

These are retrieval URLs, not proposed replacements for the accepted `$id`
identifiers. No schema identifier or accepted decision is changed here.

## Open questions before submission

1. Who will publish and maintain the `kitbag.sh/schemas/` endpoints? They must
   return these exact JSON documents successfully before the catalog PR opens.
   Public endpoint availability has not been verified. If using raw GitHub URLs
   for the catalog instead, review that choice and update the prepared entries.
2. Both existing schemas use draft 2020-12. SchemaStore currently recommends
   draft-07; Taplo's CLI validation documentation advertises Draft 4. Confirm
   actual VS Code, Zed and Taplo compatibility, including `const`, `$defs` and
   required/unknown-key validation. Do not downgrade the accepted schema contract
   without a separately reviewed decision or a tested compatibility artifact.
3. Confirm SchemaStore accepts the scoped machine-file glob, and that the selected
   TOML language servers consume the catalog. Catalog inclusion alone does not
   prove automatic validation in every editor/version.

These questions follow the upstream [SchemaStore contribution guide](https://github.com/SchemaStore/schemastore/blob/master/CONTRIBUTING.md)
and [Taplo validation documentation](https://taplo.tamasfe.dev/cli/usage/validation.html).

## Local validation before catalog publication

The repository's pinned, offline schema/runtime checks remain authoritative:

```sh
mise exec -- go test ./internal/schemacheck ./internal/config ./cmd/kitbag
```

If a reviewed/pinned Taplo version is available, run from the repository root:

```sh
taplo check --schema schemas/kitbag.schema.json schemas/examples/kitbag/v1/valid/minimal.toml
taplo check --schema schemas/config.schema.json schemas/examples/config/valid/minimal.toml
```

Taplo is not installed or pinned by this task; these commands and real editor integration are unverified.
Also check deliberately invalid copies (unknown key, wrong type, unsupported
schema and invalid enum); success on valid input alone is insufficient.

For VS Code with a TOML language server such as Even Better TOML, or Zed's TOML
language server, place a local `.taplo.toml` alongside a disposable fixture:

```toml
[[rule]]
include = ["kitbag.toml"]
[rule.schema]
path = "/absolute/path/to/kitbag/schemas/kitbag.schema.json"

[[rule]]
include = ["config.toml"]
[rule.schema]
path = "/absolute/path/to/kitbag/schemas/config.schema.json"
```

Taplo documents these rule/schema associations in its
[configuration reference](https://taplo.tamasfe.dev/configuration/file.html).
Check completions and invalid-value diagnostics with both files before removing
local associations and testing the catalog version. Editor extension/server
versions and any unsupported schema keywords must be recorded by the maintainer.

## Exact submission steps after maintainer approval

1. Resolve the questions above, merge the KitBag PR, and verify both catalog
   URLs and their `$id` fields against the published files.
2. Fork SchemaStore/schemastore, create branch `add-kitbag-schemas` from its current
   default branch, and follow its current contributing prerequisites.
3. Add the two prepared objects to `src/api/json/catalog.json`. For externally
   hosted schemas, follow the guide's remote-schema procedure and current checks;
   include representative valid and invalid TOML examples if requested upstream.
4. Run the checks specified by that checkout's contributor guide. Review the
   diff for unrelated changes, then commit `Add KitBag configuration schemas`.
5. Only after explicit maintainer approval, push the fork branch and open a PR
   against SchemaStore/schemastore using the text below.

Title: `Add KitBag manifest and machine configuration schemas`

PR body:

> Add catalog associations for KitBag's schema-v1 TOML contracts: `kitbag.toml`
> for shared agent configuration, and `**/kitbag/config.toml` for machine settings.
> The scoped machine pattern avoids claiming unrelated applications' config.toml.
>
> The schemas are maintained in TomaszJanusz/kitbag under schemas/ and published
> at the catalog URLs. KitBag checks schema examples and runtime parity in CI.
> Fill in the verified public URL checks, SchemaStore checks and editor/server
> versions here before submitting. Describe any draft compatibility artifact.
