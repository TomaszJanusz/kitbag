package manifest_test

import (
	"errors"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

const instanceHosts = `schema=1
[targets.claude]
agent='claude'
[targets.claude.instances.main]
dir='~/.claude'
[targets.claude.instances.second]
dir='~/.claude_2'
hosts=['mac', 'ci']
[targets.codex]
agent='codex'
[targets.codex.instances.work]
dir='~/.codex-work'
hosts=['mac']
`

// TestInstanceHostsResolution proves that an instance applies only to its
// hosts and that elsewhere it stays listed but disabled, so the target never
// falls back to its implicit default directory.
func TestInstanceHostsResolution(t *testing.T) {
	t.Parallel()
	m, err := manifest.Parse("kitbag.toml", []byte(instanceHosts))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		host         string
		second, work bool
	}{{"mac", true, true}, {"ci", true, false}, {"runner02", false, false}} {
		t.Run(tc.host, func(t *testing.T) {
			t.Parallel()
			got, resolveErr := m.ResolveHost(fake.New(fake.WithEnv(map[string]string{"KITBAG_HOST": tc.host})), "")
			if resolveErr != nil {
				t.Fatal(resolveErr)
			}
			assertActive(t, got, tc.second, tc.work)
		})
	}
	if second := m.Targets["claude"].Instances["second"]; second.Enabled != nil || !second.AppliesTo("ci") || second.AppliesTo("other") {
		t.Fatalf("resolution mutated input or AppliesTo is wrong: %+v", second)
	}
	if !m.Targets["claude"].Instances["main"].AppliesTo("anything") {
		t.Fatal("an instance without hosts must apply everywhere")
	}
}

func assertActive(t *testing.T, m *manifest.Manifest, second, work bool) {
	t.Helper()
	claude, codex := m.Targets["claude"].Instances, m.Targets["codex"].Instances
	if len(claude) != 2 || len(codex) != 1 || claude["main"].Enabled != nil {
		t.Fatalf("instances: %+v %+v", claude, codex)
	}
	if active(claude["second"]) != second || active(codex["work"]) != work {
		t.Fatalf("second %+v work %+v", claude["second"], codex["work"])
	}
}

// TestInstanceHostsKeepExplicitDisable proves that a listed host does not
// re-enable an instance switched off by hand.
func TestInstanceHostsKeepExplicitDisable(t *testing.T) {
	t.Parallel()
	m, err := manifest.Parse("kitbag.toml", []byte("schema=1\n[targets.claude]\nagent='claude'\n[targets.claude.instances.main]\ndir='~/.claude'\nhosts=['mac']\nenabled=false\n"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := m.ResolveHost(fake.New(fake.WithEnv(map[string]string{"KITBAG_HOST": "mac"})), "")
	if err != nil {
		t.Fatal(err)
	}
	if active(got.Targets["claude"].Instances["main"]) {
		t.Fatal("hosts re-enabled a disabled instance")
	}
}

// TestInstanceHostsRules covers the hosts grammar and its interplay with a
// host's replacement instance set.
func TestInstanceHostsRules(t *testing.T) {
	t.Parallel()
	const target = "schema=1\n[targets.claude]\nagent='claude'\n[targets.claude.instances.main]\ndir='~/.claude'\n"
	for _, tc := range []struct{ name, body, want string }{
		{"valid", target + "hosts=['mac', 'Runner-02.lan']", ""},
		{"empty", target + "hosts=[]", "minItems"},
		{"duplicate", target + "hosts=['mac', 'mac']", "items at 0 and 1 are equal"},
		{"wrong type", target + "hosts='mac'", "want array"},
		{"wrong item type", target + "hosts=[1]", "want string"},
		{"bad name", target + "hosts=['../mac']", "does not match pattern"},
		{"in a host set", "schema=1\n[targets.claude]\nagent='claude'\n[hosts.mac.targets.claude.instances.main]\ndir='~/.claude'\nhosts=['mac']", "'hosts' is not allowed in a host's instance set"},
		{"host set with hosts", target + "hosts=['mac']\n[hosts.ci.targets.claude.instances.main]\ndir='~/.claude'", "/hosts/ci/targets/claude/instances: host instance set for targets.claude, whose instance main lists its hosts"},
		{"host set on another target", target + "hosts=['mac']\n[targets.codex]\nagent='codex'\n[hosts.ci.targets.codex.instances.main]\ndir='~/.codex'", ""},
		{"host target policy with hosts", target + "hosts=['mac']\n[hosts.mac.targets.claude]\nenabled=false\nmode='copy'", ""},
		{"collision across disjoint hosts", target + "hosts=['mac']\n[targets.claude.instances.other]\ndir='~/.claude'\nhosts=['ci']", "instance directory collides"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, err := manifest.Parse("kitbag.toml", []byte(tc.body))
			assertParseResult(t, m, err, tc.want)
			var diagnostic *manifest.Error
			if tc.want != "" && (!errors.As(err, &diagnostic) || diagnostic.Line < 1) {
				t.Fatalf("missing position: %v", err)
			}
		})
	}
}

func active(i manifest.Instance) bool { return i.Enabled == nil || *i.Enabled }
