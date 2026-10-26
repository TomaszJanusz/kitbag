package agents_test

import (
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/agents"
	"github.com/TomaszJanusz/kitbag/internal/core/discover"
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
)

const (
	bare = "schema = 1\n\n[targets.claude]\nagent = \"claude\"\n"
	base = "schema = 1\n\n[targets.claude]\nagent = \"claude\"\nsettings = \"claude/settings.json\"\n\n" +
		"[targets.claude.instances.main]\ndir = \"~/.claude\"\n"
	two = base + "\n[targets.claude.instances.second]\ndir = \"~/.claude_2\"\n"
)

func home() string { return filepath.Join(string(filepath.Separator), "home", "u") }

func resolver(agent, variable, rel string) discover.Resolver {
	return discover.Resolver{Agent: agent, Variable: variable, DefaultDir: "~/" + rel, Home: func(env platform.Env) (string, error) {
		return env.AgentHome(variable, rel)
	}}
}

func kinds() discover.Table {
	return discover.Table{resolver("claude", "CLAUDE_CONFIG_DIR", ".claude"), resolver("codex", "CODEX_HOME", ".codex"), {Agent: "generic"}}
}

// newHost has ~/.claude and ~/.claude_2 directories and a ~/file file.
func newHost(t *testing.T, env map[string]string) *fake.Host {
	t.Helper()
	vars := map[string]string{"HOME": home()}
	for k, v := range env {
		vars[k] = v
	}
	h := fake.New(fake.WithEnv(vars))
	for _, dir := range []string{".claude", ".claude_2", ".codex"} {
		if err := h.MkdirAll(filepath.Join(home(), dir), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.WriteAtomic(filepath.Join(home(), "file"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	return h
}

func prepare(t *testing.T, h *fake.Host, manifest string, change agents.Change) (agents.Plan, error) {
	t.Helper()
	if h == nil {
		h = newHost(t, nil)
	}
	return agents.Prepare(agents.Input{Host: h, Kinds: kinds(), Filename: "kitbag.toml", HostName: "mac", Manifest: []byte(manifest), Change: change})
}

func add(name, dir string) agents.Change {
	return agents.Change{Op: agents.Add, Kind: "claude", Name: name, Dir: dir}
}

// expect fails the test when ok is false, showing got.
func expect(t *testing.T, ok bool, what string, got any) {
	t.Helper()
	if !ok {
		t.Fatalf("%s: %+v", what, got)
	}
}

func on() *bool  { v := true; return &v }
func off() *bool { v := false; return &v }

func TestAddMaterializesDefaultAndMessage(t *testing.T) {
	t.Parallel()
	p, err := prepare(t, nil, bare, add("second", "~/.claude_2"))
	expect(t, err == nil, "error", err)
	wantMessage := "chore(kitbag): add claude instance second at ~/.claude_2\n\n" +
		"- add claude instance second at ~/.claude_2\n" +
		"- declare the default directory ~/.claude as instance main so it stays managed\n\n" +
		"Kitbag-Host: mac\nKitbag-Op: agent.add\n"
	expect(t, p.Title == "chore(kitbag): add claude instance second at ~/.claude_2", "title", p.Title)
	expect(t, p.Message == wantMessage, "message", p.Message)
	expect(t, len(p.Edits) == 2 && p.Edits[0].Name == "main" && p.Edits[0].Dir == "~/.claude" && p.Edits[1].Name == "second", "edits", p.Edits)
	expect(t, slices.Equal(p.Warnings, []string{"the default directory ~/.claude is declared as instance main so it stays managed"}), "warnings", p.Warnings)
	expect(t, p.Revision == agents.Revision([]byte(bare)) && p.Revision != agents.Revision([]byte(base)) && len(p.Revision) == 64, "revision", p.Revision)
	expect(t, strings.Contains(string(p.After), "[targets.claude.instances.main]\ndir = \"~/.claude\""), "after", string(p.After))
}

func TestAddDefaultNamedAlternateAndRelocationWarning(t *testing.T) {
	t.Parallel()
	h := newHost(t, map[string]string{"CLAUDE_CONFIG_DIR": filepath.Join(home(), ".claude_2")})
	p, err := prepare(t, h, bare, add("main", "~/.claude_2"))
	if err != nil {
		t.Fatal(err)
	}
	if p.Edits[0].Name != "default" || p.Edits[1].Name != "main" {
		t.Fatalf("edits %+v", p.Edits)
	}
	if len(p.Warnings) != 2 || !strings.Contains(p.Warnings[1], "CLAUDE_CONFIG_DIR is set here") {
		t.Fatalf("warnings %q", p.Warnings)
	}
}

func TestAddDefaultDirectoryNeedsNoMaterialization(t *testing.T) {
	t.Parallel()
	p, err := prepare(t, nil, bare, add("first", "~/.claude"))
	if err != nil || len(p.Edits) != 1 || len(p.Warnings) != 0 {
		t.Fatalf("%+v, %v", p, err)
	}
}

func TestAddDiffAndNormalization(t *testing.T) {
	t.Parallel()
	for _, dir := range []string{"~/.claude_2", filepath.Join(home(), ".claude_2"), filepath.Join(home(), "x", "..", ".claude_2")} {
		p, err := prepare(t, nil, base, add("second", dir))
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"@@ -7,2 +7,5 @@", " [targets.claude.instances.main]", " dir = \"~/.claude\"", "+", "+[targets.claude.instances.second]", "+dir = \"~/.claude_2\""}
		if p.Change.Dir != "~/.claude_2" || p.Change.Target != "claude" || !slices.Equal(p.Diff, want) {
			t.Fatalf("%s: change %+v diff %q", dir, p.Change, p.Diff)
		}
		if p.Message != "chore(kitbag): add claude instance second at ~/.claude_2\n\nKitbag-Host: mac\nKitbag-Op: agent.add\n" {
			t.Fatalf("message %q", p.Message)
		}
	}
}

func TestAddNewTargetAndTargetSelection(t *testing.T) {
	t.Parallel()
	p, err := prepare(t, nil, base, agents.Change{Op: agents.Add, Kind: "codex", Name: "main", Dir: "~/.codex"})
	expect(t, err == nil && p.Change.Target == "codex" && p.Edits[0].Agent == "codex" && len(p.Edits) == 2, "new target", p.Edits)
	manifest := base + "\n[targets.work]\nagent = \"claude\"\n\n[targets.work.instances.w]\ndir = \"~/.claude_w\"\n"
	_, err = prepare(t, nil, manifest, add("x", "~/.claude_2"))
	expect(t, errors.Is(err, agents.ErrInvalid) && strings.Contains(err.Error(), "--target"), "ambiguous", err)
	c := add("x", "~/.claude_2")
	c.Target = "work"
	p, err = prepare(t, nil, manifest, c)
	expect(t, err == nil && p.Change.Target == "work" && strings.Contains(p.Title, "of target work"), "selected", p)
	c.Target = "fresh"
	p, err = prepare(t, nil, manifest, c)
	expect(t, err == nil && p.Edits[0].Target == "fresh", "named new target", p.Edits)
	c.Kind, c.Target = "codex", "work"
	_, err = prepare(t, nil, manifest, c)
	expect(t, errors.Is(err, agents.ErrInvalid), "target of other kind", err)
}

func TestAddWarnings(t *testing.T) {
	t.Parallel()
	h := newHost(t, nil)
	if err := h.Link(filepath.Join(home(), ".claude_2"), filepath.Join(home(), ".claude_link")); err != nil {
		t.Fatal(err)
	}
	for dir, want := range map[string]string{"~/.claude_new": "does not exist", "~/.claude_link": "symbolic link"} {
		p, err := prepare(t, h, base, add("x", dir))
		if err != nil || len(p.Warnings) != 1 || !strings.Contains(p.Warnings[0], want) {
			t.Fatalf("%s: %q, %v", dir, p.Warnings, err)
		}
	}
}

func TestPrepareRefusals(t *testing.T) {
	t.Parallel()
	hostSet := base + "\n[hosts.mac.targets.claude.instances.only]\ndir = \"~/.claude\"\n"
	edit := agents.Change{Op: agents.Edit, Target: "claude", Name: "main"}
	cases := []struct {
		kind     error
		name     string
		manifest string
		change   agents.Change
	}{
		{agents.ErrInvalid, "unknown kind", base, agents.Change{Op: agents.Add, Kind: "cursor", Name: "x", Dir: "~/x"}},
		{agents.ErrUnsupported, "generic", base, agents.Change{Op: agents.Add, Kind: "generic", Name: "x", Dir: "~/x"}},
		{agents.ErrInvalid, "bad name", base, add("Bad", "~/.claude_2")},
		{agents.ErrInvalid, "name taken", base, add("main", "~/.claude_2")},
		{agents.ErrInvalid, "home itself", base, add("x", "~")},
		{agents.ErrInvalid, "relative", base, add("x", ".claude_2")},
		{agents.ErrInvalid, "outside home", base, add("x", filepath.Join(string(filepath.Separator), "tmp", "x"))},
		{agents.ErrInvalid, "duplicate folded", base, add("x", "~/.CLAUDE")},
		{agents.ErrInvalid, "settings without fragment", bare, agents.Change{Op: agents.Add, Kind: "claude", Name: "x", Dir: "~/.claude_2", Settings: off()}},
		{agents.ErrInvalid, "settings on new target", base, agents.Change{Op: agents.Add, Kind: "codex", Name: "x", Dir: "~/.codex", Settings: on()}},
		{agents.ErrUnsupported, "host set", hostSet, add("x", "~/.claude_2")},
		{agents.ErrUnsupported, "host set edit", hostSet, agents.Change{Op: agents.Edit, Target: "claude", Name: "main", Settings: off()}},
		{agents.ErrInvalid, "not a directory", base, add("x", "~/file")},
		{agents.ErrInvalid, "nothing to change", base, edit},
		{agents.ErrInvalid, "same values", base, agents.Change{Op: agents.Edit, Target: "claude", Name: "main", Dir: "~/.claude", Settings: on()}},
		{agents.ErrInvalid, "missing instance", base, agents.Change{Op: agents.Edit, Target: "claude", Name: "nope", Settings: off()}},
		{agents.ErrInvalid, "missing target", base, agents.Change{Op: agents.Remove, Target: "nope", Name: "main"}},
		{agents.ErrInvalid, "ambiguous kind", two + "\n[targets.work]\nagent = \"claude\"\n", agents.Change{Op: agents.Remove, Target: "claude2", Name: "main"}},
		{agents.ErrInvalid, "last instance", base, agents.Change{Op: agents.Remove, Target: "claude", Name: "main"}},
		{agents.ErrInvalid, "unknown op", base, agents.Change{Op: "rename"}},
		{agents.ErrUnsupported, "layout", "schema = 1\n\n[targets.claude]\nagent = \"claude\"\ninstances = { main = { dir = \"~/.claude\" } }\n", add("x", "~/.claude_2")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := prepare(t, nil, tc.manifest, tc.change)
			if !errors.Is(err, tc.kind) || !errors.Is(err, fs.ErrInvalid) {
				t.Fatalf("got %v; want %v", err, tc.kind)
			}
		})
	}
}

func TestAmbiguousKindReference(t *testing.T) {
	t.Parallel()
	manifest := two + "\n[targets.work]\nagent = \"claude\"\n\n[targets.work.instances.w]\ndir = \"~/.claude_w\"\n"
	_, err := prepare(t, nil, manifest, agents.Change{Op: agents.Remove, Target: "codex", Name: "main"})
	if !errors.Is(err, agents.ErrInvalid) || !strings.Contains(err.Error(), "no target or agent kind") {
		t.Fatalf("unknown ref: %v", err)
	}
	manifest = strings.Replace(manifest, "[targets.claude]", "[targets.cc]", 1)
	manifest = strings.ReplaceAll(manifest, "targets.claude.", "targets.cc.")
	_, err = prepare(t, nil, manifest, agents.Change{Op: agents.Remove, Target: "claude", Name: "main"})
	if !errors.Is(err, agents.ErrInvalid) || !strings.Contains(err.Error(), "used by targets cc, work") {
		t.Fatalf("ambiguous: %v", err)
	}
}

func renamedTarget() string {
	manifest := strings.Replace(two, "[targets.claude]", "[targets.cc]", 1)
	return strings.ReplaceAll(manifest, "targets.claude.", "targets.cc.")
}

func TestEditMovesDirectory(t *testing.T) {
	t.Parallel()
	p, err := prepare(t, nil, renamedTarget(), agents.Change{Op: agents.Edit, Target: "claude", Name: "second", Dir: "~/.claude-work"})
	expect(t, err == nil && p.Dropped == "~/.claude_2" && p.Change.Target == "cc" && p.Change.Dir == "~/.claude-work", "move", p)
	// Longer than 72 characters: the title drops the details.
	expect(t, p.Title == "chore(kitbag): edit claude instance second", "title", p.Title)
	expect(t, strings.Contains(p.Message, "Kitbag-Op: agent.edit"), "message", p.Message)
}

func TestEditSettingsOnly(t *testing.T) {
	t.Parallel()
	p, err := prepare(t, nil, renamedTarget(), agents.Change{Op: agents.Edit, Target: "cc", Name: "second", Settings: off()})
	expect(t, err == nil && p.Dropped == "" && p.Edits[0].Dir == "", "settings", p)
	expect(t, strings.Contains(string(p.After), "settings = false"), "after", string(p.After))
	expect(t, strings.HasSuffix(p.Title, "(settings off)") && strings.HasSuffix(p.Message, "Kitbag-Op: agent.edit\n"), "title", p.Message)
}

func TestRemove(t *testing.T) {
	t.Parallel()
	p, err := prepare(t, nil, renamedTarget(), agents.Change{Op: agents.Remove, Target: "claude", Name: "second"})
	expect(t, err == nil && p.Dropped == "~/.claude_2" && !strings.Contains(string(p.After), "second"), "remove", p)
	expect(t, p.Title == "chore(kitbag): remove claude instance second of target cc", "title", p.Title)
	expect(t, strings.HasSuffix(p.Message, "Kitbag-Op: agent.remove\n"), "message", p.Message)
}

func TestLongTitleDropsDirectory(t *testing.T) {
	t.Parallel()
	dir := "~/" + strings.Repeat("d", 60)
	p, err := prepare(t, nil, base, add("x", dir))
	if err != nil || p.Title != "chore(kitbag): add claude instance x" || !strings.Contains(p.Message, "\n- add claude instance x at "+dir+"\n") {
		t.Fatalf("%q, %q, %v", p.Title, p.Message, err)
	}
}

func TestInvalidManifestIsReported(t *testing.T) {
	t.Parallel()
	if _, err := prepare(t, nil, "schema = [", add("x", "~/x")); err == nil {
		t.Fatal("invalid manifest accepted")
	}
}

func TestDiff(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		before, after string
		want          []string
	}{
		"identical":  {"a\nb\n", "a\nb\n", []string{}},
		"replace":    {"a\nb\nc\n", "a\nx\nc\n", []string{"@@ -1,3 +1,3 @@", " a", "+x", "-b", " c"}},
		"crlf":       {"a\r\nb\r\n", "a\r\n", []string{"@@ -1,2 +1,1 @@", " a", "-b"}},
		"from empty": {"", "a\n", []string{"@@ -0,0 +1,1 @@", "+a"}},
		"two hunks": {
			"1\n2\n3\n4\n5\n6\n7\n8\n9\n", "x\n2\n3\n4\n5\n6\n7\n8\ny\n",
			[]string{"@@ -1,3 +1,3 @@", "+x", "-1", " 2", " 3", "@@ -7,3 +7,3 @@", " 7", " 8", "+y", "-9"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := agents.Diff([]byte(tc.before), []byte(tc.after)); !slices.Equal(got, tc.want) {
				t.Fatalf("got %q; want %q", got, tc.want)
			}
		})
	}
}

// TestSecondTargetKeepsDefaultWithFirst adds the first instance to a second
// target of a kind whose default another target already declares: nothing
// else is declared, and the change is not refused.
func TestSecondTargetKeepsDefaultWithFirst(t *testing.T) {
	t.Parallel()
	manifest := base + "\n[targets.work]\nagent = \"claude\"\n"
	change := add("second", "~/.claude_2")
	change.Target = "work"
	p, err := prepare(t, nil, manifest, change)
	expect(t, err == nil && len(p.Edits) == 1, "second target", p)
	expect(t, p.Title == "chore(kitbag): add claude instance second of target work at ~/.claude_2", "title", p.Title)
}
