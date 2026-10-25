package apply_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/target"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

type declarations []target.Target

func (d declarations) Declare(_ manifest.FileSystem, _ string, _ *manifest.Manifest) ([]target.Target, error) {
	return d, nil
}

func clock() time.Time                  { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) }
func options(mode string) apply.Options { return apply.Options{Mode: mode, Now: clock} }

func testHost(t *testing.T) (*fake.Host, *state.Store) {
	t.Helper()
	h := fake.New(fake.WithEnv(map[string]string{"HOME": "/home"}))
	store, err := state.New(h, h, state.Options{HistoryLimit: 100})
	if err != nil {
		t.Fatal(err)
	}
	return h, store
}

func declaration(name string) target.Target {
	return target.Target{Path: "/home/instance/" + name, SourcePath: "/repo/" + name, Kind: target.Materialized, Backup: target.Once, Owners: []state.Owner{{Primitive: "skills", AgentInstance: "fixture"}}}
}

func request(ds ...target.Target) plan.Request {
	return plan.Request{Repository: "/repo", Manifest: &manifest.Manifest{Schema: 1}, Adapters: []target.Adapter{declarations(ds)}}
}

func write(t *testing.T, h platform.FS, path, content string) {
	t.Helper()
	must(t, h.MkdirAll(filepath.Dir(path), 0o700))
	must(t, h.WriteAtomic(path, []byte(content), 0o600))
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func run(t *testing.T, h platform.FS, store *state.Store, req plan.Request, opt apply.Options) apply.Result {
	t.Helper()
	result, err := apply.Run(h, store, req, opt)
	must(t, err)
	return result
}

func database(t *testing.T, store *state.Store) state.Database {
	t.Helper()
	var db state.Database
	must(t, store.WithLock(func(session *state.Session) error {
		var err error
		db, err = session.Load()
		return err
	}))
	return db
}

func content(t *testing.T, h platform.FS, path, want string) {
	t.Helper()
	got, err := h.ReadFile(path)
	must(t, err)
	if string(got) != want {
		t.Fatalf("%s = %q; want %q", path, got, want)
	}
}

func assertCode(t *testing.T, result apply.Result, want apply.Code) {
	t.Helper()
	if result.Code() != want {
		t.Fatalf("report = %+v; want code %d", result, want)
	}
}
