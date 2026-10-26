package tui

import (
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/TomaszJanusz/kitbag/internal/ui/model"
)

func TestPaneBreakpoint(t *testing.T) {
	t.Parallel()
	for _, width := range []int{40, 60, 99, 100, 120} {
		panes := MasterDetail(width, 30, 0, 8)
		if panes.Stacked != (width < 100) {
			t.Fatalf("width=%d: %+v", width, panes)
		}
		if panes.Stacked {
			if panes.Detail.Y != 9 || panes.Master.Width != width {
				t.Fatal(panes)
			}
		} else if panes.Detail.X <= panes.Master.Width || panes.Detail.Width < MinWidth {
			t.Fatal(panes)
		}
	}
	fixed := MasterDetail(120, 30, 50, 8)
	if fixed.Master.Width != 50 {
		t.Fatal(fixed)
	}
}

func TestSettingsResponsiveScreens(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name          string
		width, height int
	}{
		{"settings-wide", 120, 35},
		{"settings-medium", 100, 30},
		{"settings-stacked", 60, 20},
		{"settings-minimum", 40, 10},
		{"settings-too-small", 39, 9},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := start(t, newFake())
			s.waitFor("2 targets changed locally")
			s.tm.Send(tea.WindowSizeMsg{Width: tc.width, Height: tc.height})
			s.settingRow(5)
			s.press(keyEnter, keyRight)
			if tc.width >= MinWidth {
				s.waitFor("[5m]")
			} else {
				s.waitFor("Enlarge the terminal")
			}
			screen := s.final()
			assertScreenFits(t, screen, tc.width, tc.height)
			golden(t, tc.name, screen)
			s.noWrite()
		})
	}
}

func assertScreenFits(t *testing.T, screen string, width, height int) {
	t.Helper()
	lines := strings.Split(screen, "\n")
	if len(lines) > height {
		t.Fatalf("%d lines > %d", len(lines), height)
	}
	for _, line := range lines {
		if ansi.StringWidth(line) > width {
			t.Fatalf("line exceeds %d: %q", width, line)
		}
	}
}

func TestTwoPaneSelectionInlineStepperAndSideHint(t *testing.T) {
	t.Parallel()
	s := start(t, newFake())
	s.waitFor("2 targets changed locally")
	s.tm.Send(tea.WindowSizeMsg{Width: 120, Height: 35})
	s.settingRow(5)
	s.press(keyEnter, keyRight)
	s.waitFor("[5m]", "[Help]")
	screen := s.final()
	lines := strings.Split(screen, "\n")
	row := -1
	for i, line := range lines {
		if strings.Contains(line, "> sync.poll_interval") {
			row = i
		}
	}
	if row < 0 || !strings.Contains(lines[row], "[5m]") {
		t.Fatalf("stepper is not in its row:\n%s", screen)
	}
	assertSideHint(t, screen)
	s.noWrite()
}

func TestClickDetailDoesNotSelectListRow(t *testing.T) {
	t.Parallel()
	s := start(t, newFake())
	s.waitFor("2 targets changed locally")
	s.tm.Send(tea.WindowSizeMsg{Width: 120, Height: 35})
	s.press("3")
	s.waitFor("[Help]")
	s.click(90, 6)
	screen := s.final()
	if !strings.Contains(screen, "> host") || strings.Contains(screen, "> repo.url") {
		t.Fatal(screen)
	}
}

func TestStepperMouseUsesWideAndNarrowGeometry(t *testing.T) {
	t.Parallel()
	for _, width := range []int{40, 60, 100, 120} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			t.Parallel()
			m := loadedSettingsModel(t, width, 30)
			m.setting = 5
			m.startEdit()
			layout := m.settingsScreen(30 - chromeLines)
			hit := layout.hits[2]
			m.mouse(tea.MouseMsg{X: hit.start, Y: hit.y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
			if m.field().Draft != "5m" {
				t.Fatalf("width=%d draft=%s", width, m.field().Draft)
			}
		})
	}
}

func loadedSettingsModel(t *testing.T, width, height int) *Model {
	t.Helper()
	m := New(Options{NoColour: true})
	t.Cleanup(m.Close)
	m.app.Settings.Init()
	m.app.Settings.Update(model.SettingsLoaded{Seq: 1, Values: newFake().settings})
	m.width, m.height, m.tab = width, height, settingsTab
	return m
}

func assertSideHint(t *testing.T, screen string) {
	t.Helper()
	lines := strings.Split(screen, "\n")
	for _, line := range lines {
		if strings.Contains(line, "[Help]") && strings.Index(line, "[Help]") < 70 {
			t.Fatalf("help is not in the side pane: %q", line)
		}
	}
	if !strings.Contains(screen, "Enter saves") || !strings.Contains(screen, "Source: default") || !strings.Contains(screen, "Default: 60s") {
		t.Fatal(screen)
	}
}

func TestSettingsTextCursorStaysVisibleAfterResize(t *testing.T) {
	t.Parallel()
	m := loadedSettingsModel(t, 120, 35)
	m.setting = 2
	m.startEdit()
	m.editKey(keyMsg("~/" + strings.Repeat("x", 100) + "TAIL"))
	for _, width := range []int{120, 40, 60, 100} {
		m.width = width
		screen := m.View()
		if !strings.Contains(screen, "TAIL") {
			t.Fatalf("width=%d cursor end is hidden:\n%s", width, screen)
		}
	}
}
