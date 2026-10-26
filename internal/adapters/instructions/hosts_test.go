package instructions_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/adapters/instructions"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
)

// hostRepository holds host additions, decoys and a non-regular entry.
func hostRepository(t *testing.T) (*fake.Host, string) {
	t.Helper()
	h := fake.New()
	repo := filepath.FromSlash("/repo")
	files := []string{"hosts/mac.md", "hosts/runner02.md", "hosts/notes.txt", "hosts/.md", "dirs/mac/AGENTS.md", "dirs/linux/README.md", "plain"}
	for _, file := range files {
		path := filepath.Join(repo, filepath.FromSlash(file))
		must(t, h.MkdirAll(filepath.Dir(path), 0o700))
		must(t, h.WriteAtomic(path, []byte("x"), 0o600))
	}
	must(t, h.MkdirAll(filepath.Join(repo, "hosts", "folder.md"), 0o700))
	must(t, h.Link(filepath.Join(repo, "hosts", "mac.md"), filepath.Join(repo, "hosts", "alias.md")))
	return h, repo
}

func TestHostAdditions(t *testing.T) {
	t.Parallel()
	h, repo := hostRepository(t)
	for template, want := range map[string][]string{
		"hosts/{host}.md":       {"mac", "runner02"},
		"dirs/{host}/AGENTS.md": {"mac"},
		"{host}":                {"plain"},
		"missing/{host}.md":     nil,
		"plain/{host}.md":       nil,
		"hosts/{host}{host}.md": nil,
	} {
		got, err := instructions.HostAdditions(h, repo, template)
		if err != nil || !slices.Equal(got, want) {
			t.Errorf("%s: HostAdditions = %v, %v; want %v", template, got, err, want)
		}
	}
	message, err := instructions.HostMismatch(h, repo, "hosts/{host}.md", "MBP-TJ")
	if err != nil || !strings.Contains(message, `host "MBP-TJ"`) || !strings.Contains(message, "hosts/mac.md, hosts/runner02.md for host(s) mac, runner02") {
		t.Fatalf("HostMismatch = %q, %v", message, err)
	}
	for _, tc := range [][2]string{{"hosts/{host}.md", "mac"}, {"missing/{host}.md", "MBP-TJ"}, {"", "MBP-TJ"}} {
		if message, err = instructions.HostMismatch(h, repo, tc[0], tc[1]); err != nil || message != "" {
			t.Errorf("%v: HostMismatch = %q, %v; want none", tc, message, err)
		}
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
