package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/core/workflow"
	"github.com/TomaszJanusz/kitbag/internal/timefmt"
)

func TestStatusCheckTextAndJSON(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 26, 16, 29, 12, 0, time.UTC)
	for _, running := range []bool{true, false} {
		var out, warnings bytes.Buffer
		c := commandLine{stdout: &out, stderr: &warnings, clock: timefmt.Clock{Location: time.FixedZone("CET", 3600), Now: func() time.Time { return at.Add(time.Minute) }}}
		status := workflow.Status{StatusSchema: 1, DaemonRunning: running, LastCheck: &state.CheckRecord{SchemaVersion: 1, Repository: "/repo", At: at, NextAt: at.Add(time.Minute), Outcome: "ok", IntervalSeconds: 60}}
		if err := c.printStatus(status, false); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "Last check: 2026-10-26 17:29:12 CET (1 min ago), ok") || strings.Contains(out.String(), "Next check:") != running || strings.Contains(out.String(), "stale; daemon not running") == running {
			t.Fatal(out.String())
		}
		status.LastCheck.Outcome, status.LastCheck.Category = "failed", "operation"
		out.Reset()
		if err := c.printStatus(status, false); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(warnings.String(), "failed: The operation failed") || !strings.Contains(warnings.String(), "1 min ago") {
			t.Fatal(warnings.String())
		}
		assertCheckStatusJSON(t, &c, status, &out, running)
	}
}

func assertCheckStatusJSON(t *testing.T, c *commandLine, status workflow.Status, out *bytes.Buffer, running bool) {
	t.Helper()
	out.Reset()
	if err := c.printStatus(status, true); err != nil {
		t.Fatal(err)
	}
	var got workflow.Status
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.LastCheck.Outcome != "failed" || got.DaemonRunning != running || !strings.Contains(out.String(), `"at": "2026-10-26T16:29:12Z"`) {
		t.Fatal(out.String())
	}
}
