## kitbag self-update

Verify and install a newer release or show the package manager command

### Synopsis

Respect the install-source stamp. Direct installs verify a cosign-signed checksum file and archive before atomic replacement and version confirmation. Unknown sources print reinstall guidance. --check and --dry-run verify without writing.

```
kitbag self-update [flags]
```

### Options

```
      --channel string   Release channel: stable or nightly (default "stable")
      --check            Verify the available release without installing
      --dry-run          Verify the available release without installing (alias for --check)
  -h, --help             help for self-update
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

