package main

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/config"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

// The service hands git.identity from config.toml to the embedded backend,
// which commits with it when Git configuration has no identity.
func TestServiceEmbeddedIdentity(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, identity, author string
	}{
		{name: "configured", identity: "[git.identity]\nname='Config Publisher'\nemail='publisher@example.com'", author: "Config Publisher <publisher@example.com>"},
		{name: "missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			home, dir := t.TempDir(), filepath.Join(t.TempDir(), "repo")
			fixtureGit(t, home, "", "init", "--quiet", dir)
			if err := os.WriteFile(filepath.Join(dir, "file"), []byte("change\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			cfg, err := config.Parse([]byte("schema=1\n[repo]\nurl='local'\n" + tc.identity))
			if err != nil {
				t.Fatal(err)
			}
			c := &commandLine{
				host:   fake.New(fake.WithEnv(map[string]string{"HOME": home, "PATH": ""})),
				stdout: io.Discard, stderr: io.Discard, config: cfg,
				selection: vcs.Selection{Kind: vcs.Embedded, Source: vcs.SourceFlag},
			}
			svc, err := c.service(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			hash, err := svc.Backend.Commit(t.Context(), dir, vcs.CommitOptions{Message: "Publish"})
			if tc.author == "" {
				if !errors.Is(err, vcs.ErrUnsupported) {
					t.Fatalf("commit without identity = %q, %v; want ErrUnsupported", hash, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := fixtureGit(t, home, dir, "log", "-1", "--format=%an <%ae>", hash); got != tc.author {
				t.Fatalf("author = %q, want %q", got, tc.author)
			}
		})
	}
}

// fixtureGit runs system Git with an isolated configuration to build and
// inspect fixtures; the code under test never starts it.
func fixtureGit(t *testing.T, home, dir string, args ...string) string {
	t.Helper()
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), gitPath, args...) //nolint:gosec // G204: test fixture runs the resolved Git with fixed arguments.
	cmd.Dir = dir
	cmd.Env = []string{"HOME=" + home, "PATH=" + filepath.Dir(gitPath), "GIT_CONFIG_NOSYSTEM=1", "LC_ALL=C"}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}
