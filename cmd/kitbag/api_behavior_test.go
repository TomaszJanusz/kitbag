//go:build darwin || linux

package main

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/api"
	"github.com/TomaszJanusz/kitbag/internal/api/client"
	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/workflow"
)

func callAPI(t *testing.T, c client.Client, method string, params api.Params, result any) {
	t.Helper()
	if err := c.Call(t.Context(), method, params, result, nil); err != nil {
		t.Fatal(err)
	}
}

func TestAPIReadConsistency(t *testing.T) {
	t.Parallel()
	fixture := newAPIFixture(t)
	c := client.Client{Host: fixture.host}
	var status workflow.Status
	callAPI(t, c, "status.get", api.Params{}, &status)
	var inspected api.PlanResult
	callAPI(t, c, "plan.get", api.Params{}, &inspected)
	var drift []plan.Observation
	callAPI(t, c, "drift.list", api.Params{}, &drift)
	if status.Drift[plan.UpdateNeeded] != 1 || status.LastSync != nil {
		t.Fatalf("status=%+v", status)
	}
	if len(inspected.Plan.Operations) != 1 || len(drift) != 1 {
		t.Fatalf("plan=%+v, drift=%+v", inspected, drift)
	}
	if drift[0].Path != inspected.Plan.Operations[0].Path || drift[0].Classification != plan.UpdateNeeded {
		t.Fatalf("inconsistent drift=%+v", drift)
	}
}

func TestAPIConfigSetReloads(t *testing.T) {
	t.Parallel()
	fixture := newAPIFixture(t)
	c := client.Client{Host: fixture.host}
	settings := "schema=1\n[repo]\nurl=\"https://example.invalid/config.git\"\npath=\"~/repo\"\n[git]\nbackend=\"embedded\"\n[sync]\nmode=\"copy\"\n"
	var configured api.ConfigResult
	callAPI(t, c, "config.set", api.Params{Config: settings}, &configured)
	if configured.Config != settings {
		t.Fatalf("setter returned different bytes: %q", configured.Config)
	}
	callAPI(t, c, "config.get", api.Params{}, &configured)
	if configured.Config != settings {
		t.Fatal("setter did not persist the configuration")
	}
	var synced api.SyncResult
	callAPI(t, c, "sync.run", api.Params{NoPull: true}, &synced)
	if synced.Changed != 1 || synced.ExitCode != 0 {
		t.Fatalf("sync=%+v", synced)
	}
	home, err := fixture.host.HomeDir()
	if err != nil {
		t.Fatal(err)
	}
	entry, err := fixture.host.Lstat(filepath.Join(home, "output"))
	if err != nil || !entry.Mode().IsRegular() {
		t.Fatalf("sync did not reload copy mode: %v", err)
	}
	assertAPIAppliedStatus(t, c)
}

func assertAPIAppliedStatus(t *testing.T, c client.Client) {
	t.Helper()
	var status workflow.Status
	callAPI(t, c, "status.get", api.Params{}, &status)
	if status.LastSync == nil || status.LastSync.Changed != 1 || status.LastSync.Result != 0 || status.Drift[plan.InSync] != 1 {
		t.Fatalf("applied status=%+v", status)
	}
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	validateAPIValue(t, "status", data, true)
}
