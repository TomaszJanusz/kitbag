package sshauth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
)

type testPrompt struct {
	pass           string
	fingerprint    string
	calls          int
	consent, store bool
}

// Passphrase supplies an explicit test response and storage consent.
func (p *testPrompt) Passphrase(context.Context, string) ([]byte, bool, error) {
	p.calls++
	return []byte(p.pass), p.store, nil
}

// TrustHost records the fingerprint presented to the user.
func (p *testPrompt) TrustHost(_ context.Context, _ string, fingerprint string) (bool, error) {
	p.calls++
	p.fingerprint = fingerprint
	return p.consent, nil
}

type testRing struct {
	saved []byte
	puts  int
}

// Get supplies a copy so the caller can clear sensitive bytes.
func (r *testRing) Get(context.Context, string) ([]byte, error) {
	if r.saved == nil {
		return nil, errors.New("absent")
	}
	return append([]byte{}, r.saved...), nil
}

// Put copies a passphrase into a test-only vault.
func (r *testRing) Put(_ context.Context, _ string, pass []byte) error {
	r.puts++
	r.saved = append([]byte{}, pass...)
	return nil
}

func TestKeyAuthentication(t *testing.T) {
	t.Parallel()
	for _, pass := range []string{"", "fixture passphrase"} {
		t.Run(pass, func(t *testing.T) {
			t.Parallel()
			host, key := newKey(t), newKey(t)
			addr := listenServer(t, host, key)
			dir := t.TempDir()
			path := filepath.Join(dir, "identity")
			key.write(t, path, pass)
			prompt := &testPrompt{pass: pass}
			ring := &testRing{}
			target := Target{User: "git-user", Identities: []string{path}, KnownHosts: []string{knownFile(t, dir, addr, host, true)}}
			auth, err := New(t.Context(), target, Options{FS: fixtureFS{}, Prompter: prompt, Keyring: ring, Interactive: true})
			must(t, err)
			must(t, authenticate(t, addr, auth))
			if auth.Source() != "identity file identity" || ring.puts != 0 {
				t.Fatalf("source/storage = %q/%d", auth.Source(), ring.puts)
			}
			wantCalls := 0
			if pass != "" {
				wantCalls = 1
			}
			if prompt.calls != wantCalls {
				t.Fatalf("prompt calls=%d", prompt.calls)
			}
		})
	}
}

func TestAgentAuthentication(t *testing.T) {
	t.Parallel()
	host, key := newKey(t), newKey(t)
	addr := listenServer(t, host, key)
	dir := t.TempDir()
	socket := startAgent(t, key)
	target := Target{User: "git-user", Agent: socket, KnownHosts: []string{knownFile(t, dir, addr, host, false)}}
	auth, err := New(t.Context(), target, Options{FS: fixtureFS{}})
	must(t, err)
	must(t, authenticate(t, addr, auth))
	if auth.Source() != "SSH agent" {
		t.Fatal("agent not used", auth.Source())
	}
}

func TestIdentitiesOnly(t *testing.T) {
	t.Parallel()
	host, key := newKey(t), newKey(t)
	addr := listenServer(t, host, key)
	dir := t.TempDir()
	target := Target{User: "git-user", Agent: startAgent(t, key), IdentitiesOnly: true, KnownHosts: []string{knownFile(t, dir, addr, host, false)}}
	auth, err := New(t.Context(), target, Options{FS: fixtureFS{}})
	must(t, err)
	if err = authenticate(t, addr, auth); err == nil {
		t.Fatal("IdentitiesOnly offered an unconfigured agent key")
	}
	// A configured public key allows the agent to sign without a private file.
	path := filepath.Join(dir, "configured")
	key.write(t, path, "")
	target.Identities = []string{path}
	allowed, err := New(t.Context(), target, Options{FS: fixtureFS{}})
	must(t, err)
	must(t, authenticate(t, addr, allowed))
	if allowed.Source() != "SSH agent" {
		t.Fatal("configured agent key ignored")
	}
}

type hostCase struct {
	name                                       string
	known, changed, interactive, consent, want bool
}

func TestHostVerification(t *testing.T) {
	t.Parallel()
	for _, tc := range []hostCase{
		{name: "unknown noninteractive"},
		{name: "unknown rejected", interactive: true},
		{name: "unknown consent", interactive: true, consent: true, want: true},
		{name: "changed never prompts", known: true, changed: true, interactive: true, consent: true},
		{name: "known", known: true, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			host, key := newKey(t), newKey(t)
			addr := listenServer(t, host, key)
			dir := t.TempDir()
			path := filepath.Join(dir, "key")
			key.write(t, path, "")
			known := filepath.Join(dir, "absent_hosts")
			if tc.known {
				trusted := host
				if tc.changed {
					trusted = newKey(t)
				}
				known = knownFile(t, dir, addr, trusted, false)
			}
			prompt := &testPrompt{consent: tc.consent}
			target := Target{User: "git-user", Identities: []string{path}, KnownHosts: []string{known}}
			auth, err := New(t.Context(), target, Options{FS: fixtureFS{}, Interactive: tc.interactive, Prompter: prompt})
			must(t, err)
			tc.check(t, authenticate(t, addr, auth), prompt)
		})
	}
}

func (tc hostCase) check(t *testing.T, err error, prompt *testPrompt) {
	t.Helper()
	if (err == nil) != tc.want {
		t.Fatalf("authenticated=%v, error=%v", err == nil, err)
	}
	kind := ErrHostUnknown
	if tc.changed {
		kind = ErrHostChanged
	}
	if !tc.want && !errors.Is(err, kind) {
		t.Fatalf("error=%v, want %v", err, kind)
	}
	wantPrompt := tc.interactive && !tc.known
	if (prompt.calls > 0) != wantPrompt {
		t.Fatalf("prompts=%d", prompt.calls)
	}
	if wantPrompt && !strings.HasPrefix(prompt.fingerprint, "SHA256:") {
		t.Fatal("fingerprint not shown", prompt.fingerprint)
	}
}

func TestPassphraseConsent(t *testing.T) {
	t.Parallel()
	for _, consent := range []bool{false, true} {
		t.Run(map[bool]string{false: "decline", true: "consent"}[consent], func(t *testing.T) {
			t.Parallel()
			host, key := newKey(t), newKey(t)
			addr := listenServer(t, host, key)
			dir := t.TempDir()
			path := filepath.Join(dir, "encrypted")
			key.write(t, path, "fixture passphrase")
			prompt := &testPrompt{pass: "fixture passphrase", store: consent}
			ring := &testRing{}
			target := Target{User: "git-user", Identities: []string{path}, KnownHosts: []string{knownFile(t, dir, addr, host, false)}}
			auth, err := New(t.Context(), target, Options{FS: fixtureFS{}, Prompter: prompt, Keyring: ring, Interactive: true})
			must(t, err)
			must(t, authenticate(t, addr, auth))
			if (ring.puts > 0) != consent {
				t.Fatalf("stored without consent: %d", ring.puts)
			}
		})
	}
}

func TestNonInteractivePassphrase(t *testing.T) {
	t.Parallel()
	host, key := newKey(t), newKey(t)
	addr := listenServer(t, host, key)
	dir := t.TempDir()
	path := filepath.Join(dir, "encrypted")
	key.write(t, path, "fixture passphrase")
	prompt := &testPrompt{pass: "fixture passphrase", store: true}
	target := Target{User: "git-user", Identities: []string{path}, KnownHosts: []string{knownFile(t, dir, addr, host, false)}}
	auth, err := New(t.Context(), target, Options{FS: fixtureFS{}, Prompter: prompt})
	must(t, err)
	err = authenticate(t, addr, auth)
	if !errors.Is(err, ErrLocked) || !strings.Contains(err.Error(), "unlock it in an SSH agent") || prompt.calls != 0 {
		t.Fatalf("error=%v, prompts=%d", err, prompt.calls)
	}
}

func TestAliasAuthentication(t *testing.T) {
	t.Parallel()
	host, key := newKey(t), newKey(t)
	addr := listenServer(t, host, key)
	dir := t.TempDir()
	path := filepath.Join(dir, "alias_key")
	key.write(t, path, "")
	known := knownFile(t, dir, addr, host, false)
	hostname, port, err := net.SplitHostPort(addr)
	must(t, err)
	h := fake.New(fake.WithEnv(map[string]string{"HOME": dir, "SSH_AUTH_SOCK": "/missing/agent"}))
	config := filepath.Join(dir, ".ssh", "config")
	must(t, h.MkdirAll(filepath.Dir(config), 0o700))
	must(t, h.WriteAtomic(config, []byte("Host alias\n HostName "+hostname+"\n User git-user\n Port "+port+"\n IdentityAgent none\n IdentityFile "+path+"\n IdentitiesOnly yes\n UserKnownHostsFile "+known+"\n"), 0o600))
	target, err := Resolve(h, h, "alias", "", 0, false)
	must(t, err)
	auth, err := New(t.Context(), target, Options{FS: fixtureFS{}})
	must(t, err)
	must(t, authenticate(t, addr, auth))
	if target.Agent != "" || target.Host != hostname {
		t.Fatalf("alias resolution=%+v", target)
	}
}

func TestAgentOrderAndFallback(t *testing.T) {
	t.Parallel()
	for _, fallback := range []bool{false, true} {
		t.Run(map[bool]string{false: "agent first", true: "file fallback"}[fallback], func(t *testing.T) {
			t.Parallel()
			host, key := newKey(t), newKey(t)
			addr := listenServer(t, host, key)
			agentKey := key
			if fallback {
				agentKey = newKey(t)
			}
			dir := t.TempDir()
			path := filepath.Join(dir, "encrypted")
			key.write(t, path, "fixture passphrase")
			prompt := &testPrompt{pass: "fixture passphrase"}
			target := Target{User: "git-user", Agent: startAgent(t, agentKey), Identities: []string{path}, KnownHosts: []string{knownFile(t, dir, addr, host, false)}}
			auth, err := New(t.Context(), target, Options{FS: fixtureFS{}, Prompter: prompt, Interactive: true})
			must(t, err)
			must(t, authenticate(t, addr, auth))
			if (prompt.calls > 0) != fallback {
				t.Fatalf("agent ordering: prompt calls=%d", prompt.calls)
			}
		})
	}
}

func TestSavedPassphrase(t *testing.T) {
	t.Parallel()
	host, key := newKey(t), newKey(t)
	addr := listenServer(t, host, key)
	dir := t.TempDir()
	path := filepath.Join(dir, "encrypted")
	key.write(t, path, "fixture passphrase")
	ring := &testRing{saved: []byte("fixture passphrase")}
	prompt := &testPrompt{}
	target := Target{User: "git-user", Identities: []string{path}, KnownHosts: []string{knownFile(t, dir, addr, host, false)}}
	auth, err := New(t.Context(), target, Options{FS: fixtureFS{}, Prompter: prompt, Keyring: ring})
	must(t, err)
	must(t, authenticate(t, addr, auth))
	if prompt.calls != 0 || string(ring.saved) != "fixture passphrase" {
		t.Fatal("keyring retrieval prompted or corrupted storage")
	}
}

// TestKnownHostKeyAlgorithm offers a preferred ECDSA host key alongside the
// trusted Ed25519 key, as GitHub does; negotiation must pick the trusted type.
func TestKnownHostKeyAlgorithm(t *testing.T) {
	t.Parallel()
	host, key := newKey(t), newKey(t)
	preferred, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	must(t, err)
	untrusted, err := ssh.NewSignerFromKey(preferred)
	must(t, err)
	addr := listenServer(t, host, key, untrusted)
	dir := t.TempDir()
	path := filepath.Join(dir, "key")
	key.write(t, path, "")
	hostname, port, err := net.SplitHostPort(addr)
	must(t, err)
	portNumber, err := strconv.Atoi(port)
	must(t, err)
	target := Target{Host: hostname, Port: portNumber, User: "git-user", Identities: []string{path}, KnownHosts: []string{knownFile(t, dir, addr, host, true)}}
	auth, err := New(t.Context(), target, Options{FS: fixtureFS{}})
	must(t, err)
	must(t, authenticate(t, addr, auth))
}
