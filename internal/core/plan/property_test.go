package plan_test

import (
	"fmt"
	"math/rand/v2"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/target"
)

func TestPropertyApplyThenPlanIsEmpty(t *testing.T) {
	t.Parallel()
	const seeds = 100
	for seed := range uint64(seeds) {
		t.Run(strconv.FormatUint(seed, 10), func(t *testing.T) {
			t.Parallel()
			for _, mode := range []state.Kind{state.Link, state.Copy} {
				propertyConvergence(t, seed, mode)
			}
		})
	}
}

func propertyConvergence(t *testing.T, seed uint64, mode state.Kind) {
	t.Helper()
	const maxTargets = 8
	h := fake.New()
	//nolint:gosec // Fixed seeds make randomized planner tests reproducible; no security use.
	rng := rand.New(rand.NewPCG(seed, 15))
	declarations := make([]target.Target, 1+rng.IntN(maxTargets))
	for i := range declarations {
		d := declaration(fmt.Sprintf("skill-%d", i))
		generateTree(t, h, d.SourcePath, rng)
		declarations[i] = d
	}
	req := request(declarations...)
	initial := build(t, h, req)
	if len(initial.Operations) != len(declarations) {
		t.Fatal("initial plan omitted targets")
	}
	applyTestPlan(t, h, &req, initial, mode)
	assertKinds(t, build(t, h, req))
	next := mutateWorld(t, h, declarations, rng)
	req.Adapters = request(next...).Adapters
	p := build(t, h, req)
	for _, op := range p.Operations {
		if op.Kind == plan.Skip {
			t.Fatalf("unexpected conflict for seed %d: %s", seed, p.Text())
		}
	}
	applyTestPlan(t, h, &req, p, mode)
	assertKinds(t, build(t, h, req))
	verifyDesiredContents(t, h, next)
}

func generateTree(t *testing.T, h *fake.Host, source string, rng *rand.Rand) {
	t.Helper()
	const maxFiles = 5
	if rng.IntN(2) == 0 {
		write(t, h, source, strconv.FormatUint(rng.Uint64(), 10))
		return
	}
	if err := h.MkdirAll(filepath.Join(source, "empty"), 0o700); err != nil {
		t.Fatal(err)
	}
	for i := range 1 + rng.IntN(maxFiles) {
		write(t, h, filepath.Join(source, fmt.Sprintf("dir-%d", i), "SKILL.md"), strconv.FormatUint(rng.Uint64(), 10))
	}
}

func mutateWorld(t *testing.T, h *fake.Host, declarations []target.Target, rng *rand.Rand) []target.Target {
	t.Helper()
	const scenarios = 5
	next := make([]target.Target, 0, len(declarations)+1)
	for i, d := range declarations {
		switch rng.IntN(scenarios) {
		case 0: // Repository target disappears; stale state still names its destination.
			removeTestTree(t, h, d.SourcePath)
			continue
		case 1: // Move both source and destination in one plan.
			moved := declaration(fmt.Sprintf("moved-%d", i))
			if err := h.Rename(d.SourcePath, moved.SourcePath); err != nil {
				t.Fatal(err)
			}
			d = moved
		case 2: // Source content changes without local drift.
			changeSource(t, h, d.SourcePath, rng)
		case 3:
			removeTestTree(t, h, d.Path)
		case 4:
		}
		next = append(next, d)
	}
	added := declaration("added")
	added.Backup = target.Once
	write(t, h, added.SourcePath, "new")
	write(t, h, added.Path, "unmanaged original")
	return append(next, added)
}

func verifyDesiredContents(t *testing.T, h *fake.Host, declarations []target.Target) {
	t.Helper()
	for _, d := range declarations {
		source := hashTree(t, h, d.SourcePath)
		// HashTree intentionally refuses the final link; read its source explicitly.
		info, err := h.Lstat(d.Path)
		if err != nil {
			t.Fatal(err)
		}
		dest := d.Path
		if !info.Mode().IsRegular() && !info.IsDir() {
			dest, err = h.ReadLink(d.Path)
			if err != nil {
				t.Fatal(err)
			}
		}
		if source != hashTree(t, h, dest) {
			t.Fatalf("wrong output at %s", d.Path)
		}
	}
}

func changeSource(t *testing.T, h *fake.Host, source string, rng *rand.Rand) {
	t.Helper()
	info, err := h.Lstat(source)
	if err != nil {
		t.Fatal(err)
	}
	if info.IsDir() {
		source = filepath.Join(source, "changed")
	}
	write(t, h, source, strconv.FormatUint(rng.Uint64(), 10))
}
