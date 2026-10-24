package plan_test

import (
	"reflect"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
)

func TestThreeWayDecisionTable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		source    string
		current   string
		want      plan.Classification
		operation plan.Kind
	}{
		{"unchanged", "old", "old", plan.InSync, ""},
		{"source changed", "new", "old", plan.UpdateNeeded, plan.Materialize},
		{"destination changed", "old", "local", plan.LocalDrift, plan.Skip},
		{"both changed", "new", "local", plan.Conflict, plan.Skip},
		{"converged", "new", "new", plan.UpdateNeeded, plan.Materialize},
		{"missing unchanged source", "old", "", plan.UpdateNeeded, plan.Materialize},
		{"missing changed source", "new", "", plan.UpdateNeeded, plan.Materialize},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := fake.New()
			d := declaration("skill")
			write(t, h, d.SourcePath, tc.source)
			if tc.current != "" {
				write(t, h, d.Path, tc.current)
			}
			req := request(d)
			req.State.Targets = []state.Target{tracked(d, plan.HashBytes([]byte("old")), state.Copy)}
			got := build(t, h, req)
			assertClass(t, got, tc.want)
			if tc.operation == "" {
				assertKinds(t, got)
			} else {
				assertKinds(t, got, tc.operation)
			}
		})
	}
}

func TestLinkAndCopyHaveIdenticalLogicalPlans(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"initial", "unchanged", "update", "retarget source", "missing source", "stale"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			linked := modeScenario(t, state.Link, scenario)
			copied := modeScenario(t, state.Copy, scenario)
			if !reflect.DeepEqual(linked, copied) {
				t.Fatalf("link:\n%s\ncopy:\n%s", linked.Text(), copied.Text())
			}
			linkJSON, err := linked.JSON()
			if err != nil {
				t.Fatal(err)
			}
			copyJSON, err := copied.JSON()
			if err != nil || string(linkJSON) != string(copyJSON) {
				t.Fatalf("different JSON: %s\n%s; %v", linkJSON, copyJSON, err)
			}
		})
	}
}

func modeScenario(t *testing.T, kind state.Kind, scenario string) plan.Plan {
	t.Helper()
	h := fake.New()
	d := declaration("skill")
	write(t, h, d.SourcePath, "old")
	req := request(d)
	if scenario != "initial" {
		applyTestPlan(t, h, &req, build(t, h, req), kind)
	}
	switch scenario {
	case "update":
		write(t, h, d.SourcePath, "new")
	case "retarget source":
		d.SourcePath = path("repo", "moved")
		write(t, h, d.SourcePath, "new")
	case "missing source":
		if err := h.Remove(d.SourcePath); err != nil {
			t.Fatal(err)
		}
	case "stale":
		req.Adapters = nil
	}
	if scenario == "retarget source" {
		req.Adapters = request(d).Adapters
	}
	return build(t, h, req)
}

func TestForeignLinksAreNeverFollowedOrReplaced(t *testing.T) {
	t.Parallel()
	for _, kind := range []state.Kind{"", state.Link, state.Copy} {
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()
			h := fake.New()
			d := declaration("skill")
			write(t, h, d.SourcePath, "desired")
			link(t, h, path("external", "missing"), d.Path)
			req := request(d)
			if kind != "" {
				req.State.Targets = []state.Target{tracked(d, plan.HashBytes([]byte("old")), kind)}
			}
			got := build(t, h, req)
			assertKinds(t, got, plan.Skip)
			want := plan.Conflict
			if kind == "" {
				want = plan.UnmanagedObstacle
			}
			assertClass(t, got, want)
		})
	}
}
