package manifestupgrade_test

import (
	"bytes"
	"embed"
	"errors"
	"io/fs"
	"regexp"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/discover"
	"github.com/TomaszJanusz/kitbag/internal/core/manifestupgrade"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// testdata holds schema-1 inputs and reviewed upgrade goldens; a golden
// changes only by hand, after reading its diff.
//
//go:embed testdata/*.toml
var testdata embed.FS

type env map[string]string

func (e env) LookupEnv(key string) (string, bool) { value, ok := e[key]; return value, ok }

func kinds() discover.Table {
	return discover.Table{
		{Agent: "claude", Variable: "CLAUDE_CONFIG_DIR", DefaultDir: "~/.claude"},
		{Agent: "codex", Variable: "CODEX_HOME", DefaultDir: "~/.codex"},
		{Agent: "generic"},
	}
}

func prepare(t *testing.T, data []byte, variables env) (manifestupgrade.Plan, error) {
	t.Helper()
	return manifestupgrade.Prepare(manifestupgrade.Input{Env: variables, Filename: "kitbag.toml", HostName: "mac", Manifest: data, Kinds: kinds()})
}

func read(t *testing.T, name string) []byte {
	t.Helper()
	data, err := testdata.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	if want := read(t, name); !bytes.Equal(got, want) {
		t.Fatalf("%s differs:\ngot\n%s\nwant\n%s", name, got, want)
	}
}

// TestUpgradeGoldens upgrades the maintainer's commented layout and a file
// with schema-1 host replacement sets.
func TestUpgradeGoldens(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"agent-config", "host-instances", "comments"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			input := read(t, name+".toml")
			plan, err := prepare(t, input, nil)
			if err != nil {
				t.Fatal(err)
			}
			golden(t, name+".upgraded.toml", plan.After)
			assertLossless(t, input, plan.After)
			assertPlan(t, plan)
		})
	}
}

func assertPlan(t *testing.T, plan manifestupgrade.Plan) {
	t.Helper()
	if plan.Current || plan.SchemaBefore != 1 || plan.MinToolVersion != manifestupgrade.FirstSchema2Release || plan.Title != manifestupgrade.Title {
		t.Fatalf("plan %+v", plan)
	}
	if !strings.Contains(plan.Message, "\n\nKitbag-Host: mac\nKitbag-Op: manifest.upgrade\n") || !strings.Contains(strings.Join(plan.Warnings, "\n"), "before this change is published") {
		t.Fatalf("message %q warnings %q", plan.Message, plan.Warnings)
	}
	if m, err := manifest.Parse("kitbag.toml", plan.After); err != nil || m.Schema != 2 {
		t.Fatalf("upgraded manifest: %v", err)
	}
}

var commentLine = regexp.MustCompile(`#.*`)

// assertLossless requires every comment of the input in the output, as often
// as before; a moved table takes its comments along, so order may change.
func assertLossless(t *testing.T, before, after []byte) {
	t.Helper()
	count := map[string]int{}
	for _, comment := range commentLine.FindAll(before, -1) {
		count[string(bytes.TrimSpace(comment))]++
	}
	for _, comment := range commentLine.FindAll(after, -1) {
		count[string(bytes.TrimSpace(comment))]--
	}
	for comment, missing := range count {
		if missing != 0 {
			t.Fatalf("comment %q: %d missing:\n%s", comment, missing, after)
		}
	}
}

func TestUpgradeKeepsBytesOutsideRootValues(t *testing.T) {
	t.Parallel()
	input := read(t, "agent-config.toml")
	plan, err := prepare(t, input, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := bytes.Replace(input, []byte("schema = 1\nmin_tool_version = \"0.1.0\""), []byte("schema = 2\nmin_tool_version = \""+manifestupgrade.FirstSchema2Release+"\""), 1)
	if !bytes.Equal(plan.After, want) {
		t.Fatalf("changed more than schema and min_tool_version:\n%s", plan.After)
	}
	if len(plan.Diff) == 0 || len(plan.Conversions) != 2 {
		t.Fatalf("diff %q conversions %q", plan.Diff, plan.Conversions)
	}
}

func TestUpgradeMinToolVersion(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ body, want string }{
		{"schema = 1\n", "schema = 1\n"},
		{"schema = 1\nmin_tool_version = \"9.0.0\"\n", "schema = 2\nmin_tool_version = \"9.0.0\"\n"},
		{"# head\nschema = 1 # keep\n\n[publish]\nmode = \"direct\"\n", ""},
	} {
		plan, err := prepare(t, []byte(tc.body), nil)
		if err != nil {
			t.Fatal(err)
		}
		switch {
		case tc.want == "schema = 1\n":
			if string(plan.After) != "schema = 2\nmin_tool_version = \""+manifestupgrade.FirstSchema2Release+"\"\n" {
				t.Fatalf("added minimum: %q", plan.After)
			}
		case tc.want != "":
			if string(plan.After) != tc.want || plan.MinToolVersion != "9.0.0" {
				t.Fatalf("kept higher minimum: %q", plan.After)
			}
		default:
			if !bytes.HasPrefix(plan.After, []byte("# head\nschema = 2 # keep\nmin_tool_version = ")) {
				t.Fatalf("comment layout: %q", plan.After)
			}
		}
	}
}

func TestUpgradeCurrentIsNoOp(t *testing.T) {
	t.Parallel()
	plan, err := prepare(t, []byte("schema = 2\n"), nil)
	if err != nil || !plan.Current || plan.After != nil || len(plan.Edits) != 0 {
		t.Fatalf("schema 2: %+v %v", plan, err)
	}
}

func TestUpgradeDeclaredDefaultWarnsAboutRelocation(t *testing.T) {
	t.Parallel()
	body := "schema = 1\n[targets.claude]\nagent = \"claude\"\n[hosts.mac.targets.claude.instances.work]\ndir = \"~/.claude-work\"\n"
	plan, err := prepare(t, []byte(body), env{"CLAUDE_CONFIG_DIR": "/srv/claude"})
	if err != nil {
		t.Fatal(err)
	}
	m, err := manifest.Parse("kitbag.toml", plan.After)
	if err != nil {
		t.Fatal(err)
	}
	instances := m.Targets["claude"].Instances
	if instances["main"].Dir != "~/.claude" || strings.Join(instances["work"].Hosts, ",") != "mac" || m.Hosts["mac"].Targets["claude"].Instances["main"].Enabled == nil {
		t.Fatalf("converted: %+v %+v", instances, m.Hosts)
	}
	if !strings.Contains(strings.Join(plan.Warnings, "\n"), "CLAUDE_CONFIG_DIR is set here (/srv/claude)") {
		t.Fatalf("warnings %q", plan.Warnings)
	}
}

func TestUpgradeRefusals(t *testing.T) {
	t.Parallel()
	head := "schema = 1\n[targets.claude]\nagent = \"claude\"\n[targets.claude.instances.main]\ndir = \"~/.claude\"\n"
	for _, tc := range []struct{ name, body, want string }{
		{"different dir under one name", head + "[hosts.mac.targets.claude.instances.main]\ndir = \"~/.claude-mac\"\n", "different dir, roots or primitives"},
		{"two names for one directory", head + "[hosts.mac.targets.claude.instances.other]\ndir = \"~/.claude\"\n", "name the same directory ~/.claude"},
		{"host-only instance defined twice", head + "[hosts.mac.targets.claude.instances.w]\ndir = \"~/w\"\n[hosts.ci.targets.claude.instances.w]\ndir = \"~/w\"\nmode = \"copy\"\n", "define instance w differently"},
		{"mode a patch cannot unset", "schema = 1\n[targets.claude]\nagent = \"claude\"\n[targets.claude.instances.main]\ndir = \"~/.claude\"\nmode = \"copy\"\n[hosts.mac.targets.claude.instances.main]\ndir = \"~/.claude\"\n", "which a host patch cannot express"},
		{"inline host instances", head + "[hosts.mac.targets.claude]\ninstances = { main = { dir = \"~/.claude\" } }\n", "cannot edit safely"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := prepare(t, []byte(tc.body), nil)
			if !errors.Is(err, manifestupgrade.ErrRefused) || !errors.Is(err, fs.ErrInvalid) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %v, want refusal %q", err, tc.want)
			}
		})
	}
}
