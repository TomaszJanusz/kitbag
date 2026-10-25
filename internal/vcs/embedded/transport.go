package embedded

import (
	"context"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"

	"github.com/TomaszJanusz/kitbag/internal/vcs/embedded/sshauth"
)

func (b *Backend) auth(ctx context.Context, r *git.Repository, url string) (transport.AuthMethod, func() error, error) {
	noop := func() error { return nil }
	configs, err := b.configs(r)
	if err != nil {
		return nil, noop, err
	}
	if err = checkConfigs(configs, false); err != nil {
		return nil, noop, err
	}
	ep, err := transport.NewEndpoint(url)
	if err != nil {
		return nil, noop, err
	}
	return b.protocolAuth(ctx, ep, url)
}

func (b *Backend) protocolAuth(ctx context.Context, ep *transport.Endpoint, url string) (transport.AuthMethod, func() error, error) {
	noop := func() error { return nil }
	switch ep.Protocol {
	case "file":
		return nil, noop, nil
	case "https":
		if ep.User != "" || ep.Password != "" {
			return nil, noop, unsupported("HTTPS credentials (public HTTPS only)")
		}
		return nil, noop, nil
	case "ssh":
		if !strings.Contains(url, "://") && ep.Port == gitssh.DefaultPort {
			ep.Port = 0
		}
		return b.sshAuth(ctx, ep)
	default:
		return nil, noop, unsupported("transport " + ep.Protocol)
	}
}

func (b *Backend) sshAuth(ctx context.Context, ep *transport.Endpoint) (transport.AuthMethod, func() error, error) {
	noop := func() error { return nil }

	target, err := sshauth.Resolve(b.opts.Env, b.opts.SSH.FS, ep.Host, ep.User, ep.Port, !b.opts.NoSystemConfig)
	if err != nil {
		return nil, noop, err
	}
	auth, err := sshauth.New(ctx, target, b.opts.SSH)
	if err != nil {
		return nil, noop, err
	}
	return auth, auth.Close, nil
}

type sshTransport struct{ transport.Transport }

func resolvedEndpoint(ep *transport.Endpoint, auth transport.AuthMethod) (*transport.Endpoint, error) {
	credentials, ok := auth.(*sshauth.Auth)
	if !ok {
		return nil, transport.ErrInvalidAuthMethod
	}
	target := *ep
	target.Host, target.User, target.Port = credentials.Target.Host, credentials.Target.User, credentials.Target.Port
	return &target, nil
}

// NewUploadPackSession applies the already resolved SSH endpoint.
func (t sshTransport) NewUploadPackSession(ep *transport.Endpoint, auth transport.AuthMethod) (transport.UploadPackSession, error) {
	target, err := resolvedEndpoint(ep, auth)
	if err != nil {
		return nil, err
	}
	return t.Transport.NewUploadPackSession(target, auth)
}

// NewReceivePackSession applies the already resolved SSH endpoint.
func (t sshTransport) NewReceivePackSession(ep *transport.Endpoint, auth transport.AuthMethod) (transport.ReceivePackSession, error) {
	target, err := resolvedEndpoint(ep, auth)
	if err != nil {
		return nil, err
	}
	return t.Transport.NewReceivePackSession(target, auth)
}
