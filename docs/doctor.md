# Diagnosing synchronization

Run `kitbag doctor` to explain synchronization prerequisites without applying a
plan. Results are grouped **OK**, **WARN**, **FAIL**; each warning/failure has a
one-line `fix:` hint. Checks continue after failures when their prerequisites
remain available. Text groups and JSON registry order are deterministic.

`kitbag doctor --json` emits `doctor_schema: 1`, validated against
[`doctor.schema.json`](../schemas/doctor.schema.json). Every check has `id`,
`status`, `message`, and `fix` (empty for OK). Skipped checks are OK and explicitly
say why they were skipped. An unavailable prerequisite omits its dependent
checks; it does not assert that they passed.

| Check | Evidence and fix |
| --- | --- |
| Manifest/host | Load the local `kitbag.toml` and resolve the selected host; correct the working-copy path, manifest or host. |
| Tool version | Compare the running semantic version with `min_tool_version`; upgrade kitbag. Development builds use `0.0.0-dev`. |
| Git backend | Requested/active backend, version and selection source from `vcs.Open`; install suitable Git or select an available backend. An automatic fallback warns with its reason. |
| Remote | Read origin refs through the backend's optional `vcs.RemoteProber`; check URL, connectivity and authentication. No fetch, ref update, or commit. Embedded SSH reports the successful credential source (agent or identity filename only); unsupported backends warn. |
| SSH agent | `SSH_AUTH_SOCK` must name an existing socket; start an agent/export its socket. Missing is WARN, including for non-SSH remotes where it may be irrelevant. No key listing. |
| Commit identity | System Git reads effective `user.name` and `user.email`; set missing keys. Values are never returned. |
| Commit signing | System Git reads `commit.gpgsign` and presence of `user.signingkey`; configure a key or explicitly disable signing. Signing enabled with an implicit default key warns conservatively. Key availability, passphrases, and a successful signature are not verified. |
| Agents | Probe the implemented native agents, `claude --version` and `codex --version`, through an injected process runner. Missing binaries, failed probes and timeouts WARN; install/check PATH. Generic agents have no assumed binary. |
| Instances | Check every enabled, host-resolved listed instance and native default directory. Explicit dirs override agent home variables. Missing/non-directory instances WARN; create/correct the directory. A missing default is allowed to be created by sync. |
| Copy/link | Create unpredictable scratch names in the kitbag cache; verify atomic bytes and a symbolic link, then remove them. Repair cache permissions/filesystem support or use copy mode. Link failure WARNs when machine mode is copy, FAILs otherwise. This tests the cache filesystem, not all destination mounts or native agent loading. |
| State | Read and validate state and last-sync schemas through the state store while holding its nonblocking lock. Release immediately; never save/recover state. Restore a valid backup/use a compatible tool for corrupt or newer data. Wait for an active sync or correct lock/directory permissions. A leftover unlocked lock file is healthy. |
| Legacy links | With `migration.legacy_roots`, inspect shared/instance skills, instructions, old executable and agent-browser links into those roots or `~/.agents/skills`; review them, then run `kitbag adopt` ([adopt.md](adopt.md)). Dangling links count. |
| Legacy markers | Inspect instruction files for `agent-config-managed` or `Managed by agent-config-sync`; review/migrate them. Agent settings/credentials are never opened, so hook markers inside settings are deliberately not inspected. |
| Legacy units | Detect filenames containing `agent-config` under XDG `systemd/user` and `~/Library/LaunchAgents`; review/disable old timers/services manually. Presence does not prove activation. No service manager is invoked. |

Legacy checks skip when `migration.legacy_roots` is empty. They only detect;
no link, instruction, setting, timer, or service is changed. Service health,
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
to **1**. A failed `min_tool_version` check exits **4** before further probes,
state/cache locks or `--report` writes.
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

Reports contain only diagnostic fields, never file contents, an environment
snapshot, Git identity values, signing key values, or agent configuration.
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
