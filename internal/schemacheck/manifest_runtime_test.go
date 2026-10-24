package schemacheck_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

func TestManifestRuntimeGoldens(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, first, second, want string }{
		{"duplicate-skill", "alpha", "alpha", "skill name collision"},
		{"case-collision", "Alpha", "alpha", "skill name collision"},
		{"claude-reserved", "synced", "beta", "reserved for Claude"},
		{"windows-reserved", "NUL.txt", "beta", "reserved on Windows"},
		{"forbidden-character", "bad:name", "beta", "forbidden character"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := fake.New()
			for dir, name := range map[string]string{"one": tc.first, "two": tc.second} {
				writeRuntimeFile(t, h, filepath.Join(runtimeRoot(), dir, name, "SKILL.md"), "skill")
			}
			file := filepath.Join(runtimeRoot(), "kitbag.toml")
			writeRuntimeFile(t, h, file, "schema=1\n[skills]\ndirs=['one','two']\n[targets.c]\nagent='claude'")
			_, err := manifest.Load(h, file)
			runtimeGolden(t, tc.name, tc.want, err)
		})
	}
}

func TestManifestSourceGoldens(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"missing-source", "escaping-source", "escaping-host", "minimum-version"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			runtimeGolden(t, name, "", runtimeSourceError(t, name))
		})
	}
}

func runtimeSourceError(t *testing.T, name string) error {
	t.Helper()
	h := fake.New()
	root := runtimeRoot()
	writeRuntimeFile(t, h, filepath.Join(root, "safe.md"), "safe")
	outside := filepath.Join(filepath.Dir(root), "outside.md")
	writeRuntimeFile(t, h, outside, "outside")
	if err := h.Link(outside, filepath.Join(root, "escape.md")); err != nil {
		t.Fatal(err)
	}
	source := "safe.md"
	if name == "missing-source" {
		source = "missing.md"
	}
	if name == "escaping-source" {
		source = "escape.md"
	}
	file := filepath.Join(root, "kitbag.toml")
	body := "schema=1\nmin_tool_version='1.2.3'\n[instructions]\nsource='" + source + "'\nhosts='{host}.md'"
	writeRuntimeFile(t, h, file, body)
	m, err := manifest.Load(h, file)
	if err != nil {
		return err
	}
	if name == "minimum-version" {
		return m.CheckVersion("1.2.2")
	}
	_, err = m.ResolveHost(h, "escape")
	return err
}

func runtimeGolden(t *testing.T, name, want string, err error) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("expected %q, got %v", want, err)
	}
	golden(t, "testdata/manifest/"+name+".golden", err.Error()+"\n")
}
func runtimeRoot() string { return filepath.Join(string(filepath.Separator), "repo") }
func writeRuntimeFile(t *testing.T, h *fake.Host, name, body string) {
	t.Helper()
	if err := h.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := h.WriteAtomic(name, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
