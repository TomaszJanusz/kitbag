# Embedded SSH credentials

KIT-32 adds discovery to the embedded backend. System Git still uses its own SSH
implementation. The resolver under `internal/vcs/embedded/sshauth` reads config
and keys through injected platform FS/Env contracts; agent dialing, key parsing,
known_hosts verification, prompting and keyring access are replaceable inputs.
Core has no socket, filesystem or SSH-library dependency.

## Resolution and authentication

URL user and explicit URL port override SSH settings. Otherwise settings come
from `~/.ssh/config`, then `/etc/ssh/ssh_config`, then defaults (`git`, port 22).
`Host` wildcard/negated patterns and first-value semantics apply to supported
settings: `HostName`, `User`, `Port`, `IdentityAgent`, `IdentitiesOnly`,
`UserKnownHostsFile`. Every matching `IdentityFile` is tried in declaration order.
Paths support `~`, `%d`, `%h`, `%r`, `%p` and `%%`; relative paths are home-relative.
Unsupported expansions fail explicitly. There is no shell evaluation.

Authentication advances through these sources without repeating credentials:

1. The socket in `IdentityAgent`, or `SSH_AUTH_SOCK` when unspecified.
   `IdentityAgent none` disables agent access. Agents using the OpenSSH protocol,
   including 1Password, Secretive and gpg-agent, use the same injected dialer.
2. Configured identity files.
3. `~/.ssh/id_ed25519`, `~/.ssh/id_ecdsa`, `~/.ssh/id_rsa`, only when ssh_config
   supplies no `IdentityFile` for the host (as in OpenSSH).

When any `IdentityFile` applies, the default key files are never tried, with or
without `IdentitiesOnly`. With `IdentitiesOnly yes`, agent keys must match a
configured identity (its `.pub` file, private key public component, or encrypted
OpenSSH public header); without it every agent key is offered. Missing or
unusable agents fall through to files. Encrypted files are decrypted only after
earlier sources fail, so a working agent never triggers a passphrase prompt.

`ProxyJump`, `ProxyCommand`, `Match`, `Include` and `GSSAPIAuthentication` are
conservatively rejected even in unmatched blocks. The typed unsupported error
retains the established hint: **install git or resolve in the working copy**.
Includes are rejected before parsing can open another file or execute anything.

## Passphrases and host trust

`Options.Interactive` and an injected `Prompter` are both required for interaction.
Neither CLI nor daemon currently supplies one. Non-interactive encrypted-key
failures explain how to unlock the identity in an SSH agent; they never wait for
terminal input. Doctor forces non-interactive behavior even on a UI-owned backend.

The optional keyring is queried before prompting. A successful decryption stores
the passphrase only when the prompter explicitly returns storage consent. Failed
passphrases are never stored. Returned byte buffers are cleared after use;
keyring implementations must return copies and copy values they retain.
No passphrase is written to a config file or known_hosts.

A real OS keyring and CLI/TUI renderers are deferred: wiring them requires
platform-specific availability/error policy and terminal lifecycle handling.
This ticket defines and tests their consent contract without adding an unused
native dependency or changing daemon interactivity.

`UserKnownHostsFile` selects trust files (default `~/.ssh/known_hosts`); quoted
paths and multiple files work. Verification uses `skeema/knownhosts` over
`x/crypto/ssh/knownhosts`, with hashed hosts, certificate and revoked-key handling
provided by those libraries. The host key algorithms recorded for the host are
requested during negotiation, so a server that also offers an untrusted key type
(GitHub offers ECDSA before Ed25519) is verified against the trusted type instead
of being reported as changed.
Missing trust files count as an unknown host, never as trusted. Unknown hosts
present their SHA256 fingerprint to the prompter; accepting trusts **that
connection only**. The resolver never edits known_hosts, so doctor remains
read-only. Persistent trust must be added independently with OpenSSH after
verification. Changed or revoked keys are hard failures and never prompt.
`UserKnownHostsFile none` is rejected. Lint forbids the SSH verification bypass.

Doctor reports a source only after SSH authentication and a successful origin
ref advertisement: `SSH agent` or `identity file <filename>`. It never includes
identity directories, private bytes, passphrases, key comments or raw transport
errors. Its SSH connection observes the two-second probe deadline throughout
connect, handshake and exec. Agent operations also have a finite deadline.

## Verification scope

Local in-process `x/crypto/ssh` servers exercise agent signing through a temporary
Unix socket, plain/encrypted files, saved passphrases, storage consent, agent/file
ordering, `IdentitiesOnly`, aliases and user/port overrides, hashed known_hosts,
negotiation of the trusted host key type among several offered,
unknown-host rejection/consent, changed-key rejection and non-interactive failure.
A backend diagnostic test executes a `git-upload-pack` advertisement stub. A
stalled-handshake test verifies diagnostic cancellation.

No additional SSH-server module is required: `x/crypto v0.57.0` was already a
direct dependency. `kevinburke/ssh_config v1.2.0` is promoted from indirect to
direct at its existing exact pin, as is go-git's `skeema/knownhosts v1.3.1`
(host key algorithm selection). Depguard confines SSH libraries/config parsing
to the embedded subtree; core receives only safe VCS diagnostic values.

These tests do not transfer real Git packs over SSH, launch an OpenSSH server,
exercise hardware/password-manager agents or validate a native keyring/terminal
renderer. Existing Git contract tests cover object transfer separately. Windows
is cross-compiled, with runtime support still deferred to M6.
