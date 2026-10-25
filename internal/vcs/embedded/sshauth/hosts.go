package sshauth

import (
	"errors"
	"io/fs"
	"net"
	"strconv"

	"github.com/skeema/knownhosts"
	"golang.org/x/crypto/ssh"
)

// hostCallback verifies against known_hosts and returns the host key algorithms
// recorded there, so a server offering several key types negotiates the trusted one.
func (a *Auth) hostCallback() (ssh.HostKeyCallback, []string, error) {
	var existing []string
	for _, path := range a.Target.KnownHosts {
		_, err := a.opts.FS.Stat(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, nil, errors.New("cannot read known_hosts; check UserKnownHostsFile permissions")
		}
		existing = append(existing, path)
	}
	db, err := knownhosts.NewDB(existing...)
	if err != nil {
		return nil, nil, errors.New("cannot parse known_hosts; repair UserKnownHostsFile")
	}
	verify := db.HostKeyCallback()
	algorithms := db.HostKeyAlgorithms(net.JoinHostPort(a.Target.Host, strconv.Itoa(a.Target.Port)))
	return func(host string, addr net.Addr, key ssh.PublicKey) error {
		err := verify(host, addr, key)
		if err == nil {
			return nil
		}
		if !knownhosts.IsHostUnknown(err) {
			return fail(ErrHostChanged, "SSH host key changed or revoked; verify the server key independently and repair known_hosts")
		}
		return a.trustHost(host, key)
	}, algorithms, nil
}

func (a *Auth) trustHost(host string, key ssh.PublicKey) error {
	if !a.opts.Interactive || a.opts.Prompter == nil {
		return fail(ErrHostUnknown, "unknown SSH host key ("+ssh.FingerprintSHA256(key)+"); verify it and add it to known_hosts before non-interactive use")
	}
	consent, err := a.opts.Prompter.TrustHost(a.ctx, host, ssh.FingerprintSHA256(key))
	if err != nil || !consent {
		return fail(ErrHostUnknown, "unknown SSH host key rejected; verify it before adding to known_hosts")
	}
	// Consent applies to this connection only. Persistent known_hosts management
	// remains with OpenSSH; doctor and ordinary probes never modify trust files.
	return nil
}
