package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestServiceMismatchScreenAndConfirmation(t *testing.T) {
	t.Parallel()
	c := newFake()
	c.status.ServiceInstalled, c.status.ServiceStale = true, true
	c.status.DaemonVersion, c.status.BinaryVersion = "0.1.1", "0.1.3"
	s := start(t, c)
	s.waitFor("daemon 0.1.1, binary 0.1.3", "version mismatch", "[h] Repair service")
	s.tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	s.waitFor("y confirm")
	if c.refreshes.Load() != 0 {
		t.Fatal("refresh before confirmation")
	}
	golden(t, "status-service-refresh", s.final())
}

func TestServiceRefreshKeyConfirmsAndCancels(t *testing.T) {
	t.Parallel()
	c := newFake()
	c.status.ServiceInstalled, c.status.ServiceStale = true, true
	s := start(t, c)
	s.waitFor("[h] Repair service")
	s.tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	s.waitFor("y confirm")
	s.tm.Send(tea.KeyMsg{Type: tea.KeyEsc})
	s.tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if c.refreshes.Load() != 0 {
		t.Fatal("cancel refreshed service")
	}
	s.tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	s.tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	s.waitFor("Service refreshed.")
	if c.refreshes.Load() != 1 {
		t.Fatal(c.refreshes.Load())
	}
	s.final()
}
