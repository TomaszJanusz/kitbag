package plan_test

import (
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/target"
)

// legacyRendering stands in for agent-config-sync's output for a clone: its
// marker and import lines name the clone.
func legacyRendering(clone string) string {
	return "<!-- agent-config-sync (" + clone + ") -->\n@" + clone + "/global/AGENTS.md\n"
}

func legacyGenerated() target.Target {
	return target.Target{
		Path: path("home", "CLAUDE.md"), Kind: target.Generated, Backup: target.Once, Owners: owner(),
		Content: []byte("<!-- kitbag -->\n@" + path("repo") + "/global/AGENTS.md\n"),
		Legacy:  []byte(legacyRendering(path("repo"))),
	}
}

func TestLegacyOutputIsReplacedInPlace(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		current string
		roots   []string
		want    []plan.Kind
	}{
		"old clone at a legacy root":   {legacyRendering(path("old")), []string{path("old")}, []plan.Kind{plan.Write}},
		"active clone":                 {legacyRendering(path("repo")), []string{path("old")}, []plan.Kind{plan.Write}},
		"hand-edited legacy output":    {legacyRendering(path("old")) + "local\n", []string{path("old")}, []plan.Kind{plan.Backup, plan.Write}},
		"clone outside legacy roots":   {legacyRendering(path("elsewhere")), []string{path("old")}, []plan.Kind{plan.Backup, plan.Write}},
		"without legacy roots":         {legacyRendering(path("old")), nil, []plan.Kind{plan.Backup, plan.Write}},
		"legacy root prefix collision": {legacyRendering(path("old")), []string{path("ol")}, []plan.Kind{plan.Backup, plan.Write}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := fake.New()
			d := legacyGenerated()
			write(t, h, d.Path, c.current)
			req := request(d)
			req.LegacyRoots = c.roots
			got := build(t, h, req)
			assertKinds(t, got, c.want...)
			assertReplaces(t, got, len(c.want) == 1, plan.HashBytes([]byte(c.current)))
		})
	}
}

// assertReplaces checks the final output operation: an in-place replacement of
// the legacy rendering with hash, or an ordinary obstacle after a backup.
func assertReplaces(t *testing.T, got plan.Plan, inPlace bool, hash string) {
	t.Helper()
	op := got.Operations[len(got.Operations)-1]
	if !inPlace {
		if op.Replaces != nil || op.Classification != plan.UnmanagedObstacle {
			t.Fatalf("foreign file replaced in place: %+v", op)
		}
		return
	}
	r := op.Replaces
	if op.Classification != plan.UpdateNeeded || r == nil || r.WrittenHash != hash || r.Kind != state.Generated || len(r.Owners) != 1 {
		t.Fatalf("operation = %+v; want in-place replacement of %s", op, hash)
	}
	if strings.Contains(got.Text(), "backup") {
		t.Fatalf("plan text = %s", got.Text())
	}
}

// legacyMerger renders merged output as the legacy tool would: another marker.
type legacyMerger struct{ jsonMerger }

func (legacyMerger) Legacy(output []byte) ([]byte, error) {
	return []byte(strings.ReplaceAll(string(output), "kitbag-managed", "agent-config-managed")), nil
}

func TestLegacyMergedOutputIsReplacedInPlace(t *testing.T) {
	t.Parallel()
	h := fake.New()
	d := target.Target{
		Path: path("home", "settings.json"), Kind: target.Merged, Merger: legacyMerger{}, Backup: target.Once, Owners: owner(),
		Content: []byte(`{"hook":"` + path("repo") + `/bin/notify # kitbag-managed"}`),
	}
	write(t, h, d.Path, `{"hook":"`+path("old")+`/bin/notify # agent-config-managed","local":1}`)
	req := request(d)
	req.LegacyRoots = []string{path("old")}
	got := build(t, h, req)
	assertKinds(t, got, plan.Merge)
	if got.Operations[0].Replaces == nil || got.Operations[0].Replaces.Kind != state.Merged {
		t.Fatalf("operation = %+v", got.Operations[0])
	}
	// Bytes the merge does not reproduce (trailing space) keep the one-time backup.
	write(t, h, d.Path, `{"hook":"`+path("old")+`/bin/notify # agent-config-managed","local":1} `)
	assertKinds(t, build(t, h, req), plan.Backup, plan.Merge)
}
