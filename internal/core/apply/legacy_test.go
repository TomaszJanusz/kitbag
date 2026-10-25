package apply_test

import (
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/target"
)

// siblings lists the entries beside the destination: backups or staging left
// behind would be scanned by agents (a skill link backup is a second skill).
func siblings(t *testing.T, h platform.FS, dir string) []string {
	t.Helper()
	entries, err := h.ReadDir(dir)
	must(t, err)
	var names []string
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".before-kitbag.") || strings.Contains(entry.Name(), ".kitbag-stage.") {
			names = append(names, entry.Name())
		}
	}
	return names
}

func journaled(t *testing.T, db state.Database, path string) *state.Previous {
	t.Helper()
	for _, plan := range db.History {
		for _, change := range plan.Changes {
			if change.Path == path {
				return change.Previous
			}
		}
	}
	t.Fatalf("%s not journaled: %+v", path, db.History)
	return nil
}

func TestReplacedLinksKeepNoBackup(t *testing.T) {
	t.Parallel()
	h, store := testHost(t)
	legacy, owned := declaration("legacy"), declaration("owned")
	write(t, h, legacy.SourcePath, "new")
	write(t, h, owned.SourcePath, "first")
	write(t, h, "/old/agent-config/legacy", "old")
	must(t, h.MkdirAll("/home/instance", 0o700))
	must(t, h.Link("/old/agent-config/legacy", legacy.Path))
	req := request(legacy, owned)
	req.LegacyRoots = []string{"/old/agent-config"}
	assertCode(t, run(t, h, store, req, options("link")), apply.OK)
	// A source move relinks the owned link, which is replaced as well.
	moved := owned
	moved.SourcePath = "/repo/moved"
	write(t, h, moved.SourcePath, "moved")
	req = request(legacy, moved)
	req.LegacyRoots = []string{"/old/agent-config"}
	assertCode(t, run(t, h, store, req, options("link")), apply.OK)
	if names := siblings(t, h, "/home/instance"); len(names) != 0 {
		t.Fatalf("leftovers beside replaced links: %v", names)
	}
	content(t, h, "/old/agent-config/legacy", "old")
	db := database(t, store)
	if prev := journaled(t, db, legacy.Path); prev == nil || prev.Kind != state.Link || prev.LinkTarget != "/old/agent-config/legacy" || prev.Managed != nil {
		t.Fatalf("legacy link journal = %+v", prev)
	}
	if result := run(t, h, store, req, options("link")); len(result.Changed) != 0 {
		t.Fatalf("repeat = %+v", result)
	}
}

// assertReplacedJournal checks that discarded legacy output is journaled as
// previously owned output, and a backed-up file as an unmanaged original.
func assertReplacedJournal(t *testing.T, prev *state.Previous, discarded bool) {
	t.Helper()
	if discarded && (prev.Managed == nil || prev.BackupPath != "" || prev.Kind != state.Generated) {
		t.Fatalf("legacy output journal = %+v", prev)
	}
	if !discarded && (prev.Managed != nil || prev.BackupPath == "") {
		t.Fatalf("hand-edited journal = %+v", prev)
	}
}

func TestLegacyOutputReplacedWithoutBackup(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		current string
		backups int
	}{
		"legacy rendering": {"<!-- agent-config-sync (/old/agent-config) -->\nbody\n", 0},
		"hand-edited":      {"<!-- agent-config-sync (/old/agent-config) -->\nbody\nlocal\n", 1},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h, store := testHost(t)
			d := declaration("CLAUDE.md")
			d.Kind, d.SourcePath = target.Generated, ""
			d.Content = []byte("<!-- kitbag -->\nbody\n")
			d.Legacy = []byte("<!-- agent-config-sync (/repo) -->\nbody\n")
			write(t, h, d.Path, c.current)
			req := request(d)
			req.LegacyRoots = []string{"/old/agent-config"}
			assertCode(t, run(t, h, store, req, options("link")), apply.OK)
			content(t, h, d.Path, "<!-- kitbag -->\nbody\n")
			if names := siblings(t, h, "/home/instance"); len(names) != c.backups {
				t.Fatalf("backups = %v; want %d", names, c.backups)
			}
			assertReplacedJournal(t, journaled(t, database(t, store), d.Path), c.backups == 0)
			if result := run(t, h, store, req, options("link")); len(result.Changed) != 0 {
				t.Fatalf("repeat = %+v", result)
			}
		})
	}
}
