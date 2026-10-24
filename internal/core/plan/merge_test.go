package plan_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/target"
)

// jsonMerger is a tiny test adapter policy, not a production settings adapter.
type jsonMerger struct{}

func (jsonMerger) Merge(current, fragment []byte) ([]byte, error) {
	existing := map[string]json.RawMessage{}
	desired := map[string]json.RawMessage{}
	if len(current) > 0 {
		if err := json.Unmarshal(current, &existing); err != nil {
			return nil, err
		}
	}
	if err := json.Unmarshal(fragment, &desired); err != nil {
		return nil, err
	}
	if existing == nil || desired == nil {
		return nil, errors.New("expected objects")
	}
	for key, value := range desired {
		existing[key] = value
	}
	return json.Marshal(existing)
}

func TestMergePreservesLocalKeysAndBacksUpOnce(t *testing.T) {
	t.Parallel()
	h := fake.New()
	d := target.Target{Path: path("home", "settings.json"), Kind: target.Merged, Merger: jsonMerger{}, Content: []byte(`{"managed":true}`), Backup: target.Once, Owners: owner()}
	write(t, h, d.Path, `{"local":"keep"}`)
	req := request(d)
	got := build(t, h, req)
	assertKinds(t, got, plan.Backup, plan.Merge)
	if string(got.Operations[1].Desired.Content) != `{"local":"keep","managed":true}` {
		t.Fatal("local keys lost")
	}
	applyTestPlan(t, h, &req, got, state.Copy)
	assertKinds(t, build(t, h, req))
	// Local additions already accepted by the merger need only a state refresh.
	write(t, h, d.Path, `{"extra":1,"local":"keep","managed":true}`)
	refresh := build(t, h, req)
	assertKinds(t, refresh, plan.Record)
	assertClass(t, refresh, plan.InSync)
	applyTestPlan(t, h, &req, refresh, state.Copy)
	assertKinds(t, build(t, h, req))
}

func TestMergeRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	h := fake.New()
	d := target.Target{Path: path("home", "settings"), Kind: target.Merged, Merger: jsonMerger{}, Content: []byte(`{"managed":true}`), Backup: target.Once, Owners: owner()}
	write(t, h, d.Path, "malformed")
	got := build(t, h, request(d))
	assertKinds(t, got, plan.Skip)
	assertClass(t, got, plan.Conflict)
}

func TestGeneratedAndSourceMerge(t *testing.T) {
	t.Parallel()
	h := fake.New()
	generated := target.Target{Path: path("home", "instructions"), Kind: target.Generated, Content: []byte("generated"), Owners: owner()}
	merged := target.Target{Path: path("home", "settings"), SourcePath: path("repo", "fragment"), Kind: target.Merged, Merger: jsonMerger{}, Owners: owner()}
	write(t, h, merged.SourcePath, `{"managed":true}`)
	req := request(generated, merged)
	got := build(t, h, req)
	assertKinds(t, got, plan.Write, plan.Merge)
	applyTestPlan(t, h, &req, got, state.Link)
	assertKinds(t, build(t, h, req))
	write(t, h, generated.Path, "drift")
	got = build(t, h, req)
	assertKinds(t, got, plan.Skip)
	if got.Operations[0].Classification != plan.LocalDrift {
		t.Fatal(got.Text())
	}
}

func TestSourceMergeThreeWayProtectsEdits(t *testing.T) {
	t.Parallel()
	for _, sourceChanged := range []bool{false, true} {
		t.Run(stringFlag(sourceChanged), func(t *testing.T) {
			t.Parallel()
			h := fake.New()
			d := target.Target{Path: path("home", "settings"), SourcePath: path("repo", "fragment"), Kind: target.Merged, Merger: jsonMerger{}, Owners: owner()}
			write(t, h, d.SourcePath, `{"managed":true}`)
			req := request(d)
			applyTestPlan(t, h, &req, build(t, h, req), state.Copy)
			write(t, h, d.Path, `{"managed":false}`)
			if sourceChanged {
				write(t, h, d.SourcePath, `{"managed":"changed"}`)
			}
			got := build(t, h, req)
			assertKinds(t, got, plan.Skip)
			want := plan.LocalDrift
			if sourceChanged {
				want = plan.Conflict
			}
			assertClass(t, got, want)
		})
	}
}

func TestTransitionFromOwnedLinkToMergePreservesKeys(t *testing.T) {
	t.Parallel()
	h := fake.New()
	d := declaration("settings")
	write(t, h, d.SourcePath, `{"local":"keep"}`)
	req := request(d)
	applyTestPlan(t, h, &req, build(t, h, req), state.Link)
	d.Kind = target.Merged
	d.SourcePath = ""
	d.Content = []byte(`{"managed":true}`)
	d.Merger = jsonMerger{}
	req.Adapters = request(d).Adapters
	got := build(t, h, req)
	assertKinds(t, got, plan.Merge)
	if string(got.Operations[0].Desired.Content) != `{"local":"keep","managed":true}` {
		t.Fatal("merge did not read owned link contents")
	}
	applyTestPlan(t, h, &req, got, state.Copy)
	assertKinds(t, build(t, h, req))
}
