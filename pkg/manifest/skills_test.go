package manifest_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
)

func TestSkillValidation(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, first, second, agent, want string }{
		{"unique", "alpha", "beta", "claude", ""},
		{"duplicate", "alpha", "alpha", "claude", "skill name collision"},
		{"case collision", "Alpha", "alpha", "claude", "skill name collision"},
		{"unicode collision", "ſkill", "Skill", "codex", "skill name collision"},
		{"sigma collision", "ς", "σ", "codex", "skill name collision"},
		{"reserved", "synced", "beta", "claude", "reserved for Claude"},
		{"reserved mixed case", "Synced", "beta", "claude", "reserved for Claude"},
		{"other agent", "synced", "beta", "codex", ""},
		{"windows", "con.txt", "beta", "codex", "reserved on Windows"},
		{"forbidden", "bad:name", "beta", "codex", "forbidden character"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := fake.New()
			root := rootPath()
			writeFake(t, h, filepath.Join(root, "one", tc.first, "SKILL.md"), "skill")
			writeFake(t, h, filepath.Join(root, "two", tc.second, "SKILL.md"), "skill")
			// Non-skill entries must not be counted as skills.
			writeFake(t, h, filepath.Join(root, "two", "README.md"), "readme")
			m, err := loadFake(t, h, "schema=1\n[skills]\ndirs=['one','two']\n[targets.main]\nagent='"+tc.agent+"'")
			assertParseResult(t, m, normalizeFilename(err), tc.want)
		})
	}
}

func TestDirectoryContents(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, entry, want string }{
		{"windows child", "NUL.txt", "reserved on Windows"},
		{"case child", "README.md", "case-insensitive name collision"},
		{"safe child", "notes.md", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := fake.New()
			writeFake(t, h, filepath.Join(rootPath(), "commands", "readme.md"), "a")
			writeFake(t, h, filepath.Join(rootPath(), "commands", tc.entry), "b")
			m, err := loadFake(t, h, "schema=1\n[commands]\ndir='commands'")
			assertParseResult(t, m, normalizeFilename(err), tc.want)
		})
	}
}

func TestMissingHostAdditionAndForeignImport(t *testing.T) {
	t.Parallel()
	h := fake.New()
	writeFake(t, h, filepath.Join(rootPath(), "AGENTS.md"), "instructions")
	writeFake(t, h, filepath.Join(rootPath(), "skills", "pdf", "SKILL.md"), "skill")
	m, err := loadFake(t, h, `schema=1
[instructions]
source='AGENTS.md'
hosts='hosts/{host}.md'
[skills]
dirs=['skills']
[skills.imports.pdf]
source='owner/repo'
path='nonexistent/in/this/repository'
`)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := m.ResolveHost(h, "mac")
	if err != nil || resolved.Instructions.Hosts != "hosts/mac.md" {
		t.Fatalf("ResolveHost = %v, %v", resolved, err)
	}
}

func TestDirectoryLinkEscapeAndCycle(t *testing.T) {
	t.Parallel()
	for _, target := range []string{"../../external", "."} {
		t.Run(target, func(t *testing.T) {
			t.Parallel()
			h := fake.New()
			writeFake(t, h, filepath.Join(rootPath(), "commands", "safe.md"), "a")
			writeFake(t, h, filepath.Join(filepath.Dir(rootPath()), "external", "bad.md"), "b")
			if err := h.Link(target, filepath.Join(rootPath(), "commands", "link")); err != nil {
				t.Fatal(err)
			}
			_, err := loadFake(t, h, "schema=1\n[commands]\ndir='commands'")
			if err == nil || (!strings.Contains(err.Error(), "escapes repository") && !strings.Contains(err.Error(), "cycle")) {
				t.Fatalf("accepted unsafe directory link: %v", err)
			}
		})
	}
}

func TestHostAdditionConfinement(t *testing.T) {
	t.Parallel()
	h := fake.New()
	writeFake(t, h, filepath.Join(rootPath(), "AGENTS.md"), "instructions")
	writeFake(t, h, filepath.Join(rootPath(), "hosts", "safe.md"), "safe")
	writeFake(t, h, filepath.Join(filepath.Dir(rootPath()), "outside.md"), "outside")
	if err := h.Link("../../outside.md", filepath.Join(rootPath(), "hosts", "escape.md")); err != nil {
		t.Fatal(err)
	}
	m, err := loadFake(t, h, "schema=1\n[instructions]\nsource='AGENTS.md'\nhosts='hosts/{host}.md'")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.ResolveHost(h, "safe"); err != nil {
		t.Fatal(err)
	}
	if _, err = m.ResolveHost(h, "missing"); err != nil {
		t.Fatal(err)
	}
	if _, err = m.ResolveHost(h, "escape"); err == nil || !strings.Contains(err.Error(), "escape repository") {
		t.Fatalf("host symlink escape accepted: %v", err)
	}
}
