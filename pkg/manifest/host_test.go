package manifest_test

import (
	"errors"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

func hostManifest(t *testing.T) *manifest.Manifest {
	t.Helper()
	m, err := manifest.Parse("kitbag.toml", []byte(`schema=1
mode='link'
[instructions]
source='AGENTS.md'
hosts='hosts/{host}.md'
[publish]
mode='pr'
branch='main'
[[files]]
src='a'
dest='~/a'
hosts=['mac']
[[files]]
src='b'
dest='~/b'
[targets.claude]
agent='claude'
mode='link'
[targets.claude.instances.main]
dir='~/.claude'
[targets.claude.instances.other]
dir='~/.claude_2'
[hosts.mac]
mode='copy'
publish={branch='host-branch'}
[hosts.mac.targets.claude]
enabled=false
mode='copy'
[hosts.mac.targets.claude.instances.only]
dir='~/.claude_3'
`))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestHostResolution(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, system, configured, override, want string
		files                                    int
	}{
		{"short hostname", "mac.example.test", "", "", "mac", 2},
		{"config", "other.example.test", "mac", "", "mac", 2},
		{"environment", "other", "other", "mac", "mac", 2},
		{"unknown host", "other.example.test", "", "", "other", 1},
		{"explicit dotted name", "other", "mac.example", "", "mac.example", 1},
		{"empty override", "mac.example.test", "", "", "mac", 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := hostManifest(t)
			env := fake.New(fake.WithHostname(tc.system), fake.WithEnv(map[string]string{"KITBAG_HOST": tc.override}))
			got, err := m.ResolveHost(env, tc.configured)
			if err != nil {
				t.Fatal(err)
			}
			if got.Instructions.Hosts != "hosts/"+tc.want+".md" || len(got.Files) != tc.files {
				t.Fatalf("wrong host selection: %+v", got)
			}
			if tc.want == "mac" {
				assertHostOverride(t, got)
			}
			got.Targets["mutated"] = manifest.Target{Agent: "codex"}
			got.Instructions.Source = "mutated"
			if len(m.Targets) != 1 || m.Instructions.Source != "AGENTS.md" || len(m.Files) != 2 {
				t.Fatal("resolution mutated input")
			}
		})
	}
}

func assertHostOverride(t *testing.T, m *manifest.Manifest) {
	t.Helper()
	target := m.Targets["claude"]
	if m.Mode != "copy" || target.Mode != "copy" || target.Enabled == nil || *target.Enabled {
		t.Fatal("lost host policy")
	}
	if len(target.Instances) != 1 || target.Instances["only"].Dir != "~/.claude_3" {
		t.Fatal("instances were not replaced")
	}
	if m.Publish.Mode != "" || m.Publish.Branch != "host-branch" {
		t.Fatal("publish did not replace both keys")
	}
}

func TestHostFailures(t *testing.T) {
	t.Parallel()
	for _, host := range []string{"bad/host", "../mac", "CON", "", "-bad"} {
		t.Run(host, func(t *testing.T) {
			t.Parallel()
			h := fake.New(fake.WithHostname(host))
			_, err := hostManifest(t).ResolveHost(h, "")
			if err == nil {
				t.Fatal("accepted invalid detected host")
			}
		})
	}
}

func TestVersionCheck(t *testing.T) {
	t.Parallel()
	cases := []struct {
		required, current string
		old, invalid      bool
	}{
		{"1.2.3", "1.2.3", false, false},
		{"1.2.3", "1.2.4", false, false},
		{"1.2.3", "1.2.2", true, false},
		{"1.2.3", "1.2.3-rc.1", true, false},
		{"1.2.3-rc.1", "1.2.3", false, false},
		{"1.2.3-rc.10", "1.2.3-rc.2", true, false},
		{"1.2.3-alpha.1", "1.2.3-alpha", true, false},
		{"1.2.3-beta", "1.2.3-123", true, false},
		{"1.2.3-123", "1.2.3-beta", false, false},
		{"1.2.3+one", "1.2.3+two", false, false},
		{"1.2.3", "1.10.0", false, false},
		{"2.0.0", "1.99.99", true, false},
		{"1.2.3", "dev", false, true},
		{"", "0.0.0", false, false},
		{"9999999999999999999999.0.0", "9999999999999999999998.0.0", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.required+"/"+tc.current, func(t *testing.T) {
			t.Parallel()
			m := &manifest.Manifest{MinToolVersion: tc.required}
			err := m.CheckVersion(tc.current)
			var tooOld *manifest.VersionError
			if errors.As(err, &tooOld) != tc.old || (err != nil) != (tc.old || tc.invalid) {
				t.Fatalf("CheckVersion = %v", err)
			}
			if tc.old && (tooOld.Current != tc.current || tooOld.Required != tc.required) {
				t.Fatal("typed error lost versions")
			}
		})
	}
}
