package undo_test

import (
	"bytes"
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/core/undo"
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/target"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

const (
	managed = " #managed"
	legacy  = " #legacy"
)

// lineMerger keeps local lines, replaces lines it marked and can unmerge them.
type lineMerger struct{}

func (lineMerger) Merge(current, fragment []byte) ([]byte, error) {
	var out []string
	for _, line := range lines(current) {
		if !strings.HasSuffix(line, managed) && !strings.HasSuffix(line, legacy) {
			out = append(out, line)
		}
	}
	for _, line := range lines(fragment) {
		out = append(out, line+managed)
	}
	return []byte(strings.Join(out, "\n") + "\n"), nil
}

func (lineMerger) Legacy(output []byte) ([]byte, error) {
	return bytes.ReplaceAll(output, []byte(managed), []byte(legacy)), nil
}

func (lineMerger) Unmerge(current []byte) ([]byte, error) {
	if bytes.Contains(current, []byte("unparsable")) {
		return nil, errors.New("unparsable")
	}
	var out []string
	for _, line := range lines(current) {
		if !strings.HasSuffix(line, managed) && !strings.HasSuffix(line, legacy) {
			out = append(out, line)
		}
	}
	if len(out) == len(lines(current)) {
		return current, nil
	}
	return []byte(strings.Join(out, "\n") + "\n"), nil
}

func lines(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

type declarations []target.Target

func (d declarations) Declare(manifest.FileSystem, string, *manifest.Manifest) ([]target.Target, error) {
	return d, nil
}

func clock() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) }

func owners() []state.Owner { return []state.Owner{{Primitive: "skills", AgentInstance: "test"}} }

func linkDecl(name string) target.Target {
	return target.Target{Path: "/home/a/" + name, SourcePath: "/repo/" + name, Kind: target.Materialized, Owners: owners()}
}

func fileDecl(name string) target.Target {
	d := linkDecl(name)
	d.Backup = target.Once
	return d
}

func generated(name, content string) target.Target {
	return target.Target{Path: "/home/a/" + name, Kind: target.Generated, Content: []byte(content), Backup: target.Once, Owners: owners()}
}

func merged(fragment string) target.Target {
	return target.Target{Path: "/home/a/settings", Kind: target.Merged, Content: []byte(fragment), Merger: lineMerger{}, Backup: target.Once, Owners: owners()}
}

type world struct {
	h     *fake.Host
	store *state.Store
	decls []target.Target
}

func newWorld(t *testing.T) *world {
	t.Helper()
	h := fake.New(fake.WithEnv(map[string]string{"HOME": "/home"}))
	store, err := state.New(h, h, state.Options{HistoryLimit: 100})
	must(t, err)
	w := &world{h: h, store: store}
	write(t, h, "/repo/skill/SKILL.md", "skill")
	write(t, h, "/repo/tool", "tool")
	write(t, h, "/home/a/tool", "mine")
	write(t, h, "/home/a/AGENTS.md", "own rules\n")
	write(t, h, "/home/a/settings", "local-key\n")
	w.decls = []target.Target{linkDecl("skill"), fileDecl("tool"), generated("AGENTS.md", "generated\n"), generated("CODEX.md", "codex\n"), merged("repo-key\n")}
	return w
}

func (w *world) request() plan.Request {
	return plan.Request{Repository: "/repo", Manifest: &manifest.Manifest{Schema: 1}, Adapters: []target.Adapter{declarations(w.decls)}}
}

func (w *world) sync(t *testing.T) apply.Result {
	t.Helper()
	result, err := apply.Run(w.h, w.store, w.request(), apply.Options{Now: clock})
	must(t, err)
	if result.Code() != apply.OK {
		t.Fatalf("sync = %+v", result)
	}
	return result
}

func (w *world) unmanage(t *testing.T, fsys platform.FS, options undo.Options) (undo.Outcome, error) {
	t.Helper()
	req := w.request()
	req.State = state.Database{SchemaVersion: state.CurrentVersion}
	decls, err := plan.Declarations(w.h, req)
	must(t, err)
	var outcome undo.Outcome
	err = w.store.WithLock(func(session *state.Session) error {
		var runErr error
		outcome, runErr = undo.Unmanage(fsys, session, decls, options)
		return runErr
	})
	return outcome, err
}

func (w *world) revert(fsys platform.FS, to int, options undo.Options) (undo.Outcome, error) {
	var outcome undo.Outcome
	err := w.store.WithLock(func(session *state.Session) error {
		var runErr error
		outcome, runErr = undo.Revert(fsys, session, to, options)
		return runErr
	})
	return outcome, err
}

func (w *world) database(t *testing.T) state.Database {
	t.Helper()
	var db state.Database
	must(t, w.store.WithLock(func(session *state.Session) error {
		var err error
		db, err = session.Load()
		return err
	}))
	return db
}

func run(options undo.Options) undo.Options {
	options.Now = clock
	return options
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

func content(t *testing.T, h platform.FS, path, want string) {
	t.Helper()
	info, err := h.Lstat(path)
	must(t, err)
	if info.Mode()&fs.ModeSymlink != 0 {
		t.Fatalf("%s is a link", path)
	}
	got, err := h.ReadFile(path)
	must(t, err)
	if string(got) != want {
		t.Fatalf("%s = %q; want %q", path, got, want)
	}
}

func absent(t *testing.T, h platform.FS, path string) {
	t.Helper()
	if _, err := h.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("%s exists: %v", path, err)
	}
}

func linked(t *testing.T, h platform.FS, path, want string) {
	t.Helper()
	got, err := h.ReadLink(path)
	if err != nil || got != want {
		t.Fatalf("%s -> %q, %v; want %q", path, got, err, want)
	}
}

// snapshot records every entry below root: kind, file hash or link text.
func snapshot(t *testing.T, h platform.FS, root string) map[string]state.Entry {
	t.Helper()
	result := map[string]state.Entry{}
	var walk func(string)
	walk = func(path string) {
		info, err := h.Lstat(path)
		must(t, err)
		switch {
		case info.Mode()&fs.ModeSymlink != 0:
			link, linkErr := h.ReadLink(path)
			must(t, linkErr)
			result[path] = state.Entry{Kind: "link", Link: link}
		case info.IsDir():
			result[path] = state.Entry{Kind: "directory"}
			children, readErr := h.ReadDir(path)
			must(t, readErr)
			for _, child := range children {
				walk(filepath.Join(path, child.Name()))
			}
		default:
			hash, hashErr := plan.HashTree(h, path)
			must(t, hashErr)
			result[path] = state.Entry{Kind: "file", Hash: hash}
		}
	}
	walk(root)
	return result
}

func names(t *testing.T, h platform.FS, dir string) []string {
	t.Helper()
	entries, err := h.ReadDir(dir)
	must(t, err)
	result := make([]string, 0, len(entries))
	for _, entry := range entries {
		result = append(result, entry.Name())
	}
	return result
}

func kinds(p plan.Plan) map[string]plan.Kind {
	result := map[string]plan.Kind{}
	for _, op := range p.Operations {
		result[filepath.Base(op.Path)] = op.Kind
	}
	return result
}

func noReserved(t *testing.T, h platform.FS, dir string) {
	t.Helper()
	if slices.ContainsFunc(names(t, h, dir), func(n string) bool {
		return strings.Contains(n, ".kitbag-stage.") || strings.Contains(n, ".before-kitbag.")
	}) {
		t.Fatalf("reserved entries left in %s: %v", dir, names(t, h, dir))
	}
}

// crashFS fails one mutation (before or after it took effect) and then all.
type crashFS struct {
	platform.FS
	remaining int
}

func (c *crashFS) mutation(fn func() error) error {
	if c.remaining == 0 {
		return errors.New("injected interruption")
	}
	if err := fn(); err != nil {
		return err
	}
	c.remaining--
	if c.remaining == 0 {
		return errors.New("injected post-mutation interruption")
	}
	return nil
}

func (c *crashFS) MkdirAll(path string, mode fs.FileMode) error {
	return c.mutation(func() error { return c.FS.MkdirAll(path, mode) })
}

func (c *crashFS) WriteAtomic(path string, data []byte, mode fs.FileMode) error {
	return c.mutation(func() error { return c.FS.WriteAtomic(path, data, mode) })
}

func (c *crashFS) Link(target, path string) error {
	return c.mutation(func() error { return c.FS.Link(target, path) })
}

func (c *crashFS) Rename(old, path string) error {
	return c.mutation(func() error { return c.FS.Rename(old, path) })
}

func (c *crashFS) Remove(path string) error {
	return c.mutation(func() error { return c.FS.Remove(path) })
}
