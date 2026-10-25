package sshauth

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

func TestResolve(t *testing.T) {
	t.Parallel()
	home := filepath.FromSlash("/ssh-home")
	h := fake.New(fake.WithEnv(map[string]string{"HOME": home, "SSH_AUTH_SOCK": "/run/default-agent"}))
	must(t, h.MkdirAll(filepath.Join(home, ".ssh"), 0o700))
	must(t, h.WriteAtomic(filepath.Join(home, ".ssh", "config"), []byte(`Host work-* !work-excluded
 HostName git.example.invalid
 User alice
 Port 2200
 IdentityFile ~/.ssh/first
 IdentityFile ~/.ssh/second
 IdentityAgent ~/.ssh/agent
 UserKnownHostsFile ~/.ssh/custom_hosts
 IdentitiesOnly yes
Host *
 User fallback
`), 0o600))
	target, err := Resolve(h, h, "work-repo", "", 0, false)
	must(t, err)
	assertConnection(t, target, "git.example.invalid", "alice", 2200)
	if !target.IdentitiesOnly {
		t.Fatal("IdentitiesOnly ignored")
	}

	want := []string{filepath.Join(home, ".ssh", "first"), filepath.Join(home, ".ssh", "second")}
	if !reflect.DeepEqual(target.Identities, want) || target.Agent != filepath.Join(home, ".ssh", "agent") || target.KnownHosts[0] != filepath.Join(home, ".ssh", "custom_hosts") {
		t.Fatalf("paths = %+v", target)
	}
	explicit, err := Resolve(h, h, "work-repo", "bob", 22, false)
	must(t, err)
	assertConnection(t, explicit, "git.example.invalid", "bob", 22)

	excluded, err := Resolve(h, h, "work-excluded", "", 0, false)
	must(t, err)
	assertConnection(t, excluded, "work-excluded", "fallback", 22)
	if excluded.Agent != "/run/default-agent" {
		t.Fatal("default agent ignored")
	}
}

func TestUnsupportedConfig(t *testing.T) {
	t.Parallel()
	for _, keyword := range []string{"ProxyJump", "ProxyCommand", "Match", "Include", "GSSAPIAuthentication"} {
		t.Run(keyword, func(t *testing.T) {
			t.Parallel()
			err := CheckConfig([]byte("Host unrelated\n " + keyword + "=example\n"))
			var typed *vcs.UnsupportedError
			if !errors.As(err, &typed) || !errors.Is(err, vcs.ErrUnsupported) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func assertConnection(t *testing.T, target Target, host, user string, port int) {
	t.Helper()
	if target.Host != host || target.User != user || target.Port != port {
		t.Fatalf("connection = %+v", target)
	}
}

func TestConfigurationPaths(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, config string
		identities   int
		wantErr      bool
	}{
		{name: "defaults", identities: 3},
		{name: "configured without defaults", config: "IdentityFile ~/.ssh/custom\n", identities: 1},
		{name: "configured only, IdentitiesOnly unset", config: "IdentityFile ~/.ssh/a\nIdentityFile ~/.ssh/b\n", identities: 2},
		{name: "only configured", config: "IdentityFile ~/.ssh/custom\nIdentitiesOnly yes\n", identities: 1},
		{name: "quoted hosts", config: "UserKnownHostsFile \"~/.ssh/hosts one\" ~/.ssh/hosts_two\n", identities: 3},
		{name: "literal percent", config: "IdentityFile ~/.ssh/%%key\nIdentitiesOnly yes\n", identities: 1},
		{name: "bad port", config: "Port invalid\n", wantErr: true},
		{name: "out of range port", config: "Port 65536\n", wantErr: true},
		{name: "unsupported path expansion", config: "IdentityFile ~somebody/key\n", wantErr: true},
		{name: "verification disabled", config: "UserKnownHostsFile none\n", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			home := filepath.FromSlash("/ssh-home")
			h := fake.New(fake.WithEnv(map[string]string{"HOME": home}))
			must(t, h.MkdirAll(filepath.Join(home, ".ssh"), 0o700))
			must(t, h.WriteAtomic(filepath.Join(home, ".ssh", "config"), []byte(tc.config), 0o600))
			target, err := Resolve(h, h, "example.invalid", "", 0, false)
			if tc.wantErr {
				if err == nil {
					t.Fatal("invalid config accepted")
				}
				return
			}
			must(t, err)
			if len(target.Identities) != tc.identities {
				t.Fatalf("identities = %v", target.Identities)
			}
			if tc.name == "quoted hosts" && !reflect.DeepEqual(target.KnownHosts, []string{filepath.Join(home, ".ssh", "hosts one"), filepath.Join(home, ".ssh", "hosts_two")}) {
				t.Fatalf("known hosts = %v", target.KnownHosts)
			}
		})
	}
}
