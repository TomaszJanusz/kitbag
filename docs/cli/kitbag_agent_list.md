## kitbag agent list

List agent directories and declared instances

### Synopsis

Show known native agent kinds, declared instances and undeclared directory candidates using the caller's relocation variables. Read-only: no Git, daemon, state lock or writes. An absent configuration or manifest is reported without preventing discovery.

```
kitbag agent list [flags]
```

### Options

```
  -h, --help    help for list
      --json    Print stable agents_schema=1 JSON
      --probe   Detection never lists HOME on its own. Only an explicit request (kitbag agent list --probe or Look for more directories in the terminal interface) reads the names of HOME's direct entries, once, without descending into or following any of them. Names that do not match a known agent directory are discarded without being shown or stored. Detection never reads agent settings, credentials, sessions or instance files, and never runs an agent executable.
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

* [kitbag agent](kitbag_agent.md)	 - Inspect agent kinds and instances

