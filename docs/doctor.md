# Diagnosing synchronization

Run `kitbag doctor` to explain synchronization prerequisites without applying a
plan. Results are grouped **OK**, **WARN**, **FAIL**; each warning/failure has a
one-line `fix:` hint. Checks continue after failures when their prerequisites
remain available. Like status and the TUI, doctor automatically requests bounded
service recovery when it observes an eligible older daemon. It shows a transient
notice for at most 150 seconds; a persistent mismatch or failed recovery has an
actionable warning. Recovery preserves the saved service inputs and never
starts an intentionally stopped or disabled service. Text groups and JSON
registry order are deterministic.

`kitbag doctor --json` emits `doctor_schema: 1`, validated against
[`doctor.schema.json`](../schemas/doctor.schema.json). Every check has `id`,
`status`, `message`, and `fix` (empty for OK). Skipped checks are OK and explicitly
say why they were skipped. An unavailable prerequisite omits its dependent
checks; it does not assert that they passed.

| Check | Evidence and fix |
| --- | --- |
| Manifest/host | Load the local `kitbag.toml` and resolve the selected host; correct the working-copy path, manifest or host. The `host` check shows the host and its source; it WARNs when the host is the unconfigured system hostname, has no instructions addition and other hosts have one, naming their files: set the host with `kitbag config set host <name>` (never guessed). |
| Tool version | Compare the running semantic version with `min_tool_version`; upgrade kitbag. Development builds use `0.0.0-dev`. |
| Manifest schema | `manifest-schema`, after Manifest/host: reports the manifest `schema` in use, OK as `schema 1 (supported)` or `schema 2 (supported)`. A schema newer than this kitbag reads is the only reported check and FAILs: upgrade kitbag. An older or malformed schema is a manifest validation failure. |
| Instance hosts | `instance-hosts-<target>/<instance>`, schema 2 only: WARNs when an instance's `hosts` list names a host that is neither this machine nor declared under `[hosts]`, a likely typo. Correct the name, or ignore it when that machine really uses it without a host table. |
| Git backend | Requested/active backend, version and selection source from `vcs.Open`; install suitable Git or select an available backend. An automatic fallback warns with its reason. |
| Remote | Read origin refs through the backend's optional `vcs.RemoteProber`; check URL, connectivity and authentication. No fetch, ref update, or commit. Embedded SSH reports the successful credential source (agent or identity filename only); unsupported backends warn. |
| SSH agent | `SSH_AUTH_SOCK` must name an existing socket; start an agent/export its socket. Missing is WARN, including for non-SSH remotes where it may be irrelevant. No key listing. |
| Commit identity | System Git reads effective `user.name` and `user.email`; set missing keys. Values are never returned. |
| Commit signing | System Git reads `commit.gpgsign` and presence of `user.signingkey`; configure a key or explicitly disable signing. Signing enabled with an implicit default key warns conservatively. Key availability, passphrases, and a successful signature are not verified. |
| Agents | Probe the implemented native agents, `claude --version` and `codex --version`, through an injected process runner. Missing binaries, failed probes and timeouts WARN; install/check PATH. Generic agents have no assumed binary. |
| Instances | Check every enabled, host-resolved listed instance (in schema 2, only instances whose `hosts` include this machine) and native default directory (Claude, Codex, Gemini, OpenCode, resolved as by their adapters). Explicit dirs override agent home variables. Missing/non-directory instances WARN; create/correct the directory. A missing default is allowed to be created by sync. |
| Copy/link | Create unpredictable scratch names in the kitbag cache; verify atomic bytes and a symbolic link, then remove them. Repair cache permissions/filesystem support or use copy mode. Link failure WARNs when machine mode is copy, FAILs otherwise. This tests the cache filesystem, not all destination mounts or native agent loading. |
| State | Read and validate state and last-sync schemas through the state store while holding its nonblocking lock. Release immediately; never save/recover state. Restore a valid backup/use a compatible tool for corrupt or newer data. Wait for an active sync or correct lock/directory permissions. A leftover unlocked lock file is healthy. |
| Legacy links | With `migration.legacy_roots`, inspect shared/instance skills, instructions, old executable and agent-browser links into those roots or `~/.agents/skills`; review them, then run `kitbag adopt` ([adopt.md](adopt.md)). Dangling links count. Destinations KitBag owns (adopted or synced) are not legacy and are skipped; their drift is `plan`/`status` output. Without readable state nothing is skipped. |
| Legacy markers | Inspect instruction files for `agent-config-managed` or `Managed by agent-config-sync`; review/migrate them. Owned files, such as adopted ones that keep the script's marker until their output changes, are skipped. |
| Legacy settings markers | `legacy-settings-markers`: for every enabled Claude instance, a bounded byte search of `<dir>/settings.json` for the hook marker `# agent-config-managed`. It reports only the instance, the path and the number of occurrences; run `kitbag adopt`, then `kitbag sync` ([adopt.md](adopt.md)). A missing file is OK; an unreadable file, or one larger than 4 MiB (not read at all), WARNs with a permission/size hint. |
| Service | Background service: WARN when not installed (the machine then syncs only on `kitbag sync`; fix `kitbag service install`); WARN when unloaded, inactive, disabled, stale, the last supervisor exit code is nonzero (except 75, the daemon's restart after its executable was replaced), or the executable the definition runs no longer exists (for example after a package upgrade). Shows supervisor run/exit observations; run `kitbag service refresh` when the executable is missing, otherwise reinstall with `kitbag service install`, and inspect `kitbag service status`. |
| Daemon version | An eligible older daemon is automatically refreshed through the saved installation. While recovery is pending, report that the daemon is restarting on the new version, usually within a few minutes. After the bounded window or when recovery is unavailable, warn and offer repair diagnostics. Matching versions never trigger recovery. |
| Legacy units | Detect filenames containing `agent-config` under XDG `systemd/user` and `~/Library/LaunchAgents`. Definitions of the host's own manager (systemd units on Linux, `<label>.plist` on macOS, label from the filename) are inspected read-only with `platform.UnitInspect` (`systemctl --user show`, `launchctl print`/`print-disabled`; nothing is loaded, started or changed): a unit neither loaded nor enabled is counted as ignored, anything else WARNs with `loaded=`/`enabled=`. A booted-out LaunchAgent that is not disabled still WARNs, as launchd loads it again at login. Other definitions and failed inspections WARN by presence. Disable and unload old timers/services manually. |

Legacy checks skip when `migration.legacy_roots` is empty. They only detect;
no link, instruction, setting, timer, or service is changed; the only commands
they run are the read-only unit inspections above. Agent settings
are read in exactly one place: the `legacy-settings-markers` byte search above,
through the platform filesystem, only for Claude instances' `settings.json`.
Nothing is parsed or interpreted, no settings content is ever displayed, stored
or put in `--json`/`--report` output, and credentials files are never opened.
HTTPS credential-source discovery and API socket checks belong to later milestones.

Every network/process check has a two-second context deadline. Agent probes run
serially with only PATH and `LC_ALL=C`, without HOME, agent settings overrides or
credential variables, in the executable volume root. System Git retains its
existing explicit environment and user Git configuration. Remote failures do
not copy helper output/URLs into diagnostics. Arbitrary installed programs and
Git helpers remain trusted executables; their behavior is outside this policy.

## Exit codes

The [shared exit table](apply.md#reports-and-exit-codes) applies: all OK or only
ordinary WARN results exit **0**; any FAIL exits **1**; `--strict` also maps WARN
to **1**. A failed `min_tool_version` or `manifest-schema` check (the manifest
needs a newer tool) exits **4** before further probes, state/cache locks or
`--report` writes.
Invalid flags/configuration exit **2** where classified as usage; operational
setup/output/report-write failures exit **1**. Windows still exits **5** before
running checks. Doctor never uses conflict exit 3.

## Local issue reports

```sh
kitbag doctor --report ./kitbag-doctor.json
kitbag doctor --strict --json --report ./kitbag-doctor.json
```

`--report <path>` writes schema-1 JSON locally with mode **0600**, through the
platform atomic writer. The parent must exist. It performs **no remote probe**;
that check explicitly says skipped. No telemetry, upload, network request, or
sync is performed by report generation. Standard output follows the usual text
or `--json` format; only the saved file is redacted. Review it before attaching.
A write failure fails the command. An existing file or link at the report path
is replaced atomically; its link target is not followed.

Reports contain only diagnostic fields, never file contents (the settings marker
check contributes only a path and a count), an environment snapshot, Git identity
values, signing key values, or agent configuration.
Before JSON encoding, the redactor masks URL userinfo, common GitHub/OpenAI/
GitLab/Slack token patterns, named token/API-key/password/authorization/secret
assignments and email addresses. The current home prefix becomes `~`, as do
other users' `/home/<user>`, `/Users/<user>` and Windows `C:\Users\<user>`
prefixes. This is defense in depth for diagnostic text, not a scanner for
arbitrary unknown secret formats. Do not add raw configuration/helper dumps to
new checks. SSH provenance uses only an identity filename, never its directories,
key material, key comments or passphrase. Failed SSH probes use fixed diagnostics
and actionable agent/IdentityFile/known_hosts hints. Embedded doctor always
disables interaction, even when a caller supplied a prompter; `--report` skips it
entirely. Unit and end-to-end tests seed secrets/emails/home paths and assert
that they reach unredacted input and cannot occur in the saved report.

After the minimum-version gate passes, doctor may create kitbag's private state
directory/lock and cache directory to
verify availability. It never writes state.json or last-sync.json; successful
probes remove their scratch. A lock failure does not attempt to break the lock.
Configuration must be parseable before checks can run; repair a configuration
parse error using its normal CLI diagnostic first.

## Extending checks

Policy lives in `internal/core/doctor.Service`, with small registry methods over
injected platform FS/Env, state store, backend resolution and version probes.
Core does not import `os` or `os/exec`. Real agent probes use the existing
`system.Runner`; Git-specific reads remain in `vcs/system`. Optional capabilities
avoid widening the base backend contract while KIT-22 adds embedded Git.
Add a testscript failure/warning with its fix hint for every new check, plus
schema and exit-code coverage when the public contract changes.

An installed, active daemon also has a `daemon-poll` check. It reads the bounded
`last-check.json` snapshot and warns when its age exceeds three recorded poll
intervals or the record cannot be read. It does not contact the remote. Missing
records are reported as not recorded yet (including older daemon versions).
Timer services and stopped services do not get this check.

The `daemon-version` check reads the existing local API `version.get` method,
including for older API v1 daemons. A running process shows **daemon X, binary Y**;
a mismatch is WARN with `kitbag service refresh` as its fix. No listening daemon
adds no version check; authentication/protocol failures warn without assuming a
version. `--report` still permits this local observation while skipping remote
network probes. Doctor never installs, reloads or restarts the service.
