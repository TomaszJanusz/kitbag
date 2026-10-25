package sshauth

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/TomaszJanusz/kitbag/internal/platform"
)

// fixtureFS exposes only test-created files; the policy never reads process HOME.
type fixtureFS struct{ platform.FS }

// ReadFile reads a named fixture.
func (fixtureFS) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) } //nolint:gosec // G304: paths name test-created fixtures only.

// Stat reads named fixture metadata.
func (fixtureFS) Stat(path string) (fs.FileInfo, error) { return os.Stat(path) }

type testKey struct {
	signer  ssh.Signer
	private ed25519.PrivateKey
}

func newKey(t *testing.T) testKey {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	must(t, err)
	signer, err := ssh.NewSignerFromKey(private)
	must(t, err)
	return testKey{private: private, signer: signer}
}

func (key testKey) write(t *testing.T, path, pass string) {
	t.Helper()
	block, err := ssh.MarshalPrivateKey(key.private, "")
	must(t, err)
	if pass != "" {
		block, err = ssh.MarshalPrivateKeyWithPassphrase(key.private, "", []byte(pass))
		must(t, err)
	}
	must(t, os.WriteFile(path, pem.EncodeToMemory(block), 0o600))
}

func listenServer(t *testing.T, host, testIdentity testKey, extraHostKeys ...ssh.Signer) string {
	t.Helper()
	config := &ssh.ServerConfig{PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		if conn.User() != "git-user" || !bytes.Equal(key.Marshal(), testIdentity.signer.PublicKey().Marshal()) {
			return nil, errors.New("denied")
		}
		return &ssh.Permissions{}, nil
	}}
	config.AddHostKey(host.signer)
	for _, extra := range extraHostKeys {
		config.AddHostKey(extra)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	must(t, err)
	var workers sync.WaitGroup
	workers.Go(func() {
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			workers.Go(func() { serveSSH(conn, config) })
		}
	})
	t.Cleanup(func() { must(t, listener.Close()); workers.Wait() })
	return listener.Addr().String()
}

func serveSSH(conn net.Conn, config *ssh.ServerConfig) {
	defer func() {
		if err := conn.Close(); err != nil {
			return
		}
	}()
	// Only a hang guard: bcrypt key decryption takes seconds under -race on a busy host.
	const timeout = 60 * time.Second
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return
	}
	server, chans, requests, err := ssh.NewServerConn(conn, config)
	if err != nil {
		return
	}
	defer func() {
		if closeErr := server.Close(); closeErr != nil {
			return
		}
	}()
	go ssh.DiscardRequests(requests)
	for channel := range chans {
		if err = channel.Reject(ssh.Prohibited, "test authenticates only"); err != nil {
			return
		}
	}
}

func knownFile(t *testing.T, dir, addr string, key testKey, hashed bool) string {
	t.Helper()
	host := knownhosts.Normalize(addr)
	if hashed {
		host = knownhosts.HashHostname(host)
	}
	path := filepath.Join(dir, "known_hosts")
	must(t, os.WriteFile(path, []byte(knownhosts.Line([]string{host}, key.signer.PublicKey())+"\n"), 0o600))
	return path
}

func startAgent(t *testing.T, key testKey) string {
	t.Helper()
	// Keep paths below macOS's 104-byte sun_path limit; /var may be a symlink.
	dir, err := os.MkdirTemp("/tmp", "kitbag-ssh-")
	must(t, err)
	dir, err = filepath.EvalSymlinks(dir)
	must(t, err)
	path := filepath.Join(dir, "a")
	listener, err := net.Listen("unix", path)
	must(t, err)
	ring := agent.NewKeyring()
	must(t, ring.Add(agent.AddedKey{PrivateKey: key.private}))
	var workers sync.WaitGroup
	workers.Go(func() {
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			workers.Go(func() {
				defer func() {
					if closeErr := conn.Close(); closeErr != nil {
						return
					}
				}()
				if serveErr := agent.ServeAgent(ring, conn); serveErr != nil {
					return
				}
			})
		}
	})
	t.Cleanup(func() { must(t, listener.Close()); workers.Wait(); must(t, os.RemoveAll(dir)) })
	return path
}

func authenticate(t *testing.T, addr string, auth *Auth) error {
	t.Helper()
	defer func() { must(t, auth.Close()) }()
	config, err := auth.ClientConfig()
	if err != nil {
		return err
	}
	conn, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", addr)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := conn.Close(); closeErr != nil && !errors.Is(closeErr, net.ErrClosed) {
			t.Error(closeErr)
		}
	}()
	// Only a hang guard, as in serveSSH.
	const timeout = 60 * time.Second
	must(t, conn.SetDeadline(time.Now().Add(timeout)))
	client, _, _, err := ssh.NewClientConn(conn, addr, config)
	if err == nil {
		must(t, client.Close())
	}
	return err
}
