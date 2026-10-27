## kitbag agent

Inspect and change agent kinds and instances

### Options

```
  -h, --help   help for agent
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
* [kitbag agent add](kitbag_agent_add.md)	 - Declare an agent instance in kitbag.toml and publish it
* [kitbag agent edit](kitbag_agent_edit.md)	 - Change an instance's directory, settings merge or hosts and publish it
* [kitbag agent list](kitbag_agent_list.md)	 - List agent directories and declared instances
* [kitbag agent remove](kitbag_agent_remove.md)	 - Remove an instance from kitbag.toml and publish it

