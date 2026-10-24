package plan_test

import (
	"math/rand/v2"
	"reflect"
	"slices"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/target"
)

func TestShuffledInputsHaveIdenticalPlans(t *testing.T) {
	t.Parallel()
	const iterations = 64
	h := fake.New()
	a, b, c := declaration("a"), declaration("b"), declaration("c")
	a.Owners = append(a.Owners, state.Owner{Primitive: "skills", AgentInstance: "other"})
	for _, d := range []target.Target{a, b, c} {
		write(t, h, d.SourcePath, "desired")
	}
	a.Backup = target.Once
	write(t, h, a.Path, "unmanaged")
	req := request(a, b, c)
	staleA, staleB := declaration("stale-a"), declaration("stale-b")
	req.State.Targets = []state.Target{tracked(staleA, plan.HashBytes(nil), state.Copy), tracked(staleB, plan.HashBytes(nil), state.Copy)}
	expected := build(t, h, req)
	assertKinds(t, expected, plan.Remove, plan.Remove, plan.Backup, plan.Materialize, plan.Materialize, plan.Materialize)
	//nolint:gosec // Fixed seeds make randomized planner tests reproducible; no security use.
	rng := rand.New(rand.NewPCG(15, 3))
	for range iterations {
		declarations := []target.Target{a, b, c}
		rng.Shuffle(len(declarations), func(i, j int) { declarations[i], declarations[j] = declarations[j], declarations[i] })
		declarations[0].Owners = slices.Clone(declarations[0].Owners)
		slices.Reverse(declarations[0].Owners)
		req.Adapters = []target.Adapter{fakeAdapter(declarations[:1]), fakeAdapter(declarations[1:])}
		rng.Shuffle(len(req.Adapters), func(i, j int) { req.Adapters[i], req.Adapters[j] = req.Adapters[j], req.Adapters[i] })
		rng.Shuffle(len(req.State.Targets), func(i, j int) {
			req.State.Targets[i], req.State.Targets[j] = req.State.Targets[j], req.State.Targets[i]
		})
		got := build(t, h, req)
		if !reflect.DeepEqual(got, expected) {
			t.Fatalf("unstable plan: %v\n%v", got, expected)
		}
	}
}

func TestSharedDestinationUnionsOwnersWithoutMutatingInputs(t *testing.T) {
	t.Parallel()
	h := fake.New()
	a, b := declaration("shared"), declaration("shared")
	b.Owners = []state.Owner{{Primitive: "skills", AgentInstance: "second"}}
	write(t, h, a.SourcePath, "shared")
	req := request(a, b)
	got := build(t, h, req)
	assertKinds(t, got, plan.Materialize)
	if len(got.Operations[0].Desired.Owners) != 2 {
		t.Fatal("missing shared owner")
	}
	if len(a.Owners) != 1 || len(b.Owners) != 1 {
		t.Fatal("mutated owners")
	}
	applyTestPlan(t, h, &req, got, state.Copy)
	req.Adapters = request(a).Adapters
	refreshed := build(t, h, req)
	assertKinds(t, refreshed, plan.Record)
	applyTestPlan(t, h, &req, refreshed, state.Copy)
	assertKinds(t, build(t, h, req))
}

func TestPlanDoesNotMutateContentOrState(t *testing.T) {
	t.Parallel()
	h := fake.New()
	d := target.Target{Path: path("home", "generated"), Kind: target.Generated, Content: []byte("original"), Owners: owner()}
	req := request(d)
	got := build(t, h, req)
	got.Operations[0].Desired.Content[0] = 'X'
	got.Operations[0].Desired.Owners[0].Primitive = "changed"
	if string(d.Content) != "original" || d.Owners[0].Primitive != "skills" {
		t.Fatal("plan aliases inputs")
	}
	if len(req.State.Targets) != 0 {
		t.Fatal("planning changed state")
	}
	if _, err := h.Lstat(d.Path); err == nil {
		t.Fatal("planning wrote destination")
	}
}
