// Package system implements vcs.Backend with the git executable installed on
// the machine, version 2.30 or newer, so the user's credential helpers, SSH
// configuration, signing and hooks keep working (ADR 4).
//
// Every Git process gets a complete, fixed environment instead of kitbag's
// own: HOME from the platform, PATH, XDG_CONFIG_HOME, SSH_AUTH_SOCK and
// TMPDIR when set, LC_ALL=C, GIT_TERMINAL_PROMPT=0 and editors and pagers
// that never wait for input. No GIT_* variable is inherited, and
// GIT_CEILING_DIRECTORIES confines repository discovery to the working copy.
// Output is read in Git's machine-readable formats (porcelain v2, -z NUL
// separation), so paths with spaces, newlines or any Unicode are exact.
//
// Processes start through a [Runner]; [ExecRunner] is the only use of os/exec
// outside internal/platform, which lint enforces.
package system
