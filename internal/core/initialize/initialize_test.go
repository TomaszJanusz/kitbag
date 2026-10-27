package initialize

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/adapters/files"
	"github.com/TomaszJanusz/kitbag/internal/config"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/target"
	"github.com/TomaszJanusz/kitbag/internal/vcs"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

type testBackend struct {
	vcs.Backend
	failure        error
	host           *fake.Host
	origin, branch string
	clones         int
	identity       bool
}

func (b *testBackend) Clone(_ context.Context, url, dir string, opts vcs.CloneOptions) error {
	b.clones++
	b.origin, b.branch = url, opts.Branch
	if b.failure != nil {
		return b.failure
	}
	if err := b.host.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return b.host.WriteAtomic(filepath.Join(dir, "kitbag.toml"), []byte("schema=1\n"), 0o600)
}

func (b *testBackend) RemoteURL(context.Context, string, string) (string, error) {
	return b.origin, b.failure
}

func (b *testBackend) Head(context.Context, string) (vcs.Head, error) {
	return vcs.Head{Branch: b.branch}, b.failure
}

func (b *testBackend) InspectCommit(context.Context, string) (vcs.CommitConfig, error) {
	return vcs.CommitConfig{Identity: b.identity}, b.failure
}
func (b *testBackend) Init(context.Context, string) error { return b.failure }

func setup(t *testing.T) (Service, *testBackend) {
	t.Helper()
	home := filepath.Join(string(filepath.Separator), "home", "test")
	host := fake.New(fake.WithEnv(map[string]string{"HOME": home}))
	backend := &testBackend{host: host}
	cfg := config.Config{Schema: 1}
	cfg.Repo.URL, cfg.Repo.Branch = "https://example.com/config.git", "main"
	filename, err := ConfigPath(host, "")
	must(t, err)
	s := Service{Host: host, Backend: backend, Config: cfg, Filename: filename, Version: "0.1.0"}
	s.Adapters = func(_ *manifest.Manifest, _ *[]string) ([]target.Adapter, error) {
		return []target.Adapter{files.New(home)}, nil
	}
	return s, backend
}

func TestClonePlanSaveAndRepeat(t *testing.T) {
	t.Parallel()
	s, git := setup(t)
	repo, reused, err := s.Clone(context.Background())
	if err != nil || reused || git.clones != 1 {
		t.Fatalf("clone: %q %t %v", repo, reused, err)
	}
	assertDryPlan(t, s, repo)
	err = s.Save()
	must(t, err)
	loaded, err := config.Load(s.Host, s.Filename)
	if err != nil || loaded.Repo.URL != s.Config.Repo.URL {
		t.Fatalf("saved config: %+v %v", loaded, err)
	}
	if err = s.Save(); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("clobber: %v", err)
	}
	_, reused, err = s.Clone(context.Background())
	if err != nil || !reused || git.clones != 1 {
		t.Fatalf("repeat: %t %v clones=%d", reused, err, git.clones)
	}
}

func TestCloneRefusesInvalidAndForeign(t *testing.T) {
	t.Parallel()
	for _, test := range []string{"invalid", "clone error", "file", "symlink", "origin", "branch", "backend error"} {
		t.Run(test, func(t *testing.T) {
			t.Parallel()
			s, git := setup(t)
			arrangeInvalidClone(t, &s, git, test)
			var err error
			if _, _, err = s.Clone(context.Background()); err == nil {
				t.Fatal("accepted unsafe setup")
			}
		})
	}
}

func TestIdentityAndInspectionErrors(t *testing.T) {
	t.Parallel()
	s, git := setup(t)
	repo, _, err := s.Clone(context.Background())
	must(t, err)
	for _, identity := range []bool{true, false} {
		git.identity = identity
		needed, identityErr := s.NeedsIdentity(context.Background(), repo)
		if identityErr != nil || needed == identity {
			t.Fatalf("identity: %t %v", needed, identityErr)
		}
	}
	git.failure = fs.ErrPermission
	if _, err = s.NeedsIdentity(context.Background(), repo); !errors.Is(err, fs.ErrPermission) {
		t.Fatal(err)
	}
	write(t, s.Host, filepath.Join(repo, "kitbag.toml"), "schema=3")
	if _, err = s.Inspect(repo); err == nil {
		t.Fatal("invalid manifest accepted")
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func assertDryPlan(t *testing.T, s Service, repo string) {
	t.Helper()
	write(t, s.Host, filepath.Join(repo, "kitbag.toml"), "schema=1\n[[files]]\nsrc='source'\ndest='~/output'\n")
	write(t, s.Host, filepath.Join(repo, "source"), "hello")
	inspection, err := s.Inspect(repo)
	if err != nil || len(inspection.Plan.Operations) != 1 {
		t.Fatalf("plan: %+v %v", inspection, err)
	}
	home, err := s.Host.HomeDir()
	must(t, err)
	if exists, existsErr := Exists(s.Host, filepath.Join(home, "output")); exists || existsErr != nil {
		t.Fatalf("plan mutated destination: %v", existsErr)
	}
}

func arrangeInvalidClone(t *testing.T, s *Service, git *testBackend, test string) {
	t.Helper()
	repo, err := s.Config.WorkingCopy(s.Host)
	must(t, err)
	switch test {
	case "invalid":
		s.Config.Schema = 2
	case "clone error":
		git.failure = fs.ErrPermission
	case "file":
		write(t, s.Host, repo, "occupied")
	case "symlink":
		err = s.Host.MkdirAll(filepath.Dir(repo), 0o700)
		must(t, err)
		err = s.Host.Link(filepath.Join(filepath.Dir(repo), "other"), repo)
		must(t, err)
	default:
		if _, _, err = s.Clone(context.Background()); err != nil {
			t.Fatal(err)
		}
		switch test {
		case "origin":
			git.origin = "different"
		case "branch":
			git.branch = "different"
		case "backend error":
			git.failure = fs.ErrPermission
		}
	}
}
