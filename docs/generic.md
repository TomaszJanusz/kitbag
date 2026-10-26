# Generic targets, files and Git includes

`internal/adapters/generic` implements `target.Adapter` for any manifest target
whose `agent` is `generic`. Target names identify arbitrary agents; each enabled
instance must name its directory and primitive roots. No default paths are
inferred. Skills enumerate child directories containing a regular `SKILL.md`;
collections preserve immediate child names, with `per_agent.generic` entries
replacing same-name shared entries. Naming a collection root declares its native
format. MCP has no generic format and produces an unsupported warning.

Instructions use the shared renderer with `concat` as default. Explicit `concat`
is supported; `import` fails at declaration time because generic agents have no
documented import syntax. Target and instance primitive controls and resolved
manifest modes apply; machine copy policy still wins. Missing instance directories
are skipped by the planner. Adapters read only through `manifest.FileSystem`.

`internal/adapters/files` implements the same contract for host-resolved `[[files]]`
entries. It has no agent defaults and imports no peer adapter. Each declaration
uses `Backup: Once`: core backs up a pre-existing unmanaged regular file before
linking or copying, and refuses another backup if that namespace is occupied.
Foreign links and real directories retain the planner's protection. Per-file
mode overrides the resolved manifest default; apply combines machine policy.
Depguard registers this non-agent package with the same isolated import boundary.

Git includes are **a separate idempotent step after apply**, not file/link targets.
`vcs.EnsureIncludes` uses the optional `vcs.GlobalConfig` system-backend capability
and supplied read-only filesystem. The smaller separate step avoids adding Git
process execution, config-specific operations or a second journal to the file
planner. Apply continues to write only its planned operations. Include additions
are additive, outside file ownership/recovery, and retried safely on the next
sync after failure; removal from the manifest never removes existing includes.
This step is not an atomic transaction with file apply, and concurrent external
Git config writers can race the read/add sequence.

Existing include values are compared after expanding `~`/`~/`, cleaning paths and
resolving symbolic links in every component, so an include of the same file
through a linked directory (for example agent-config-sync's include of a clone
reached through a link) causes no second entry. Missing components, link loops
and relative values are compared as spelled. An include of a different file,
such as the old clone's copy when `repo.path` names another working copy, is a
different include: KitBag adds its own and never removes the user's.
A matching chezmoi-managed entry causes no write, including in a linked or
read-only global config. Otherwise a symlink, nonregular or read-only config is
left alone with a warning naming the missing include. System Git performs reads
and additions through its existing Runner with an explicit clean environment;
only global commands honour `GIT_CONFIG_GLOBAL`. Relative or unclean global
config locations are refused so the safety check and Git name the same file.
Without it, Git's existing
`~/.gitconfig` takes write precedence over the XDG global config, then a new
`~/.gitconfig` is used. Includes are read without following included files.

Public `sync --no-pull` exercises these flows with a throwaway HOME and global
config. Nonblocking instruction warnings are retained through `Adapter.Plan`,
just as with Claude and Codex. No real home or global Git configuration is used
by tests.
