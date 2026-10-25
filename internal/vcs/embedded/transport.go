package embedded

import (
	"bufio"
	"context"
	"errors"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"golang.org/x/crypto/ssh/agent"

	"github.com/TomaszJanusz/kitbag/internal/vcs"
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
	switch ep.Protocol {
	case "file":
		return nil, noop, nil
	case "https":
		if ep.User != "" || ep.Password != "" {
			return nil, noop, unsupported("HTTPS credentials (public HTTPS only)")
		}
		return nil, noop, nil
	case "ssh":
		return b.sshAuth(ctx, ep)
	default:
		return nil, noop, unsupported("transport " + ep.Protocol)
	}
}

func (b *Backend) sshAuth(ctx context.Context, ep *transport.Endpoint) (transport.AuthMethod, func() error, error) {
	noop := func() error { return nil }
	if b.opts.Env == nil {
		return nil, noop, vcs.ErrInvalidArgument
	}
	home, err := b.opts.Env.HomeDir()
	if err != nil {
		return nil, noop, err
	}
	paths := []string{filepath.Join(home, ".ssh", "config")}
	if !b.opts.NoSystemConfig {
		paths = append(paths, "/etc/ssh/ssh_config")
	}
	for _, path := range paths {
		if err = checkSSHConfig(path); err != nil {
			return nil, noop, err
		}
	}
	callback, err := gitssh.NewKnownHostsCallback(filepath.Join(home, ".ssh", "known_hosts"))
	if err != nil {
		return nil, noop, err
	}
	socket, _ := b.opts.Env.LookupEnv("SSH_AUTH_SOCK")
	if socket == "" {
		return nil, noop, unsupported("SSH requires SSH_AUTH_SOCK (agent authentication only)")
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", socket)
	if err != nil {
		return nil, noop, err
	}
	user := ep.User
	if user == "" {
		user = "git"
	}
	auth := &gitssh.PublicKeysCallback{User: user, Callback: agent.NewClient(conn).Signers, HostKeyCallbackHelper: gitssh.HostKeyCallbackHelper{HostKeyCallback: callback}}
	return auth, conn.Close, nil
}

func checkSSHConfig(path string) error {
	data, err := os.ReadFile(path) //nolint:gosec // G304: explicit system and HOME SSH config paths, never a remote-supplied filename.
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		fields := strings.Fields(strings.ReplaceAll(scanner.Text(), "=", " "))
		if len(fields) == 0 {
			continue
		}
		switch strings.ToLower(fields[0]) {
		case "proxyjump":
			return unsupported("SSH ProxyJump")
		case "proxycommand":
			return unsupported("SSH ProxyCommand")
		case "gssapiauthentication":
			return unsupported("Kerberos authentication")
		case "include":
			return unsupported("SSH config includes")
		}
	}
	return scanner.Err()
}
