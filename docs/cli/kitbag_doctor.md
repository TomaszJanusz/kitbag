## kitbag doctor

Explain synchronization problems and suggest fixes

### Synopsis

Check local sync prerequisites and remote reachability without applying changes. --report writes redacted local JSON and skips network probes. WARN exits 0 unless --strict; FAIL exits 1, or 4 when min_tool_version is unmet.

```
kitbag doctor [flags]
```

### Options

```
  -h, --help            help for doctor
      --json            Print stable doctor_schema=1 JSON
      --report string   Write a local redacted JSON report (skips network probes)
      --strict          Exit 1 for warnings as well as failures
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

