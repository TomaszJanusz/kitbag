# Manifest API baseline

`pkg/manifest` is the only public library package. `manifest.export` is the
export-data baseline written by the mise-pinned `apidiff`; it is an intentional
versioned API artifact, not a build output. `mise run check` compares the current
API against it and rejects additions as well as incompatible changes. Review
public changes before recording a replacement:

```sh
mise run manifest:api:update
mise exec -- bash scripts/manifest-api.sh
```

`Parse(filename, data)` checks the entire schema-v1 document without host access.
`Load(fsys, filename)` also checks repository source existence, file types,
symlink confinement, portable directory entries and skill uniqueness. Pass a
`platform.FS` (or its read-only subset); the tests use `platform/fake`. Paths must
be absolute and clean at the filesystem boundary. Foreign import subpaths are
intent for another repository and are not checked for local existence. Optional
host instruction additions may be missing. Neither entry point runs commands or
reads the process environment.

The returned model has named structs for the contract's tables, string enums,
and nullable boolean switches (`nil` means the documented default, usually true).
Unset mode, render, transport, merge and publish fields retain their documented
adapter or policy defaults. No public signature exposes an internal package type.

Before planning, call `m.CheckVersion(current)` and
`m.ResolveHost(env, configuredHost)`; a `platform.Env` satisfies `Environment`.
Host precedence is nonempty `KITBAG_HOST`, configured host, then the system's
short hostname. Explicit configured/override names are used verbatim. Host
resolution copies the model, replaces instance sets and the whole publish table,
filters host-specific files and expands `{host}`. Lower-level materialization
mode precedence and agent capability checks belong to the planner. Source paths
and skills are checked across all hosts, including disabled declarations.

Errors reject the whole document. `errors.As` finds `*manifest.Error` with the
filename, one-based byte line/column and JSON-pointer key. A missing key points
to its enclosing table (or 1:1 for a missing root key). Filesystem causes are
preserved for `errors.Is`; `errors.As` finds `*manifest.VersionError` when the
running tool is too old, including through a positioned diagnostic.

The TOML dependency is `github.com/pelletier/go-toml/v2 v2.2.4`. Its decoder
checks TOML syntax and its AST ranges locate semantic errors, including inline,
quoted and dotted keys and array tables. The unstable AST API is confined to
one private file and the exact library version is pinned. BurntSushi v1.6.0
remains the independent schema test decoder: its public metadata API exposes
keys but not key positions. JSON Schema remains test-only.

`internal/schemacheck` feeds the same three valid and 31 invalid manifest
fixtures to both readers, checks each intended rejection, compares diagnostic
goldens, and tests wrong-type/unknown-key mutations throughout the full fixture.
The ten invalid machine-config fixtures retain their separate schema gate;
`config.toml` parsing is outside this package. Re-run diagnostic updates with:

```sh
mise run manifest:golden
```

The short two-worker parser fuzz run is part of `mise run check` on both CI
operating systems. For a longer local run:

```sh
mise exec -- go test ./pkg/manifest -run='^$' -fuzz='^FuzzParse$' -fuzztime=30s -parallel=2
```

Validation is a snapshot, not a filesystem transaction: callers must account for
source changes between loading a manifest and applying a future plan.

For a model returned by `Load`, host resolution also checks any existing host
instruction addition for confinement and regular-file type. A missing addition
remains optional; the planner reports it and skips it when assembling content.

The internal local client/server contract is [API v1](api-v1.md), defined by
[JSON Schema](api-v1.schema.json). Its [transport guide](local-api.md) covers
permissions, token authentication, negotiation, lifecycle hints and CLI fallback.
It is independent of the public manifest Go API baseline above.
