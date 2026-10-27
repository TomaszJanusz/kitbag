package manifest_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

const schema2Body = `schema = 2
[targets.claude]
agent = "claude"
[targets.claude.instances.main]
dir = "~/.claude"
[targets.claude.instances.second]
dir = "~/.claude_2"
hosts = ["mac"]
[targets.codex]
agent = "codex"
[targets.codex.instances.work]
dir = "~/.codex-work"
hosts = ["runner02"]
[hosts.mac.targets.claude.instances.main]
enabled = false
mode = "copy"
[hosts.mac.targets.claude.instances.second]
settings = false
`

func resolve(t *testing.T, m *manifest.Manifest, host string) *manifest.Manifest {
	t.Helper()
	resolved, err := m.ResolveHost(fake.New(fake.WithEnv(map[string]string{"KITBAG_HOST": host})), "")
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func TestSchema2HostsFilterAndPatches(t *testing.T) {
	t.Parallel()
	m, err := manifest.Parse("kitbag.toml", []byte(schema2Body))
	if err != nil {
		t.Fatal(err)
	}
	assertMacResolution(t, resolve(t, m, "mac").Targets)
	other := resolve(t, m, "other").Targets
	if main := other["claude"].Instances["main"]; main.Enabled != nil || main.Mode != "" {
		t.Fatalf("patch leaked to another host: %+v", main)
	}
	if second := other["claude"].Instances["second"]; !off(second.Enabled) {
		t.Fatalf("second active outside its hosts: %+v", second)
	}
	if work := resolve(t, m, "runner02").Targets["codex"].Instances["work"]; work.Enabled != nil {
		t.Fatalf("work disabled on its own host: %+v", work)
	}
	original := m.Targets["claude"].Instances
	if original["main"].Enabled != nil || original["second"].Enabled != nil || original["second"].Settings != nil {
		t.Fatalf("resolution mutated input: %+v", original)
	}
}

func assertMacResolution(t *testing.T, mac map[string]manifest.Target) {
	t.Helper()
	main, second := mac["claude"].Instances["main"], mac["claude"].Instances["second"]
	if !off(main.Enabled) || main.Mode != "copy" || main.Dir != "~/.claude" {
		t.Fatalf("mac main not patched: %+v", main)
	}
	if second.Enabled != nil || !off(second.Settings) {
		t.Fatalf("mac second not active and patched: %+v", second)
	}
	// A target whose only instance excludes the host keeps it, disabled, so
	// the adapter never falls back to the implicit default directory.
	if len(mac["codex"].Instances) != 1 || !off(mac["codex"].Instances["work"].Enabled) {
		t.Fatalf("codex on mac: %+v", mac["codex"].Instances)
	}
}

// off reports an explicit false switch.
func off(value *bool) bool { return value != nil && !*value }

func TestSchema1HostTablesStillReplace(t *testing.T) {
	t.Parallel()
	head, _, _ := strings.Cut(schema2Body, "[hosts.")
	body := strings.Replace(head, "schema = 2", "schema = 1", 1)
	if _, err := manifest.Parse("kitbag.toml", []byte(body)); err == nil || !strings.Contains(err.Error(), "/targets/claude/instances/second/hosts: 'hosts' on an instance needs manifest schema 2") {
		t.Fatalf("schema 1 accepted instance hosts: %v", err)
	}
	// The same host tables are replacement sets in schema 1: they need a dir.
	body = "schema = 1\n[targets.claude]\nagent = \"claude\"\n[targets.claude.instances.main]\ndir = \"~/.claude\"\n[hosts.mac.targets.claude.instances.main]\nenabled = false\n"
	if _, err := manifest.Parse("kitbag.toml", []byte(body)); err == nil || !strings.Contains(err.Error(), "/hosts/mac/targets/claude/instances/main: missing property 'dir'") {
		t.Fatalf("schema-1 host table read as a patch: %v", err)
	}
	m, err := manifest.Parse("kitbag.toml", []byte(body+"dir = \"~/.claude-mac\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := resolve(t, m, "mac").Targets["claude"].Instances; len(got) != 1 || got["main"].Dir != "~/.claude-mac" {
		t.Fatalf("schema-1 host set not replaced: %+v", got)
	}
}

func TestSchema2Relations(t *testing.T) {
	t.Parallel()
	head := "schema = 2\n[targets.claude]\nagent = \"claude\"\n[targets.claude.instances.main]\ndir = \"~/.claude\"\nhosts = [\"mac\"]\n"
	for _, tc := range []struct{ name, body, key, want string }{
		{"patch names undeclared instance", head + "[hosts.mac.targets.claude.instances.other]\nenabled = false\n", "/hosts/mac/targets/claude/instances/other", "does not declare"},
		{"patch on excluded host", head + "[hosts.ci.targets.claude.instances.main]\nenabled = false\n", "/hosts/ci/targets/claude/instances/main", "hosts list (mac) excludes"},
		{"patch on undeclared target", head + "[hosts.mac.targets.codex.instances.main]\nenabled = false\n", "/hosts/mac/targets/codex", "undeclared target"},
		{"identity collision across hosts", head + "[targets.claude.instances.copy]\ndir = \"~/.claude\"\nhosts = [\"ci\"]\n", "/targets/claude/instances/main", "instance directory collides"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := manifest.Parse("kitbag.toml", []byte(tc.body))
			var positioned *manifest.Error
			if !errors.As(err, &positioned) || positioned.Key != tc.key || positioned.Line < 2 || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %v, want %s at %s", err, tc.want, tc.key)
			}
		})
	}
}

func TestInstanceAppliesTo(t *testing.T) {
	t.Parallel()
	if !(manifest.Instance{}).AppliesTo("any") {
		t.Fatal("unfiltered instance excluded")
	}
	limited := manifest.Instance{Hosts: []string{"mac", "ci"}}
	if !limited.AppliesTo("ci") || limited.AppliesTo("Mac") || limited.AppliesTo("runner02") {
		t.Fatal("hosts filter mismatch")
	}
}
