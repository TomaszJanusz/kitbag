package workflow_test

import (
	"testing"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/core/state"
)

func TestStatusHistoricalCheck(t *testing.T) {
	t.Parallel()
	f := setup(t)
	r := state.CheckRecord{SchemaVersion: 1, Repository: f.s.Repository, At: f.s.Now(), NextAt: f.s.Now().Add(time.Minute), Outcome: "ok", IntervalSeconds: 60}
	if err := f.s.Store.SaveCheck(r); err != nil {
		t.Fatal(err)
	}
	status, err := f.s.Status(t.Context())
	if err != nil || status.LastCheck == nil || status.LastCheck.Outcome != "ok" || status.DaemonRunning || status.LastSync != nil {
		t.Fatal(status, err)
	}
	r.Repository += "-other"
	if err = f.s.Store.SaveCheck(r); err != nil {
		t.Fatal(err)
	}
	status, err = f.s.Status(t.Context())
	if err != nil || status.LastCheck != nil {
		t.Fatal(status, err)
	}
}
