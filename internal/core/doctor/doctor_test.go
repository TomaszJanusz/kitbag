package doctor

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

func subject(t *testing.T) (Service, *fake.Host) {
	t.Helper()
	home := filepath.FromSlash("/home/doctor")
	host := fake.New(fake.WithEnv(map[string]string{"HOME": home}))
	repo := filepath.Join(home, "checkout")
	must(t, host.MkdirAll(repo, probeDirPerm))
	must(t, host.WriteAtomic(filepath.Join(repo, "kitbag.toml"), []byte("schema=1\n"), probeFilePerm))
	store, err := state.New(host, host, state.Options{})
	must(t, err)
	backend := &diagnosticBackend{cfg: vcs.CommitConfig{Identity: true}}
	return Service{Host: host, Store: store, Repository: repo, Version: "1.0.0", Open: func(context.Context) (vcs.Backend, vcs.Resolution, error) {
		return backend, vcs.Resolution{Selection: vcs.Selection{Kind: vcs.Auto, Source: vcs.SourceDefault}, Active: vcs.System, Version: "git 2.50.0"}, nil
	}, VersionProbe: func(context.Context, string) (string, error) { return "1.0.0", nil }}, host
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

type diagnosticBackend struct {
	vcs.Backend
	remoteErr error
	cfg       vcs.CommitConfig
	calls     int
}

// Kind identifies the fake diagnostic backend.
func (*diagnosticBackend) Kind() vcs.Kind { return vcs.System }

// ProbeRemote records calls without a network.
func (b *diagnosticBackend) ProbeRemote(context.Context, string) error { b.calls++; return b.remoteErr }

// InspectCommit returns configured presence flags.
func (b *diagnosticBackend) InspectCommit(context.Context, string) (vcs.CommitConfig, error) {
	return b.cfg, nil
}

func find(t *testing.T, r Report, id string) Check {
	t.Helper()
	for _, check := range r.Checks {
		if check.ID == id {
			return check
		}
	}
	t.Fatalf("missing check %s: %+v", id, r)
	return Check{}
}

func TestRegistryAndOffline(t *testing.T) {
	t.Parallel()
	s, host := subject(t)
	s.Offline = true
	backend, _, err := s.Open(t.Context())
	must(t, err)
	b, ok := backend.(*diagnosticBackend)
	if !ok {
		t.Fatal("wrong test backend")
	}
	b.remoteErr = errors.New("unreachable")
	first, second := s.Run(t.Context()), s.Run(t.Context())
	if !reflect.DeepEqual(first, second) {
		t.Fatal("non-deterministic report", first, second)
	}
	if b.calls != 0 || find(t, first, "remote").Status != OK {
		t.Fatal("offline report performed network probe")
	}
	dirs, err := host.Dirs()
	must(t, err)
	entries, err := host.ReadDir(dirs.Cache)
	must(t, err)
	if len(entries) != 0 {
		t.Fatal("probe left scratch", entries)
	}
	s.Offline = false
	r := s.Run(t.Context())
	if b.calls != 1 || find(t, r, "remote").Status != Fail || r.Code(false) != apply.Warnings {
		t.Fatal("unreachable remote was not a failure", r)
	}
}

func TestStateContentionAndRelease(t *testing.T) {
	t.Parallel()
	s, _ := subject(t)
	must(t, s.Store.WithLock(func(*state.Session) error {
		r := s.Run(t.Context())
		if find(t, r, "state-lock").Status != Fail || r.Code(false) != apply.Warnings {
			t.Fatal("held lock reported available", r)
		}
		return nil
	}))
	r := s.Run(t.Context())
	if find(t, r, "state-lock").Status != OK || find(t, r, "state-health").Status != OK {
		t.Fatal("lock was not released", r)
	}
}

func TestExitCodes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		status Status
		id     string
		strict bool
		want   apply.Code
	}{
		{OK, "remote", true, apply.OK}, {Warn, "agent-codex", false, apply.OK}, {Warn, "agent-codex", true, apply.Warnings}, {Fail, "remote", false, apply.Warnings}, {Fail, "tool-version", false, apply.ToolTooOld},
	} {
		t.Run(string(tc.status)+tc.id+map[bool]string{false: "normal", true: "strict"}[tc.strict], func(t *testing.T) {
			t.Parallel()
			r := Report{Checks: []Check{{ID: tc.id, Status: tc.status}}}
			if got := r.Code(tc.strict); got != tc.want {
				t.Fatalf("code=%d want %d", got, tc.want)
			}
		})
	}
}

func TestCopyAndLinkFailures(t *testing.T) {
	t.Parallel()
	s, host := subject(t)
	broken := noLinkHost{Host: host}
	s.Host = broken
	r := s.Run(t.Context())
	if find(t, r, "copy").Status != OK || find(t, r, "link").Status != Fail {
		t.Fatal("link failure hidden", r)
	}
	s.ModeFlag = "copy"
	if got := find(t, s.Run(t.Context()), "link"); got.Status != Warn || got.Fix == "" {
		t.Fatal(got)
	}
}

type noLinkHost struct{ *fake.Host }

// Link simulates a filesystem without symbolic links.
func (noLinkHost) Link(string, string) error { return fs.ErrPermission }
