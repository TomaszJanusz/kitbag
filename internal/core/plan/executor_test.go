package plan_test

import (
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/target"
)

// applyTestPlan is deliberately TEST-ONLY. KIT-16 owns actual execution,
// locking, precondition rechecks, crash recovery, journaling, and permissions.
// It should reuse/replace this model in its own apply/plan property test.
func applyTestPlan(t *testing.T, h *fake.Host, req *plan.Request, p plan.Plan, mode state.Kind) {
	t.Helper()
	for _, op := range p.Operations {
		switch op.Kind {
		case plan.Remove:
			removeTestTree(t, h, op.Path)
			req.State.Targets = slices.DeleteFunc(req.State.Targets, func(s state.Target) bool { return s.Path == op.Path })
		case plan.Backup:
			if err := h.Rename(op.Path, op.BackupPath); err != nil {
				t.Fatal(err)
			}
		case plan.Materialize, plan.Write, plan.Merge, plan.Record:
			applyTestOutput(t, h, op, mode)
			saveTestTarget(req, *op.Desired, mode)
		case plan.Skip:
			// Conflicts are persistent diagnostics; never pretend to resolve them.
		default:
			t.Fatalf("unknown operation %s", op.Kind)
		}
	}
}

func applyTestOutput(t *testing.T, h *fake.Host, op plan.Operation, mode state.Kind) {
	t.Helper()
	d := op.Desired
	if d == nil {
		t.Fatal("missing desired metadata")
	}
	if op.Kind == plan.Record {
		return
	}
	if err := h.MkdirAll(filepath.Dir(d.Path), 0o700); err != nil {
		t.Fatal(err)
	}
	if op.Kind != plan.Materialize {
		write(t, h, d.Path, string(d.Content))
		return
	}
	if mode == state.Link {
		link(t, h, d.SourcePath, d.Path)
		return
	}
	removeTestTree(t, h, d.Path)
	copyTestTree(t, h, d.SourcePath, d.Path)
}

func saveTestTarget(req *plan.Request, d plan.Desired, mode state.Kind) {
	kind := mode
	switch d.Kind {
	case target.Generated:
		kind = state.Generated
	case target.Merged:
		kind = state.Merged
	case target.Materialized:
	}
	s := state.Target{Path: d.Path, Kind: kind, SourcePath: d.SourcePath, SourceHash: d.SourceHash, WrittenHash: d.WrittenHash, Owners: d.Owners}
	for i, old := range req.State.Targets {
		if old.Path == d.Path {
			req.State.Targets[i] = s
			return
		}
	}
	req.State.Targets = append(req.State.Targets, s)
}

func copyTestTree(t *testing.T, h *fake.Host, source, dest string) {
	t.Helper()
	info, err := h.Lstat(source)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		content, readErr := h.ReadFile(source)
		if readErr != nil {
			t.Fatal(readErr)
		}
		write(t, h, dest, string(content))
		return
	}
	if err = h.MkdirAll(dest, 0o700); err != nil {
		t.Fatal(err)
	}
	entries, err := h.ReadDir(source)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		copyTestTree(t, h, filepath.Join(source, entry.Name()), filepath.Join(dest, entry.Name()))
	}
}

func removeTestTree(t *testing.T, h *fake.Host, name string) {
	t.Helper()
	info, err := h.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if info.IsDir() {
		entries, readErr := h.ReadDir(name)
		if readErr != nil {
			t.Fatal(readErr)
		}
		for _, entry := range entries {
			removeTestTree(t, h, filepath.Join(name, entry.Name()))
		}
	}
	if err = h.Remove(name); err != nil {
		t.Fatal(err)
	}
}
