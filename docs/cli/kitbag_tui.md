## kitbag tui

Open the interactive terminal interface

### Synopsis

Show status, drift, settings, agents and activity in a full-screen terminal interface; the Agents tab adds, edits and removes agent instances with a previewed, confirmed wizard. It uses the running daemon and falls back to reading this machine directly when none listens. Requires an interactive terminal; NO_COLOR or ui.color=never disables colour. Press ? for keys.

```
kitbag tui [flags]
```

### Options

```
  -h, --help   help for tui
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

* [kitbag](kitbag.md)	 - Synchronize coding-agent configuration

