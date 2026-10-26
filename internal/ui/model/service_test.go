package model

import (
	"errors"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
)

func TestServiceRefreshRequiresConfirmation(t *testing.T) {
	t.Parallel()
	c := &fakeClient{status: StatusSnapshot{Daemon: true, DaemonVersion: "0.1.1", BinaryVersion: "0.1.3", ServiceInstalled: true}}
	m := readyStatus(t, c)
	view := m.View()
	checkText(t, statusTexts(view)...)
	if len(view.Warnings) != 1 || view.Warnings[0].Key != text.StatusVersionMismatch {
		t.Fatal(view.Warnings)
	}
	checkRefreshCancellation(t, m)
	m.Update(RequestServiceRefresh{})
	cmds := m.Update(ConfirmServiceRefresh{Accept: true})
	if _, ok := only(t, cmds).(RunServiceRefresh); !ok {
		t.Fatal(cmds)
	}
	if got := m.Update(RequestServiceRefresh{}); len(got) != 0 || m.View().ConfirmRefresh {
		t.Fatal("duplicate refresh")
	}
	settle(t, m, c, cmds)
	if c.refreshes != 1 || !m.refreshed {
		t.Fatal(c.refreshes, m.View())
	}
}

func TestServiceRefreshNegativeControls(t *testing.T) {
	t.Parallel()
	for _, snapshot := range []StatusSnapshot{
		{Daemon: true, DaemonVersion: "same", BinaryVersion: "same", ServiceInstalled: true},
		{Daemon: true, DaemonVersion: "old", BinaryVersion: "new"},
		{ServiceInstalled: true},
	} {
		m := readyStatus(t, &fakeClient{status: snapshot})
		m.Update(RequestServiceRefresh{})
		if m.View().ConfirmRefresh || len(m.Update(ConfirmServiceRefresh{Accept: true})) != 0 {
			t.Fatal("unneeded refresh")
		}
	}
}

func TestServiceRefreshStaleAndError(t *testing.T) {
	t.Parallel()
	c := &fakeClient{status: StatusSnapshot{ServiceInstalled: true, ServiceStale: true}, refreshErr: errors.New("reload failed")}
	m := readyStatus(t, c)
	view := m.View()
	if len(view.Warnings) != 1 || view.Warnings[0].Key != text.StatusServiceStale {
		t.Fatal(view)
	}
	m.Update(RequestServiceRefresh{})
	settle(t, m, c, m.Update(ConfirmServiceRefresh{Accept: true}))
	if m.refreshProblem == nil || m.refreshed || c.refreshes != 1 {
		t.Fatal(m.View())
	}
}

func checkRefreshCancellation(t *testing.T, m *Status) {
	t.Helper()
	if got := m.Update(ConfirmServiceRefresh{Accept: true}); len(got) != 0 {
		t.Fatal("confirmed without prompt", got)
	}
	m.Update(RequestServiceRefresh{})
	if !m.View().ConfirmRefresh {
		t.Fatal("missing confirmation")
	}
	if got := m.Update(ConfirmServiceRefresh{}); len(got) != 0 || m.View().ConfirmRefresh {
		t.Fatal("cancel ran refresh")
	}
}
