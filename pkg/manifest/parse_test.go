package manifest_test

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

func TestParseRules(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, body, want string }{
		{"minimal", "schema=1", ""},
		{"missing schema", "", "missing property 'schema'"},
		{"future schema", "schema=2", "manifest schema 2 is newer than this kitbag reads (schema 1); update kitbag"},
		{"old schema", "schema=0", "value must be 1"},
		{"type", "schema=true", "want integer"},
		{"syntax", "schema=[", "toml:"},
		{"unknown", "schema=1\nmod='copy'", "did you mean 'mode'"},
		{"nested unknown", "schema=1\n[instructions]\nsource='a'\nhost='b'", "did you mean 'hosts'"},
		{"provenance", "schema=1\n[provenance]", "'provenance' not allowed"},
		{"state", "schema=1\n[state]", "'state' not allowed"},
		{"path escape", "schema=1\n[instructions]\nsource='../a'", "does not match pattern"},
		{"dot segment", "schema=1\n[instructions]\nsource='a/./b'", "does not match pattern"},
		{"empty segment", "schema=1\n[instructions]\nsource='a//b'", "does not match pattern"},
		{"absolute", "schema=1\n[instructions]\nsource='/a'", "does not match pattern"},
		{"environment", "schema=1\n[instructions]\nsource='$HOME/a'", "does not match pattern"},
		{"home source", "schema=1\n[instructions]\nsource='~/a'", "does not match pattern"},
		{"backslash", "schema=1\n[instructions]\nsource='a\\b'", "does not match pattern"},
		{"trailing slash", "schema=1\n[instructions]\nsource='a/'", "does not match pattern"},
		{"foreign escape", "schema=1\n[skills]\ndirs=['skills']\n[skills.imports.pdf]\nsource='owner/repo'\npath='../a'", "does not match pattern"},
		{"bad import dir", "schema=1\n[skills]\ndirs=['skills']\n[skills.imports.pdf]\nsource='owner/repo'\ndir='other'", "must be one of skills.dirs"},
		{"host unknown target", "schema=1\n[hosts.mac.targets.absent]\nenabled=false", "undeclared target"},
		{"host collision", "schema=1\n[hosts.Mac]\nmode='copy'\n[hosts.mac]\nmode='link'", "case-insensitive name collision"},
		{"path collision", "schema=1\n[git]\ninclude=['A.gitconfig','a.gitconfig']", "case-insensitive path collision"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, err := manifest.Parse("kitbag.toml", []byte(tc.body))
			assertParseResult(t, m, err, tc.want)
		})
	}
}

func assertParseResult(t *testing.T, m *manifest.Manifest, err error, want string) {
	t.Helper()
	if want == "" {
		if err != nil || m == nil {
			t.Fatalf("Parse = %v, %v", m, err)
		}
		return
	}
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want %q", err, want)
	}
	if m != nil {
		t.Fatal("invalid manifest returned a partial model")
	}
	assertPositioned(t, err)
}

func assertPositioned(t *testing.T, err error) {
	t.Helper()
	var diagnostic *manifest.Error
	if !errors.As(err, &diagnostic) || diagnostic.Line < 1 || diagnostic.Column < 1 || diagnostic.File != "kitbag.toml" {
		t.Fatalf("missing diagnostic position: %v", err)
	}
}

func TestPortableNames(t *testing.T) {
	t.Parallel()
	names := []string{"CON", "con.txt", "NUL", "PRN", "AUX", "COM1", "lpt9.md", "COM¹", "LPT².txt", "CONIN$", "CONOUT$", "a.", "a ", "a:b", "a?b", "a*b", "a|b", "a<b", "a>b", "a\"b", "a\x01b"}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			body := fmt.Sprintf("schema=1\n[instructions]\nsource=%q", name)
			_, err := manifest.Parse("kitbag.toml", []byte(body))
			if err == nil {
				t.Fatal("accepted a nonportable source name")
			}
		})
	}
}

func TestValidPortablePaths(t *testing.T) {
	t.Parallel()
	for _, name := range []string{".agents/skills", "..prefix/a", "a/..prefix", "a b/é.md", "COM0", "COM10", "console", "null"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := manifest.Parse("kitbag.toml", []byte(fmt.Sprintf("schema=1\n[instructions]\nsource=%q", name)))
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPositions(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, body, key string
		line, column    int
	}{
		{"dotted", "schema=1\ntargets.c.agent='claude'\ntargets.c.setings='a'", "/targets/c/setings", 3, 11},
		{"quoted inline", "schema=1\ntargets = { 'c' = {agent='claude', setings='a'} }", "/targets/c/setings", 2, 36},
		{"second file", "schema=1\n[[files]]\nsrc='a'\ndest='~/a'\n[[files]]\nsrc='../b'\ndest='~/b'", "/files/1/src", 6, 1},
		{"multiline array", "schema=1\n[skills]\ndirs=[\n 'a',\n '../b',\n]", "/skills/dirs/1", 5, 2},
		{"missing key", "schema=1\n[targets.c]\nmode='copy'", "/targets/c", 2, 10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := manifest.Parse("kitbag.toml", []byte(tc.body))
			var got *manifest.Error
			if !errors.As(err, &got) {
				t.Fatalf("expected diagnostic, got %v", err)
			}
			if got.Key != tc.key || got.Line != tc.line || got.Column != tc.column {
				t.Fatalf("got %s:%d:%d, want %s:%d:%d", got.Key, got.Line, got.Column, tc.key, tc.line, tc.column)
			}
		})
	}
}

func rootPath() string { return filepath.Join(string(filepath.Separator), "repo") }
func writeFake(t *testing.T, h platform.FS, name, content string) {
	t.Helper()
	if err := h.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := h.WriteAtomic(name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func loadFake(t *testing.T, h platform.FS, body string) (*manifest.Manifest, error) {
	t.Helper()
	name := filepath.Join(rootPath(), "kitbag.toml")
	writeFake(t, h, name, body)
	return manifest.Load(h, name)
}

func TestLoadSources(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, source, want string }{
		{"exists", "present.md", ""},
		{"missing", "missing.md", "source missing.md"},
		{"directory as file", "dir", "must be a regular file"},
		{"symlink escape", "escape", "escapes repository"},
		{"internal link", "internal", ""},
		{"cycle", "cycle", "too many symbolic links"},
		{"intermediate escape", "outside/file", "escapes repository"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := fake.New()
			setupSourceFS(t, h)
			m, err := loadFake(t, h, fmt.Sprintf("schema=1\n[instructions]\nsource=%q", tc.source))
			assertParseResult(t, m, normalizeFilename(err), tc.want)
			if tc.name == "missing" && !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("lost cause: %v", err)
			}
		})
	}
}

func setupSourceFS(t *testing.T, h platform.FS) {
	t.Helper()
	root := rootPath()
	writeFake(t, h, filepath.Join(root, "present.md"), "present")
	writeFake(t, h, filepath.Join(filepath.Dir(root), "external", "file"), "external")
	if err := h.MkdirAll(filepath.Join(root, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	links := map[string]string{"escape": "../external/file", "internal": "present.md", "cycle": "cycle", "outside": "../external"}
	for name, target := range links {
		if err := h.Link(target, filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}
}

func normalizeFilename(err error) error {
	var diagnostic *manifest.Error
	if errors.As(err, &diagnostic) {
		diagnostic.File = "kitbag.toml"
	}
	return err
}

func TestFileDestinationCollisions(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, hosts, source string
		valid               bool
	}{
		{"different sources", "", "b", false},
		{"same source", "", "a", true},
		{"overlapping hosts", "hosts=['mac']", "b", false},
		{"disjoint hosts", "hosts=['linux']", "b", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			body := "schema=1\n[[files]]\nsrc='a'\ndest='~/same'\nhosts=['mac']\n[[files]]\nsrc='" + tc.source + "'\ndest='~/same'\n" + tc.hosts
			_, err := manifest.Parse("kitbag.toml", []byte(body))
			if (err == nil) != tc.valid {
				t.Fatalf("destination validation = %v", err)
			}
		})
	}
}
