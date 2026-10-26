## kitbag init

Set up a configuration clone or scaffold a new repository

### Synopsis

Clone a configuration repository, detect existing supported agent directories, show a dry plan and save machine configuration after confirmation. Existing configuration is never replaced. Then offer the background service: the wizard asks (default yes), --service installs it, --no-service skips it, and --yes alone prints how to install it later. Use --yes for headless setup, or --scaffold <dir> for a new repository matching the minimal manifest example.

```
kitbag init [git-url] [flags]
```

### Options

```
      --agents strings          Scaffold agent kinds: claude,codex (default: detected, else claude)
      --branch string           Repository branch (default: remote default)
  -h, --help                    help for init
      --identity-email string   Fallback unsigned embedded commit author email
      --identity-name string    Fallback unsigned embedded commit author name
      --instance stringArray    Scaffold instance: agent.name=~/directory (repeatable)
      --no-service              Do not install the background service and do not ask
      --repo string             Repository URL (alternative to the positional URL)
      --repo-path string        Use or create the working copy at this path below HOME instead of the data directory, for example an existing clone
      --scaffold                Create a new repository in the positional directory
      --service                 Install and start the background service (daemon mode) without asking
      --yes                     Accept defaults and confirmations without reading terminal input
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

