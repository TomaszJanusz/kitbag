package embedded

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

func TestSSHRemoteDiagnosisAlias(t *testing.T) {
	t.Parallel()
	w := sshAliasWorld(t, true)

	diagnosis, err := w.b.DiagnoseRemote(t.Context(), w.clone)
	must(t, err)
	if diagnosis.Source != "identity file configured_key" || diagnosis.Fix != "" {
		t.Fatalf("diagnosis=%+v", diagnosis)
	}
	if strings.Contains(diagnosis.Source, w.home) {
		t.Fatal("home leaked", diagnosis)
	}
}

func sshFixtureKey(t *testing.T) (ssh.Signer, []byte) {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	must(t, err)
	signer, err := ssh.NewSignerFromKey(key)
	must(t, err)
	block, err := ssh.MarshalPrivateKey(key, "")
	must(t, err)
	return signer, pem.EncodeToMemory(block)
}

func sshRefServer(t *testing.T, host, identity ssh.Signer) string {
	t.Helper()
	cfg := &ssh.ServerConfig{PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		if conn.User() != "configured-user" || !bytes.Equal(key.Marshal(), identity.PublicKey().Marshal()) {
			return nil, errors.New("denied")
		}
		return &ssh.Permissions{}, nil
	}}
	cfg.AddHostKey(host)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	must(t, err)
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		serveEmptyRefs(t, conn, cfg)
	}()
	t.Cleanup(func() { must(t, listener.Close()); <-done })
	return listener.Addr().String()
}

func serveEmptyRefs(t *testing.T, conn net.Conn, cfg *ssh.ServerConfig) {
	t.Helper()
	defer func() {
		if err := conn.Close(); err != nil {
			return
		}
	}()
	const timeout = 5 * time.Second
	must(t, conn.SetDeadline(time.Now().Add(timeout)))
	server, channels, requests, err := ssh.NewServerConn(conn, cfg)
	if err != nil {
		// Rejections are asserted by the client; e.g. an unknown host key.
		return
	}
	defer func() {
		if closeErr := server.Close(); closeErr != nil {
			return
		}
	}()
	go ssh.DiscardRequests(requests)
	for request := range channels {
		channel, incoming, acceptErr := request.Accept()
		if acceptErr != nil {
			t.Error(acceptErr)
			return
		}
		for message := range incoming {
			if message.Type != "exec" {
				must(t, message.Reply(false, nil))
				continue
			}
			checkUploadCommand(t, message.Payload)
			must(t, message.Reply(true, nil))
			must(t, packp.NewAdvRefs().Encode(channel))
			_, err = io.Copy(io.Discard, channel)
			must(t, err)
			if closeErr := channel.Close(); closeErr != nil && !errors.Is(closeErr, io.EOF) {
				t.Error(closeErr)
			}

			return
		}
	}
}

func checkUploadCommand(t *testing.T, payload []byte) {
	t.Helper()
	if !bytes.Contains(payload, []byte("git-upload-pack")) {
		t.Errorf("unexpected exec %q", payload)
	}
}

func sshAliasWorld(t *testing.T, trusted bool) testWorld {
	t.Helper()
	w := setup(t)
	signer, key := sshFixtureKey(t)
	host, _ := sshFixtureKey(t)
	address := sshRefServer(t, host, signer)
	hostname, port, err := net.SplitHostPort(address)
	must(t, err)
	sshDir := filepath.Join(w.home, ".ssh")
	must(t, os.Mkdir(sshDir, 0o700))
	must(t, os.WriteFile(filepath.Join(sshDir, "configured_key"), key, 0o600))
	if trusted {
		put(t, filepath.Join(sshDir, "known_hosts"), knownhosts.Line([]string{knownhosts.Normalize(address)}, host.PublicKey())+"\n")
	}
	put(t, filepath.Join(sshDir, "config"), "Host fixture-alias\n HostName "+hostname+"\n User configured-user\n Port "+port+"\n IdentityAgent none\n IdentityFile ~/.ssh/configured_key\n IdentitiesOnly yes\n")
	repo, err := git.PlainOpen(w.clone)
	must(t, err)
	must(t, repo.DeleteRemote(vcs.DefaultRemote))
	_, err = repo.CreateRemote(&config.RemoteConfig{Name: vcs.DefaultRemote, URLs: []string{"ssh://fixture-alias/repo"}})
	must(t, err)
	return w
}

func TestSSHFetchAlias(t *testing.T) {
	t.Parallel()
	w := sshAliasWorld(t, true)
	must(t, w.b.Fetch(t.Context(), w.clone))
}

func TestSSHRemoteDiagnosisUnknownHost(t *testing.T) {
	t.Parallel()
	w := sshAliasWorld(t, false)
	diagnosis, err := w.b.DiagnoseRemote(t.Context(), w.clone)
	if err == nil || diagnosis.Source != "" {
		t.Fatalf("unknown host authenticated: %+v", diagnosis)
	}
	if !strings.Contains(diagnosis.Fix, "known_hosts") || !strings.Contains(err.Error(), "SHA256:") {
		t.Fatalf("diagnosis=%+v, error=%v", diagnosis, err)
	}
}
