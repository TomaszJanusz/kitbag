## kitbag agent remove

Remove an instance from kitbag.toml and publish it

### Synopsis

Remove an instance declaration. Neither the directory nor any file in it is deleted: the next sync removes only the links and unchanged copies KitBag made there and releases ownership; modified or unmanaged files stay and are reported. The last listed instance of a target cannot be removed, because the target would then manage its default directory.

Changes are written to the shared kitbag.toml in the working copy and apply
to every host (per-host instances need manifest schema 2). The working copy must
be clean, on its configured branch and without unpublished commits. KitBag
fast-forwards it, edits only the affected lines (comments and layout stay),
commits with a generated Conventional Commit message (Kitbag-Host and Kitbag-Op
trailers) and, when this host's publish mode is direct, pushes without force; a
rejected push is replayed once on the new upstream. A published change is then
synchronized. With publish mode pr nothing is written: the preview and the
commands to propose it are printed. A preview is shown first; without a terminal
pass --yes, or --dry-run to only preview.

```
kitbag agent remove <target-or-kind>/<name> [flags]
```

### Options

```
      --dry-run   Show the change, diff and commit message without writing
  -h, --help      help for remove
      --json      Print stable agent_change_schema=1 JSON (needs --yes or --dry-run)
      --yes       Apply without asking for confirmation
```

### Options inherited from parent commands

```
      --config string        Machine configuration file (default: <config>/config.toml)
      --git-backend string   Git backend: auto, system or embedded
      --host string          Host override (before KITBAG_HOST and config host)
      --mode string          Materialization mode: link or copy (before KITBAG_MODE and config)
      --quiet                Suppress successful sync change output
      --verbose              Show backend selection and working copy diagnostics
```

### SEE ALSO

* [kitbag agent](kitbag_agent.md)	 - Inspect and change agent kinds and instances

