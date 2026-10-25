package sshauth

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

// Prompter is implemented by interactive renderers. Calls must honor ctx.
// A nil prompter always rejects requests, including in daemon contexts.
type Prompter interface {
	Passphrase(ctx context.Context, identity string) (passphrase []byte, store bool, err error)
	TrustHost(ctx context.Context, host, fingerprint string) (bool, error)
}

// Keyring keeps passphrases outside configuration files. Put is called only
// after successful decryption and explicit storage consent from Prompter.
type Keyring interface {
	Get(ctx context.Context, identity string) ([]byte, error)
	Put(ctx context.Context, identity string, passphrase []byte) error
}

// Options inject the host and optional interaction/credential services.
type Options struct {
	FS        platform.FS
	Prompter  Prompter
	Keyring   Keyring
	DialAgent func(context.Context, string) (net.Conn, error)
	ParseKey  func([]byte, []byte) (ssh.Signer, error)
	// Interactive must also be true; supplying a prompter alone never enables it.
	Interactive bool
}

// Auth is one operation's credential chain; it must not be shared across operations.
type Auth struct {
	ctx     context.Context
	lastErr error
	opts    Options
	source  string
	closers []io.Closer
	stop    []func() bool
	Target  Target
	next    int
}

// New constructs a lazy chain: agent, configured identities, then default keys.
func New(ctx context.Context, target Target, opts Options) (*Auth, error) {
	if opts.FS == nil {
		return nil, vcs.ErrInvalidArgument
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if opts.DialAgent == nil {
		opts.DialAgent = dialAgent
	}
	if opts.ParseKey == nil {
		opts.ParseKey = parseKey
	}
	a := &Auth{Target: target, opts: opts, ctx: ctx, next: -1}
	return a, nil
}

// Name identifies public-key authentication to go-git.
func (*Auth) Name() string { return "ssh-public-keys" }

// String intentionally includes no user, host, paths or credentials.
func (*Auth) String() string { return "embedded SSH credentials" }

// Source identifies the signer used by a completed authentication.
func (a *Auth) Source() string { return a.source }

// Close releases agent sockets even on a failed operation.
func (a *Auth) Close() error {
	for _, stop := range a.stop {
		stop()
	}
	var err error
	for _, closer := range a.closers {
		if closeErr := closer.Close(); closeErr != nil && !errors.Is(closeErr, net.ErrClosed) {
			err = errors.Join(err, closeErr)
		}
	}
	return err
}

// ClientConfig returns the verified public-key chain. The finite number of
// stages advances credentials; it never repeats a failed credential or prompt.
func (a *Auth) ClientConfig() (*ssh.ClientConfig, error) {
	verify, algorithms, err := a.hostCallback()
	if err != nil {
		return nil, err
	}
	return &ssh.ClientConfig{
		User: a.Target.User, HostKeyCallback: verify, HostKeyAlgorithms: algorithms, Timeout: connectionTimeout,
		Auth: []ssh.AuthMethod{ssh.RetryableAuthMethod(ssh.PublicKeysCallback(a.signers), len(a.Target.Identities)+1)},
	}, nil
}

func (a *Auth) signers() ([]ssh.Signer, error) {
	if err := a.ctx.Err(); err != nil {
		return nil, err
	}
	if a.next < 0 {
		a.next = 0
		if signers := a.agentSigners(); len(signers) > 0 {
			a.source = "SSH agent"
			return signers, nil
		}
	}
	for a.next < len(a.Target.Identities) {
		path := a.Target.Identities[a.next]
		a.next++
		signer, err := a.fileSigner(path)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				a.lastErr = err
			}
			continue
		}
		a.source = a.Target.label(path)
		return []ssh.Signer{signer}, nil
	}
	if a.lastErr != nil {
		return nil, a.lastErr
	}
	return nil, fail(ErrNoCredentials, "no usable SSH credentials; start an SSH agent or configure IdentityFile")
}

func (a *Auth) agentSigners() []ssh.Signer {
	if a.Target.Agent == "" {
		return nil
	}
	conn, err := a.opts.DialAgent(a.ctx, a.Target.Agent)
	if err != nil {
		return nil
	}
	a.track(conn)
	if err = a.deadline(conn); err != nil {
		return nil
	}

	signers, err := agent.NewClient(conn).Signers()
	if err != nil {
		return nil
	}
	if a.Target.IdentitiesOnly {
		return a.filterAgent(signers)
	}
	return signers
}

func dialAgent(ctx context.Context, path string) (net.Conn, error) {
	return (&net.Dialer{Timeout: connectionTimeout}).DialContext(ctx, "unix", path)
}
