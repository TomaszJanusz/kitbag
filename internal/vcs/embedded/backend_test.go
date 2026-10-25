package embedded

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/vcs"
	"github.com/TomaszJanusz/kitbag/internal/vcs/embedded/sshauth"
)

type testWorld struct {
	b                   *Backend
	remote, clone, home string
}

func setup(t *testing.T) testWorld {
	t.Helper()
	home, root := t.TempDir(), t.TempDir()
	b := New(Options{SSH: sshauth.Options{FS: sshTestFS{}}, Env: fake.New(fake.WithEnv(map[string]string{"HOME": home, "PATH": ""})), Identity: vcs.Identity{Name: "Fallback Publisher", Email: "fallback@example.com"}, NoSystemConfig: true})
	remote, clone := filepath.Join(root, "remote.git"), filepath.Join(root, "clone")
	repo, err := git.PlainInit(remote, true)
	must(t, err)
	must(t, repo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName("main"))))
	must(t, b.Clone(t.Context(), remote, clone, vcs.CloneOptions{}))
	return testWorld{b: b, remote: remote, clone: clone, home: home}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func put(t *testing.T, path, data string) {
	t.Helper()
	must(t, os.WriteFile(path, []byte(data), 0o600))
}

func (w testWorld) commit(t *testing.T, message string) string {
	t.Helper()
	hash, err := w.b.Commit(t.Context(), w.clone, vcs.CommitOptions{Message: message})
	must(t, err)
	return hash
}

// No fixture or operation in this test can rely on a Git executable.
func TestWithoutGit(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	w := setup(t)
	put(t, filepath.Join(w.clone, "file"), "first\n")
	hash := w.commit(t, "First")
	must(t, w.b.Push(t.Context(), w.clone, vcs.PushOptions{SetUpstream: true}))
	consumer := filepath.Join(t.TempDir(), "consumer")
	must(t, w.b.Clone(t.Context(), w.remote, consumer, vcs.CloneOptions{}))
	put(t, filepath.Join(w.clone, "file"), "second\n")
	next := w.commit(t, "Second")
	must(t, w.b.Push(t.Context(), w.clone, vcs.PushOptions{}))
	res, err := vcs.Pull(t.Context(), w.b, consumer)
	must(t, err)
	if res.Status.Commit != next || res.Outcome != vcs.PullUpdated {
		t.Fatalf("pull = %+v", res)
	}
	preview, err := w.b.Preview(t.Context(), consumer, hash, next)
	must(t, err)
	if !strings.Contains(preview, "-first\n+second") {
		t.Fatalf("preview = %q", preview)
	}
	repo, err := git.PlainOpen(w.clone)
	must(t, err)
	commit, err := repo.CommitObject(plumbing.NewHash(hash))
	must(t, err)
	if commit.Author.Name != "Fallback Publisher" || commit.Author.Email != "fallback@example.com" {
		t.Fatalf("author = %+v", commit.Author)
	}
}

func TestIdentityMissingAndGitPrecedence(t *testing.T) {
	t.Parallel()
	w := setup(t)
	put(t, filepath.Join(w.clone, "file"), "first")
	w.b.opts.Identity = vcs.Identity{}
	_, err := w.b.Commit(t.Context(), w.clone, vcs.CommitOptions{Message: "Missing"})
	assertUnsupported(t, err, "commit identity missing")
	h, err := w.b.Head(t.Context(), w.clone)
	must(t, err)
	if !h.Unborn() {
		t.Fatalf("missing identity created commit %+v", h)
	}
	put(t, filepath.Join(w.home, ".gitconfig"), "[user]\n name=Git Identity\n email=git@example.com\n")
	hash := w.commit(t, "Configured")
	repo, err := git.PlainOpen(w.clone)
	must(t, err)
	c, err := repo.CommitObject(plumbing.NewHash(hash))
	must(t, err)
	if c.Author.Name != "Git Identity" || c.Author.Email != "git@example.com" {
		t.Fatalf("identity %+v", c.Author)
	}
}

func TestUnsupportedConfiguration(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, config, feature string }{
		{"signing", "[commit]\n gpgSign=true", "signed commits"},
		{"rewrite", "[url \"ssh://elsewhere/\"]\n insteadOf=https://example.com/", "insteadOf protocol rewrites"},
		{"push rewrite", "[url \"ssh://elsewhere/\"]\n pushInsteadOf=https://example.com/", "insteadOf protocol rewrites"},
		{"merge driver", "[merge \"custom\"]\n driver=custom %A %B", "merge drivers"},
		{"rerere", "[rerere]\n enabled=true", "rerere"},
		{"TLS cert", "[http]\n sslCert=client.pem", "TLS client certificates"},
		{"TLS URL cert", "[http \"https://example.com\"]\n sslKey=client.key", "TLS client certificates"},
		{"Kerberos", "[http]\n emptyAuth=true", "Kerberos authentication"},
		{"includes", "[include]\n path=elsewhere", "Git config includes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := setup(t)
			put(t, filepath.Join(w.clone, "file"), "change")
			put(t, filepath.Join(w.home, ".gitconfig"), tc.config)
			_, err := w.b.Commit(t.Context(), w.clone, vcs.CommitOptions{Message: "Unsupported"})
			assertUnsupported(t, err, tc.feature)
		})
	}
}

func TestUnsupportedSSH(t *testing.T) {
	t.Parallel()
	for _, keyword := range []string{"ProxyJump", "ProxyCommand", "GSSAPIAuthentication", "Include"} {
		t.Run(keyword, func(t *testing.T) {
			t.Parallel()
			w := setup(t)
			must(t, os.Mkdir(filepath.Join(w.home, ".ssh"), 0o700))
			put(t, filepath.Join(w.home, ".ssh", "config"), "Host *\n "+keyword+" example\n")
			err := w.b.Clone(t.Context(), "ssh://git@example.com/repo", filepath.Join(t.TempDir(), "clone"), vcs.CloneOptions{})
			if !errors.Is(err, vcs.ErrUnsupported) {
				t.Fatalf("%s = %v", keyword, err)
			}
		})
	}
}

func assertUnsupported(t *testing.T, err error, feature string) {
	t.Helper()
	var unsupported *vcs.UnsupportedError
	if !errors.As(err, &unsupported) || !errors.Is(err, vcs.ErrUnsupported) {
		t.Fatalf("error = %v, want UnsupportedError", err)
	}
	if !strings.Contains(err.Error(), feature) || !strings.Contains(err.Error(), "install git or resolve in the working copy") {
		t.Fatalf("error = %v", err)
	}
}

func TestUnsupportedInProgress(t *testing.T) {
	t.Parallel()
	for _, state := range []string{"MERGE_HEAD", "rebase-merge", "rebase-apply"} {
		t.Run(state, func(t *testing.T) {
			t.Parallel()
			w := setup(t)
			put(t, filepath.Join(w.clone, "file"), "change")
			put(t, filepath.Join(w.clone, ".git", state), "in progress")
			_, err := w.b.Commit(t.Context(), w.clone, vcs.CommitOptions{Message: "Refuse"})
			if !errors.Is(err, vcs.ErrUnsupported) {
				t.Fatalf("commit in %s: %v", state, err)
			}
		})
	}
}

func TestUnsupportedNonFastForward(t *testing.T) {
	t.Parallel()
	w := setup(t)
	put(t, filepath.Join(w.clone, "file"), "base")
	w.commit(t, "Base")
	must(t, w.b.Push(t.Context(), w.clone, vcs.PushOptions{}))
	other := filepath.Join(t.TempDir(), "other")
	must(t, w.b.Clone(t.Context(), w.remote, other, vcs.CloneOptions{}))
	put(t, filepath.Join(other, "local"), "local")
	_, err := w.b.Commit(t.Context(), other, vcs.CommitOptions{Message: "Local"})
	must(t, err)
	put(t, filepath.Join(w.clone, "upstream"), "upstream")
	w.commit(t, "Upstream")
	must(t, w.b.Push(t.Context(), w.clone, vcs.PushOptions{}))
	must(t, w.b.Fetch(t.Context(), other))
	err = w.b.FastForward(t.Context(), other)
	assertUnsupported(t, err, "non-fast-forward merge")
	if !errors.Is(err, vcs.ErrNotFastForward) {
		t.Fatalf("classification = %v", err)
	}
}

// sshTestFS reads only explicitly named test fixtures.
type sshTestFS struct{ platform.FS }

// ReadFile reads an explicit test fixture.
func (sshTestFS) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) } //nolint:gosec // G304: test-created fixtures only.

// Stat reads fixture metadata.
func (sshTestFS) Stat(path string) (os.FileInfo, error) { return os.Stat(path) }
