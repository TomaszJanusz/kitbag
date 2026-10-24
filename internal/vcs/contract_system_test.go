package vcs_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/vcs/system"
)

// testGitConfig is the only Git configuration tests see: the system file is
// disabled and HOME is a temporary directory holding this file.
const testGitConfig = `[user]
	name = KitBag Test
	email = test@kitbag.invalid
[init]
	defaultBranch = main
[commit]
	gpgSign = false
[core]
	autocrlf = false
[advice]
	detachedHead = false
`

// TestSystemContract runs the backend contract against the git on the test
// process's PATH, with a temporary HOME and no system configuration.
func TestSystemContract(t *testing.T) {
	t.Parallel()
	runContract(t, newSystemSubject)
}

func newSystemSubject(t *testing.T) subject {
	t.Helper()
	gitPath, err := exec.LookPath("git")
	mustNil(t, "find git", err)
	home := t.TempDir()
	mustNil(t, "write .gitconfig", os.WriteFile(filepath.Join(home, ".gitconfig"), []byte(testGitConfig), 0o600))
	env := map[string]string{"HOME": home, "PATH": filepath.Dir(gitPath)}

	b, err := system.New(t.Context(), system.Options{Env: fake.New(fake.WithEnv(env)), NoSystemConfig: true})
	mustNil(t, "system backend", err)
	return subject{
		backend: b,
		git: func(t *testing.T, dir string, args ...string) {
			t.Helper()
			cmd := exec.CommandContext(t.Context(), gitPath, args...) //nolint:gosec // G204: test fixture runs the resolved Git with fixed arguments.
			cmd.Dir = dir
			cmd.Env = []string{"HOME=" + home, "PATH=" + env["PATH"], "GIT_CONFIG_NOSYSTEM=1", "LC_ALL=C"}
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %q: %v\n%s", args, err, out)
			}
		},
	}
}
