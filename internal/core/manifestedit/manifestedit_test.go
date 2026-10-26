package manifestedit_test

import (
	"embed"
	"errors"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/manifestedit"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// testdata holds the maintainer's agent-config layout and reviewed goldens;
// a golden changes only by hand, after reading its diff.
//
//go:embed testdata/*.toml
var testdata embed.FS

func off() *bool { value := false; return &value }
func on() *bool  { value := true; return &value }

// TestGolden edits the maintainer's commented agent-config layout; every byte
// outside the edited block must stay as it was.
func TestGolden(t *testing.T) {
	t.Parallel()
	cases := map[string][]manifestedit.Edit{
		"add":               {{Op: manifestedit.AddInstance, Target: "claude", Name: "third", Dir: "~/.claude_3", Settings: off()}},
		"edit":              {{Op: manifestedit.EditInstance, Target: "claude", Name: "second", Dir: "~/.claude-work", Settings: off()}},
		"remove":            {{Op: manifestedit.RemoveInstance, Target: "claude", Name: "second"}},
		"add-target":        {{Op: manifestedit.AddTarget, Target: "gemini", Agent: "gemini"}, {Op: manifestedit.AddInstance, Target: "gemini", Name: "main", Dir: "~/.gemini"}},
		"remove-last-codex": {{Op: manifestedit.RemoveInstance, Target: "codex", Name: "main"}},
	}
	input := read(t, "agent-config.toml")
	for name, edits := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := manifestedit.Apply("kitbag.toml", input, edits...)
			if err != nil {
				t.Fatal(err)
			}
			if want := read(t, "agent-config."+name+".toml"); string(got) != string(want) {
				t.Fatalf("got\n%s\nwant\n%s", got, want)
			}
		})
	}
}

func read(t *testing.T, name string) []byte {
	t.Helper()
	data, err := testdata.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestEdits(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, input, want string
		edit              manifestedit.Edit
	}{
		{
			"add without instances", "schema = 1\n\n[targets.c]\nagent = \"claude\"\n", "schema = 1\n\n[targets.c]\nagent = \"claude\"\n\n[targets.c.instances.second]\ndir = \"~/.claude_2\"\n",
			manifestedit.Edit{Op: manifestedit.AddInstance, Target: "c", Name: "second", Dir: "~/.claude_2"},
		},
		{
			"add compact before next table", "schema=1\n[targets.c]\nagent=\"claude\"\n[publish]\nmode=\"direct\"\n", "schema=1\n[targets.c]\nagent=\"claude\"\n\n[targets.c.instances.\"a.b\"]\ndir = \"~/x\"\n\n[publish]\nmode=\"direct\"\n",
			manifestedit.Edit{Op: manifestedit.AddInstance, Target: "c", Name: "a.b", Dir: "~/x"},
		},
		{
			"add without final newline", "schema=1\n[targets.c]\nagent=\"claude\"", "schema=1\n[targets.c]\nagent=\"claude\"\n\n[targets.c.instances.x]\ndir = \"~/x\"\n",
			manifestedit.Edit{Op: manifestedit.AddInstance, Target: "c", Name: "x", Dir: "~/x"},
		},
		{
			"crlf", "schema=1\r\n[targets.c]\r\nagent=\"claude\"\r\n", "schema=1\r\n[targets.c]\r\nagent=\"claude\"\r\n\r\n[targets.c.instances.x]\r\ndir = \"~/x\"\r\nsettings = false\r\n",
			manifestedit.Edit{Op: manifestedit.AddInstance, Target: "c", Name: "x", Dir: "~/x", Settings: off()},
		},
		{
			"edit keeps comment and quotes", "schema=1\n[targets.c]\nagent='claude'\nsettings='s.json'\n[targets.c.instances.x]\ndir = '~/x' # old\nsettings = false\n", "schema=1\n[targets.c]\nagent='claude'\nsettings='s.json'\n[targets.c.instances.x]\ndir = \"~/y\" # old\nsettings = true\n",
			manifestedit.Edit{Op: manifestedit.EditInstance, Target: "c", Name: "x", Dir: "~/y", Settings: on()},
		},
		{
			"settings on stays implicit", "schema=1\n[targets.c]\nagent='claude'\n[targets.c.instances.x]\ndir='~/x'\n", "schema=1\n[targets.c]\nagent='claude'\n[targets.c.instances.x]\ndir='~/x'\n",
			manifestedit.Edit{Op: manifestedit.EditInstance, Target: "c", Name: "x", Settings: on()},
		},
		{
			"settings off after last line", "schema=1\n[targets.c]\nagent='claude'\n[targets.c.instances.x]\ndir='~/x'", "schema=1\n[targets.c]\nagent='claude'\n[targets.c.instances.x]\ndir='~/x'\nsettings = false\n",
			manifestedit.Edit{Op: manifestedit.EditInstance, Target: "c", Name: "x", Settings: off()},
		},
		{
			"remove with sub-table and empty parent", "schema=1\n[targets.c]\nagent='claude'\n[targets.c.instances]\n# kept\n[targets.c.instances.x]\ndir='~/x'\n[targets.c.instances.x.roots]\nskills='~/x/skills'\n\n# about publish\n[publish]\nmode='pr'\n", "schema=1\n[targets.c]\nagent='claude'\n# kept\n\n# about publish\n[publish]\nmode='pr'\n",
			manifestedit.Edit{Op: manifestedit.RemoveInstance, Target: "c", Name: "x"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := manifestedit.Apply("kitbag.toml", []byte(tc.input), tc.edit)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Fatalf("got\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}

func TestRefusals(t *testing.T) {
	t.Parallel()
	add := manifestedit.Edit{Op: manifestedit.AddInstance, Target: "c", Name: "y", Dir: "~/y"}
	edit := manifestedit.Edit{Op: manifestedit.EditInstance, Target: "c", Name: "x", Dir: "~/z"}
	remove := manifestedit.Edit{Op: manifestedit.RemoveInstance, Target: "c", Name: "x"}
	cases := []struct {
		want  error
		name  string
		input string
		edit  manifestedit.Edit
	}{
		{manifestedit.ErrLayout, "inline instances", "schema=1\n[targets.c]\nagent='claude'\ninstances = { x = { dir = '~/x' } }\n", add},
		{manifestedit.ErrLayout, "dotted instance", "schema=1\n[targets.c]\nagent='claude'\ninstances.x.dir = '~/x'\n", edit},
		{manifestedit.ErrLayout, "inline instance value", "schema=1\n[targets.c]\nagent='claude'\n[targets.c.instances]\nx = { dir = '~/x' }\n", remove},
		{manifestedit.ErrLayout, "dotted target", "schema=1\ntargets.c.agent='claude'\n", add},
		{manifestedit.ErrLayout, "inline targets for new target", "schema=1\ntargets = { c = { agent = 'claude' } }\n", manifestedit.Edit{Op: manifestedit.AddTarget, Target: "d", Agent: "codex"}},
		{manifestedit.ErrExists, "duplicate name", "schema=1\n[targets.c]\nagent='claude'\n[targets.c.instances.y]\ndir='~/q'\n", add},
		{manifestedit.ErrExists, "duplicate target", "schema=1\n[targets.c]\nagent='claude'\n", manifestedit.Edit{Op: manifestedit.AddTarget, Target: "c", Agent: "claude"}},
		{manifestedit.ErrNotFound, "missing instance", "schema=1\n[targets.c]\nagent='claude'\n", remove},
		{manifestedit.ErrNotFound, "missing target", "schema=1\n", add},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := manifestedit.Apply("kitbag.toml", []byte(tc.input), tc.edit)
			if !errors.Is(err, tc.want) || got != nil {
				t.Fatalf("got %q, %v; want %v", got, err, tc.want)
			}
		})
	}
}

// TestVerifyRejectsInvalidResult proves the reparse check: a name the
// manifest grammar refuses or a colliding directory never reaches the caller.
func TestVerifyRejectsInvalidResult(t *testing.T) {
	t.Parallel()
	input := []byte("schema=1\n[targets.c]\nagent='claude'\n[targets.c.instances.x]\ndir='~/x'\n[targets.c.instances.y]\ndir='~/y'\n")
	for name, edit := range map[string]manifestedit.Edit{
		"bad name":  {Op: manifestedit.AddInstance, Target: "c", Name: "Bad Name", Dir: "~/y"},
		"bad dir":   {Op: manifestedit.AddInstance, Target: "c", Name: "z", Dir: "/abs"},
		"collision": {Op: manifestedit.EditInstance, Target: "c", Name: "x", Dir: "~/Y"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := manifestedit.Apply("kitbag.toml", input, edit); !errors.Is(err, manifestedit.ErrVerify) {
				t.Fatalf("got %v, want ErrVerify", err)
			}
		})
	}
	if _, err := manifestedit.Apply("kitbag.toml", []byte("schema=["), manifestedit.Edit{}); err == nil {
		t.Fatal("invalid input accepted")
	}
}

func TestQuotesSpecialCharacters(t *testing.T) {
	t.Parallel()
	input := []byte("schema=1\n[targets.c]\nagent='claude'\n")
	dir := "~/it's é #1"
	got, err := manifestedit.Apply("kitbag.toml", input, manifestedit.Edit{Op: manifestedit.AddInstance, Target: "c", Name: "x", Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	m, err := manifest.Parse("kitbag.toml", got)
	if err != nil || m.Targets["c"].Instances["x"].Dir != dir || !strings.Contains(string(got), `dir = "~/it's é #1"`) {
		t.Fatalf("%s: %v", got, err)
	}
}
