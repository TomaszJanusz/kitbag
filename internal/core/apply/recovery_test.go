package apply_test

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/target"
)

func recoveryWorld(t *testing.T, mode string) (*fake.Host, *state.Store, plan.Request) {
	t.Helper()
	h, store := testHost(t)
	old, tree, added := declaration("old"), declaration("tree"), declaration("added")
	generated := declaration("generated")
	generated.Kind = target.Generated
	generated.SourcePath = ""
	generated.Content = []byte("generated")
	write(t, h, old.SourcePath, "old")
	write(t, h, tree.SourcePath+"/a", "tree old")
	assertCode(t, run(t, h, store, request(old, tree), options(mode)), apply.OK)
	write(t, h, tree.SourcePath+"/b", "tree new")
	write(t, h, added.SourcePath, "new")
	write(t, h, added.Path, "unmanaged")
	// Legacy entries are replaced in place, after the tree, without backups.
	legacyLink, legacyOutput := declaration("z-legacy-link"), declaration("z-legacy-output")
	legacyOutput.Kind, legacyOutput.SourcePath = target.Generated, ""
	legacyOutput.Content, legacyOutput.Legacy = []byte("kitbag output"), []byte("legacy output of /repo")
	write(t, h, legacyLink.SourcePath, "relinked")
	must(t, h.Link("/old/z-legacy-link", legacyLink.Path))
	write(t, h, legacyOutput.Path, "legacy output of /old")
	req := request(tree, added, generated, legacyLink, legacyOutput)
	req.LegacyRoots = []string{"/old"}
	return h, store, req
}

func TestEveryStateWriteInterruptionConverges(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"link", "copy"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			clean, cleanStore, req := recoveryWorld(t, mode)
			assertCode(t, run(t, clean, cleanStore, req, options(mode)), apply.OK)
			expected := database(t, cleanStore)
			for _, stage := range []fake.WriteStage{fake.PartialWrite, fake.BeforeRename} {
				testWriteInterruptionPoints(t, mode, stage, expected)
			}
		})
	}
}

func testWriteInterruptionPoints(t *testing.T, mode string, stage fake.WriteStage, expected state.Database) {
	t.Helper()
	const maxWrites = 40
	for n := range maxWrites {
		h, store, req := recoveryWorld(t, mode)
		before := len(database(t, store).History)
		dirs, err := h.Dirs()
		must(t, err)
		must(t, h.InterruptWriteAfter(filepath.Join(dirs.State, "state.json"), stage, n))
		_, err = apply.Run(h, store, req, options(mode))
		if err == nil {
			if n == 0 {
				t.Fatal("fault did not interrupt apply")
			}
			t.Logf("%s stage %d: all %d state-write interruption points converged", mode, stage, n)
			return
		}
		db := database(t, store)
		const writesPerStep = 2
		if len(db.History) != before+n/writesPerStep {
			t.Fatalf("step %d not persisted before next: %+v", n, db)
		}
		assertCode(t, run(t, h, store, req, options(mode)), apply.OK)
		compareRecovered(t, h, store, expected)
		result := run(t, h, store, req, options(mode))
		if result.Code() != apply.OK || len(result.Changed) != 0 {
			t.Fatalf("repeat after %d = %+v", n, result)
		}
	}
	t.Fatal("interruption loop did not reach the clean completion boundary")
}

func compareRecovered(t *testing.T, h *fake.Host, store *state.Store, expected state.Database) {
	t.Helper()
	db := database(t, store)
	if !reflect.DeepEqual(db.Targets, expected.Targets) || db.Pending != nil {
		t.Fatalf("recovered targets differ: %+v\nwant %+v", db, expected)
	}
	content(t, h, "/home/instance/added", "new")
	content(t, h, "/home/instance/tree/a", "tree old")
	content(t, h, "/home/instance/tree/b", "tree new")
	content(t, h, "/home/instance/generated", "generated")
	content(t, h, "/home/instance/z-legacy-link", "relinked")
	content(t, h, "/home/instance/z-legacy-output", "kitbag output")
	entries, err := h.ReadDir("/home/instance")
	must(t, err)
	originals := 0
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".kitbag-stage.") || entry.Name() == "old" || strings.HasPrefix(entry.Name(), "z-legacy-link.") || strings.HasPrefix(entry.Name(), "z-legacy-output.") {
			t.Fatalf("leftover %s", entry.Name())
		}
		if strings.HasPrefix(entry.Name(), "added.before-kitbag.") {
			originals++
			content(t, h, "/home/instance/"+entry.Name(), "unmanaged")
		}
	}
	if originals != 1 {
		t.Fatalf("original backup count = %d", originals)
	}
}

func TestCopyStageWriteInterruptionConverges(t *testing.T) {
	t.Parallel()
	h, store, req := recoveryWorld(t, "copy")
	// Removals, backup, added output, generated output precede the tree update.
	const treeStepIndex = 4
	stagePath := fmt.Sprintf("/home/instance/tree.kitbag-stage.%s-%d/b", clock().Format(apply.BackupTimeLayout), treeStepIndex+2)
	must(t, h.InterruptNextWrite(stagePath, fake.PartialWrite))
	if _, err := apply.Run(h, store, req, options("copy")); err == nil {
		t.Fatal("stage fault did not fire")
	}
	assertCode(t, run(t, h, store, req, options("copy")), apply.OK)
	content(t, h, "/home/instance/tree/b", "tree new")
}
