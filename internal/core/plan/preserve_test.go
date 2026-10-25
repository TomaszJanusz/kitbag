package plan_test

import (
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
)

func TestPreserveLeavesOwnershipAndContentsAlone(t *testing.T) {
	t.Parallel()
	for _, owned := range []bool{false, true} {
		h := fake.New()
		d := declaration("settings")
		d.Preserve = true
		write(t, h, d.Path, "local settings")
		req := request(d)
		if owned {
			req.State.Targets = []state.Target{tracked(d, plan.HashBytes([]byte("local settings")), state.Merged)}
		}
		// No source exists; preserving must bypass source reads and stale removal.
		assertKinds(t, build(t, h, req))
	}
}
