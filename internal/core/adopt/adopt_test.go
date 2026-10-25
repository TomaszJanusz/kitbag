package adopt_test

import (
	"bytes"
	"errors"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/core/adopt"
	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/core/undo"
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/target"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// markerMerger writes the fragment and renders the legacy marker variant.
type markerMerger struct{}

func (markerMerger) Merge(_, fragment []byte) ([]byte, error) { return bytes.Clone(fragment), nil }
func (markerMerger) Legacy(output []byte) ([]byte, error) {
	return bytes.ReplaceAll(output, []byte("#kitbag"), []byte("#legacy")), nil
}

type declarations []target.Target

func (d declarations) Declare(manifest.FileSystem, string, *manifest.Manifest) ([]target.Target, error) {
	return d, nil
}

func clock() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) }

func owners() []state.Owner { return []state.Owner{{Primitive: "skills", AgentInstance: "test"}} }

func skill(dir, name string) target.Target {
	return target.Target{Path: "/home/" + dir + "/" + name, SourcePath: "/repo/skills/" + name, Kind: target.Materialized, Owners: owners()}
}

func generated(name, content, legacy string) target.Target {
	d := target.Target{Path: "/home/agent/" + name, Kind: target.Generated, Content: []byte(content), Backup: target.Once, Owners: owners()}
	if legacy != "" {
		d.Legacy = []byte(legacy)
	}
	return d
}

type world struct {
	h     *fake.Host
	store *state.Store
	mode  string
	decls []target.Target
	roots []string
}

// newWorld is what agent-config-sync leaves behind, plus obstacles.
func newWorld(t *testing.T) *world {
	t.Helper()
	h := fake.New(fake.WithEnv(map[string]string{"HOME": "/home"}))
	store, err := state.New(h, h, state.Options{HistoryLimit: 100})
	must(t, err)
	for _, name := range []string{"review", "browse", "elsewhere", "real", "missing"} {
		write(t, h, "/repo/skills/"+name+"/SKILL.md", name)
	}
	link(t, h, "/repo/skills/review", "/home/skills/review")
	link(t, h, "../../repo/skills/browse", "/home/skills/browse") // relative
	link(t, h, "/home/old/skills/elsewhere", "/home/skills/elsewhere")
	link(t, h, "/repo/skills/gone", "/home/skills/gone") // removed skill
	write(t, h, "/home/skills/real/SKILL.md", "older copy")
	write(t, h, "/home/agent/LEGACY.md", "<!-- legacy -->\nrules\n")
	write(t, h, "/home/agent/SAME.md", "<!-- kitbag -->\nrules\n")
	write(t, h, "/home/agent/OTHER.md", "hand written\n")
	write(t, h, "/home/agent/settings", "hooks #legacy\n")
	decls := []target.Target{
		skill("skills", "review"), skill("skills", "browse"), skill("skills", "elsewhere"), skill("skills", "real"), skill("skills", "missing"),
		generated("LEGACY.md", "<!-- kitbag -->\nrules\n", "<!-- legacy -->\nrules\n"),
		generated("SAME.md", "<!-- kitbag -->\nrules\n", "<!-- legacy -->\nrules\n"),
		generated("OTHER.md", "<!-- kitbag -->\nrules\n", ""),
		{Path: "/home/agent/settings", Kind: target.Merged, Content: []byte("hooks #kitbag\n"), Merger: markerMerger{}, Backup: target.Once, Owners: owners()},
	}
	return &world{h: h, store: store, decls: decls, roots: []string{"/repo", "/home/old"}}
}

func (w *world) request() plan.Request {
	return plan.Request{Repository: "/repo", Manifest: &manifest.Manifest{Schema: 1}, Adapters: []target.Adapter{declarations(w.decls)}, LegacyRoots: w.roots, Mode: w.mode}
}

func (w *world) adopt(t *testing.T, options adopt.Options) (adopt.Report, error) {
	t.Helper()
	options.Home = "/home"
	var report adopt.Report
	err := w.store.WithLock(func(session *state.Session) error {
		var runErr error
		report, runErr = adopt.Run(w.h, session, w.request(), options)
		return runErr
	})
	return report, err
}

func (w *world) plan(t *testing.T) plan.Plan {
	t.Helper()
	var p plan.Plan
	must(t, w.store.WithLock(func(session *state.Session) error {
		db, err := session.Load()
		if err != nil {
			return err
		}
		req := w.request()
		req.State = db
		p, err = plan.Build(w.h, req)
		return err
	}))
	return p
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

func TestAdoptRecordsOnlyWhatIsInSyncWithoutChanges(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	before := snapshot(t, w.h, "/home")
	report, err := w.adopt(t, adopt.Options{Now: clock})
	must(t, err)
	got := map[string]bool{}
	for _, a := range report.Adopted {
		got[filepath.Base(a.Path)] = a.Legacy
	}
	want := map[string]bool{"review": false, "browse": false, "LEGACY.md": true, "SAME.md": false, "settings": true}
	if !maps.Equal(got, want) {
		t.Fatalf("adopted = %v; want %v", got, want)
	}
	if after := snapshot(t, w.h, "/home"); !maps.Equal(before, after) {
		t.Fatalf("adoption changed files:\n%v\n%v", before, after)
	}
	assertRemaining(t, report)
	db := w.database(t)
	if len(db.Targets) != len(want) || len(db.History) != 1 || !db.History[0].Changes[0].Adopted {
		t.Fatalf("state = %+v", db)
	}
	assertInSync(t, w, want)
	again, err := w.adopt(t, adopt.Options{Now: clock})
	must(t, err)
	if len(again.Adopted) != 0 || len(w.database(t).History) != 1 {
		t.Fatalf("repeat adoption = %+v", again)
	}
}

// assertRemaining checks what the next sync would still do, and the
// undeclared legacy link that adoption leaves alone.
func assertRemaining(t *testing.T, report adopt.Report) {
	t.Helper()
	remaining := map[string]plan.Kind{}
	for _, op := range report.Remaining {
		remaining[filepath.Base(op.Path)] = op.Kind
	}
	want := map[string]plan.Kind{"elsewhere": plan.Materialize, "missing": plan.Materialize, "real": plan.Skip, "OTHER.md": plan.Write}
	for name, kind := range want {
		if remaining[name] != kind {
			t.Fatalf("remaining = %v", remaining)
		}
	}
	if !slices.Equal(report.Undeclared, []string{"/home/skills/gone"}) {
		t.Fatalf("undeclared = %v", report.Undeclared)
	}
}

func assertInSync(t *testing.T, w *world, adopted map[string]bool) {
	t.Helper()
	for _, op := range w.plan(t).Operations {
		if _, owned := adopted[filepath.Base(op.Path)]; owned {
			t.Fatalf("adopted target still planned: %+v", op)
		}
	}
}

func TestAdoptDryRunAndUsage(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	report, err := w.adopt(t, adopt.Options{Now: clock, DryRun: true, MoveRealDirs: true})
	must(t, err)
	if len(report.Adopted) == 0 || !report.DryRun || len(report.Moved) != 1 {
		t.Fatalf("dry run = %+v", report)
	}
	if db := w.database(t); len(db.Targets) != 0 || len(db.History) != 0 {
		t.Fatalf("dry run saved %+v", db)
	}
	content(t, w.h, "/home/skills/real/SKILL.md", "older copy")
	if _, err = w.adopt(t, adopt.Options{}); err == nil {
		t.Fatal("adoption without a clock succeeded")
	}
	w.roots = nil
	if _, err = w.adopt(t, adopt.Options{Now: clock}); !adopt.NoLegacyRoots(err) || !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("missing roots = %v", err)
	}
}

func TestAdoptCopyModeAndVerification(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.mode = "copy"
	write(t, w.h, "/home/copies/review/SKILL.md", "review")
	w.decls = []target.Target{skill("copies", "review"), skill("skills", "review")}
	// A drifted stale ancestor blocks its descendants; adoption must drop them.
	write(t, w.h, "/home/stale/review/SKILL.md", "review")
	w.decls = append(w.decls, skill("stale", "review"))
	db := w.database(t)
	db.Targets = []state.Target{{Path: "/home/stale", Kind: state.Copy, SourcePath: "/repo/skills", SourceHash: hashOf("x"), WrittenHash: hashOf("x"), Owners: owners()}}
	must(t, w.store.WithLock(func(session *state.Session) error { return session.Save(db) }))
	report, err := w.adopt(t, adopt.Options{Now: clock})
	must(t, err)
	if len(report.Adopted) != 1 || report.Adopted[0].Path != "/home/copies/review" || report.Adopted[0].Kind != state.Copy {
		t.Fatalf("adopted = %+v", report.Adopted)
	}
}

func TestMoveRealDirsOnce(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	write(t, w.h, "/home/.agents/skills-before-kitbag/skills/browse/SKILL.md", "earlier")
	must(t, w.h.Remove("/home/skills/browse"))
	write(t, w.h, "/home/skills/browse/SKILL.md", "second copy")
	write(t, w.h, "/outside/review/SKILL.md", "outside")
	w.decls = append(w.decls, target.Target{Path: "/outside/review", SourcePath: "/repo/skills/review", Kind: target.Materialized, Owners: owners()})
	report, err := w.adopt(t, adopt.Options{Now: clock, MoveRealDirs: true})
	must(t, err)
	if len(report.Moved) != 1 || report.Moved[0].Backup != "/home/.agents/skills-before-kitbag/skills/real" || report.Moves.Code() != 3 {
		t.Fatalf("moves = %+v / %+v", report.Moved, report.Moves)
	}
	absent(t, w.h, "/home/skills/real")
	content(t, w.h, "/home/.agents/skills-before-kitbag/skills/real/SKILL.md", "older copy")
	content(t, w.h, "/home/skills/browse/SKILL.md", "second copy")
	content(t, w.h, "/home/.agents/skills-before-kitbag/skills/browse/SKILL.md", "earlier")
	content(t, w.h, "/outside/review/SKILL.md", "outside")
	db := w.database(t)
	move := db.History[0].Changes[0]
	if move.Path != "/home/skills/real" || move.Previous == nil || move.Previous.BackupPath != report.Moved[0].Backup || move.Previous.Managed != nil {
		t.Fatalf("move journal = %+v", db.History)
	}
}

func hashOf(text string) string { return plan.HashBytes([]byte(text)) }

func write(t *testing.T, h platform.FS, path, text string) {
	t.Helper()
	must(t, h.MkdirAll(filepath.Dir(path), 0o700))
	must(t, h.WriteAtomic(path, []byte(text), 0o600))
}

func link(t *testing.T, h platform.FS, target, path string) {
	t.Helper()
	must(t, h.MkdirAll(filepath.Dir(path), 0o700))
	must(t, h.Link(target, path))
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func content(t *testing.T, h platform.FS, path, want string) {
	t.Helper()
	got, err := h.ReadFile(path)
	if err != nil || string(got) != want {
		t.Fatalf("%s = %q, %v; want %q", path, got, err, want)
	}
}

func absent(t *testing.T, h platform.FS, path string) {
	t.Helper()
	if _, err := h.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("%s exists: %v", path, err)
	}
}

// snapshot records every entry below root with its kind and bytes or link,
// except KitBag's own state directory.
func snapshot(t *testing.T, h platform.FS, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	var walk func(string)
	walk = func(path string) {
		if path == "/home/.local" {
			return
		}
		info, err := h.Lstat(path)
		must(t, err)
		switch {
		case info.Mode()&fs.ModeSymlink != 0:
			target, linkErr := h.ReadLink(path)
			must(t, linkErr)
			result[path] = "link " + target
		case info.IsDir():
			result[path] = "dir"
			entries, readErr := h.ReadDir(path)
			must(t, readErr)
			for _, entry := range entries {
				walk(filepath.Join(path, entry.Name()))
			}
		default:
			data, readErr := h.ReadFile(path)
			must(t, readErr)
			result[path] = "file " + string(data)
		}
	}
	walk(root)
	return result
}

// TestAdoptChainThroughLegacyRoot reproduces the macOS layout agent-config-sync
// left: ~/.claude/skills/x -> ~/.agents/skills/x -> <clone>/skills/x. Adoption
// takes the chain over only when the intermediate's directory is a declared
// legacy root, changes nothing, and unmanage later treats it as KitBag's own.
func TestAdoptChainThroughLegacyRoot(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		roots   []string
		adopted []string
	}{
		{"declared root", []string{"/repo", "/home/.agents/skills"}, []string{"/home/.agents/skills/review", "/home/.claude/skills/review"}},
		{"undeclared root", []string{"/repo"}, []string{"/home/.agents/skills/review"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := fake.New(fake.WithEnv(map[string]string{"HOME": "/home"}))
			store, err := state.New(h, h, state.Options{HistoryLimit: 100})
			must(t, err)
			write(t, h, "/repo/skills/review/SKILL.md", "review")
			link(t, h, "/repo/skills/review", "/home/.agents/skills/review")
			link(t, h, "/home/.agents/skills/review", "/home/.claude/skills/review")
			w := &world{h: h, store: store, roots: tc.roots, decls: []target.Target{skill(".agents/skills", "review"), skill(".claude/skills", "review")}}
			before := snapshot(t, h, "/home/.claude")
			report, err := w.adopt(t, adopt.Options{Now: clock})
			must(t, err)
			got := make([]string, 0, len(report.Adopted))
			for _, a := range report.Adopted {
				got = append(got, a.Path)
			}
			if !slices.Equal(got, tc.adopted) || !maps.Equal(before, snapshot(t, h, "/home/.claude")) {
				t.Fatalf("adopted %v, want %v; files %v", got, tc.adopted, snapshot(t, h, "/home/.claude"))
			}
			assertChain(t, w, len(tc.adopted) == 2)
		})
	}
}

// assertChain checks the plan after adoption: an adopted chain is in sync and
// unmanage discards it as unchanged; an unadopted one stays a foreign link.
func assertChain(t *testing.T, w *world, adopted bool) {
	t.Helper()
	p := w.plan(t)
	if !adopted {
		if len(p.Operations) != 1 || p.Operations[0].Kind != plan.Skip {
			t.Fatalf("plan = %+v", p.Operations)
		}
		return
	}
	if len(p.Operations) != 0 {
		t.Fatalf("plan after adoption = %+v", p.Operations)
	}
	var outcome undo.Outcome
	must(t, w.store.WithLock(func(session *state.Session) error {
		var err error
		outcome, err = undo.Unmanage(w.h, session, w.decls, undo.Options{Now: clock, LegacyRoots: w.roots, DryRun: true})
		return err
	}))
	for _, op := range outcome.Plan.Operations {
		if op.Kind != plan.Discard {
			t.Fatalf("unmanage %s = %s %s", op.Path, op.Kind, op.Warning)
		}
	}
}
