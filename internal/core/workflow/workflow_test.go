package workflow_test

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/config"
	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/core/workflow"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/target"
	"github.com/TomaszJanusz/kitbag/internal/vcs"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

type backend struct {
	vcs.Backend
	fetch             func() error
	status            vcs.Status
	fetches, forwards int
}

func (b *backend) Status(context.Context, string) (vcs.Status, error) { return b.status, nil }
func (b *backend) Fetch(context.Context, string) error {
	b.fetches++
	if b.fetch != nil {
		return b.fetch()
	}
	return nil
}

func (b *backend) FastForward(context.Context, string) error {
	b.forwards++
	b.status.Behind = 0
	return nil
}

type fixture struct {
	host  *fake.Host
	git   *backend
	s     workflow.Service
	calls int
}

func setup(t *testing.T) *fixture {
	t.Helper()
	root := string(filepath.Separator)
	home := filepath.Join(root, "home", "test")
	h := fake.New(fake.WithEnv(map[string]string{"HOME": home}))
	store, err := state.New(h, h, state.Options{})
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{host: h, git: &backend{status: vcs.Status{Head: vcs.Head{Branch: "main", Commit: "abc"}, Upstream: "origin/main"}}}
	f.s = workflow.Service{Host: h, Backend: f.git, Store: store, Repository: filepath.Join(home, "repo"), Version: "0.1.0", Now: func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) }}
	f.s.Adapters = func(_ *manifest.Manifest, _ *[]string) ([]target.Adapter, error) {
		f.calls++
		return []target.Adapter{fileAdapter{home: home}}, nil
	}
	f.write(t, "kitbag.toml", "schema=1\n[[files]]\nsrc='source'\ndest='~/output'")
	f.write(t, "source", "first")
	return f
}

func (f *fixture) write(t *testing.T, name, body string) {
	t.Helper()
	write(t, f.host, filepath.Join(f.s.Repository, name), body)
}

func write(t *testing.T, h *fake.Host, path, body string) {
	t.Helper()
	if err := h.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := h.WriteAtomic(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

type fileAdapter struct{ home string }

func (a fileAdapter) Declare(_ manifest.FileSystem, repo string, m *manifest.Manifest) ([]target.Target, error) {
	ds := make([]target.Target, 0, len(m.Files))
	for _, file := range m.Files {
		ds = append(ds, target.Target{Path: config.Expand(a.home, file.Dest), SourcePath: filepath.Join(repo, file.Src), Kind: target.Materialized, Backup: target.Once, Owners: []state.Owner{{Primitive: "files", AgentInstance: "fixture"}}})
	}
	return ds, nil
}

func syncOK(t *testing.T, f *fixture, noPull bool) workflow.SyncResult {
	t.Helper()
	r, err := f.s.Sync(context.Background(), noPull, false)
	if err != nil || r.Code() != apply.OK {
		t.Fatalf("sync: %+v, %v", r, err)
	}
	return r
}

func TestSyncFreshTreeOnceAndRepeat(t *testing.T) {
	t.Parallel()
	f := setup(t)
	f.git.fetch = func() error {
		f.write(t, "kitbag.toml", "schema=1\n[[files]]\nsrc='new'\ndest='~/new-output'")
		f.write(t, "new", "new tree")
		f.git.status.Behind = 1
		return nil
	}
	result := syncOK(t, f, false)
	if len(result.Apply.Changed) != 1 || f.calls != 1 || f.git.forwards != 1 {
		t.Fatalf("must plan/apply once: %+v, calls %d", result, f.calls)
	}
	if _, err := f.host.Lstat(filepath.Join(filepath.Dir(f.s.Repository), "output")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("old manifest applied: %v", err)
	}
	f.git.fetch = nil
	repeat := syncOK(t, f, false)
	if len(repeat.Apply.Changed) != 0 || f.calls != 2 {
		t.Fatalf("repeat: %+v, calls %d", repeat, f.calls)
	}
	assertLastSync(t, f)
}

func TestPullPolicies(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name                    string
		noPull, dirty, diverged bool
		outcome                 vcs.PullOutcome
		code                    apply.Code
		fetches, calls          int
	}{
		{name: "no pull", noPull: true, calls: 1},
		{name: "dirty", dirty: true, outcome: vcs.PullSkippedDirty, calls: 1},
		{name: "diverged", diverged: true, outcome: vcs.PullDiverged, code: apply.Conflict, fetches: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := setup(t)
			if tc.dirty {
				f.git.status.Changes = []vcs.Change{{Path: "untracked", Kind: vcs.ChangeUntracked}}
			}
			if tc.diverged {
				f.git.status.Ahead = 1
				f.git.status.Behind = 1
			}
			r, err := f.s.Sync(context.Background(), tc.noPull, false)
			if err != nil || r.Code() != tc.code || r.Pull != tc.outcome || f.git.fetches != tc.fetches || f.calls != tc.calls {
				t.Fatalf("result %+v, %v; fetch %d, calls %d", r, err, f.git.fetches, f.calls)
			}
		})
	}
}

func TestSyncFailureRecords(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		mutate func(*testing.T, *fixture)
		code   apply.Code
	}{
		"fetch": {func(_ *testing.T, f *fixture) { f.git.fetch = func() error { return fs.ErrPermission } }, apply.Warnings},
		"version": {func(t *testing.T, f *fixture) {
			t.Helper()
			f.write(t, "kitbag.toml", "schema=1\nmin_tool_version='9.0.0'")
		}, apply.ToolTooOld},
		"invalid manifest": {func(t *testing.T, f *fixture) { t.Helper(); f.write(t, "kitbag.toml", "schema=2") }, apply.Warnings},
		"mode":             {func(_ *testing.T, f *fixture) { f.s.ModeFlag = "wrong" }, apply.Usage},
		"adapter": {func(_ *testing.T, f *fixture) {
			f.s.Adapters = func(*manifest.Manifest, *[]string) ([]target.Adapter, error) { return nil, fs.ErrPermission }
		}, apply.Warnings},
		"state": {func(t *testing.T, f *fixture) {
			t.Helper()
			dirs, err := f.host.Dirs()
			if err != nil {
				t.Fatal(err)
			}
			write(t, f.host, filepath.Join(dirs.State, "state.json"), "invalid")
		}, apply.Warnings},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := setup(t)
			tc.mutate(t, f)
			r, err := f.s.Sync(context.Background(), false, false)
			if err == nil || max(r.Code(), apply.ErrorCode(err)) != tc.code {
				t.Fatalf("result %+v, error %v", r, err)
			}
			if err = f.s.Store.WithLock(func(s *state.Session) error {
				record, exists, loadErr := s.LoadSync()
				if loadErr == nil && (!exists || record.Result != int(tc.code)) {
					t.Errorf("record %+v", record)
				}
				return loadErr
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestInspectStatusAndWarnings(t *testing.T) {
	t.Parallel()
	f := setup(t)
	f.s.ModeFlag = "copy"
	syncOK(t, f, true)
	output := filepath.Join(filepath.Dir(f.s.Repository), "output")
	write(t, f.host, output, "local drift")
	f.git.status.Ahead = 2
	f.git.status.Behind = 3
	f.git.status.Changes = []vcs.Change{{Path: "source", Kind: vcs.ChangeModified}}
	result, err := f.s.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Git.Ahead != 2 || result.Git.Behind != 3 || !result.Git.Dirty || result.Drift[plan.LocalDrift] != 1 || len(result.Warnings) != 1 || f.git.fetches != 0 {
		t.Fatalf("status: %+v", result)
	}
	assertDriftSync(t, f)
}

func TestIncludesAndAdapterWarnings(t *testing.T) {
	t.Parallel()
	f := setup(t)
	f.write(t, "gitconfig", "[alias]")
	f.write(t, "kitbag.toml", "schema=1\n[git]\ninclude=['gitconfig']")
	original := f.s.Adapters
	f.s.Adapters = func(m *manifest.Manifest, w *[]string) ([]target.Adapter, error) {
		*w = append(*w, "host addition missing")
		return original(m, w)
	}
	r, err := f.s.Sync(context.Background(), true, false)
	if err != nil || r.Code() != apply.Warnings || len(r.Includes.Warnings) != 1 || len(r.Warnings) != 1 {
		t.Fatalf("includes unavailable: %+v, %v", r, err)
	}
	git := &includeBackend{backend: f.git}
	f.s.Backend = git
	r, err = f.s.Sync(context.Background(), true, false)
	if err != nil || len(r.Includes.Added) != 1 {
		t.Fatalf("includes: %+v, %v", r, err)
	}
}

type includeBackend struct{ *backend }

func (*includeBackend) GlobalConfigFiles() []string {
	return []string{filepath.Join(string(filepath.Separator), "home", "test", ".gitconfig")}
}
func (*includeBackend) GlobalIncludes(context.Context) ([]string, error) { return nil, nil }
func (*includeBackend) AddGlobalInclude(context.Context, string) error   { return nil }

func TestHostFlagAndLegacyRoots(t *testing.T) {
	t.Parallel()
	f := setup(t)
	f.s.HostFlag = "chosen"
	f.s.Config.Host = "other"
	f.s.Config.Migration.LegacyRoots = []string{"~/legacy"}
	f.write(t, "kitbag.toml", "schema=1\n[[files]]\nsrc='source'\ndest='~/chosen'\nhosts=['chosen']")
	r := syncOK(t, f, true)
	if len(r.Apply.Changed) != 1 {
		t.Fatalf("host flag not used: %+v", r)
	}
}

func assertLastSync(t *testing.T, f *fixture) {
	t.Helper()
	status, err := f.s.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.LastSync == nil || status.LastSync.Result != 0 || status.LastSync.Changed != 0 || status.Drift[plan.InSync] != 1 {
		t.Fatalf("status: %+v", status)
	}
}

func assertDriftSync(t *testing.T, f *fixture) {
	t.Helper()
	r, err := f.s.Sync(context.Background(), true, false)
	if err != nil || r.Code() != apply.Warnings {
		t.Fatalf("drift: %+v, %v", r, err)
	}
	f.git.status.Upstream = ""
	result, err := f.s.Status(context.Background())
	if err != nil || len(result.Warnings) != 2 {
		t.Fatalf("no upstream: %+v, %v", result, err)
	}
}

func TestSyncProgressPhases(t *testing.T) {
	t.Parallel()
	f := setup(t)
	var phases []string
	f.s.Progress = func(phase string) { phases = append(phases, phase) }
	syncOK(t, f, false)
	want := []string{"pull", "plan", "apply", "includes"}
	if !slices.Equal(phases, want) {
		t.Fatalf("progress=%v, want %v", phases, want)
	}
	phases = nil
	f.git.fetch = func() error { return fs.ErrPermission }
	if _, err := f.s.Sync(t.Context(), false, false); err == nil {
		t.Fatal("fetch unexpectedly succeeded")
	}
	if !slices.Equal(phases, []string{"pull"}) {
		t.Fatalf("failed sync emitted later phases: %v", phases)
	}
}
