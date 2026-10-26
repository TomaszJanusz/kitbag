package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/TomaszJanusz/kitbag/internal/ui/model"
)

func TestHomeKeyboardOpensScreens(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		title, key string
		down       int
	}{{"Agents", "a", 1}, {"Drift", "d", 2}, {"Settings", "c", 3}, {"Activity", "l", 4}} {
		t.Run(tc.title, func(t *testing.T) {
			t.Parallel()
			s := startHome(t, newFake())
			s.waitFor("2 targets changed locally", "> Sync now")
			for range tc.down {
				s.press(keyDown)
			}
			s.press(keyEnter)
			s.waitFor("kitbag › " + tc.title)
			s.press(keyEscape)
			s.waitFor("? help · q quit")
			s.press(tc.key)
			s.waitFor("kitbag › " + tc.title)
			s.press(keyQuit)
			s.waitFor("? help · q quit")
			s.final()
			if !s.m.home {
				t.Fatal("Esc/q did not return home")
			}
		})
	}
}

func TestHomeStatusAndSync(t *testing.T) {
	t.Parallel()
	s := startHome(t, newFake())
	s.waitFor("2 targets changed locally")
	s.press(keyVimUp, keyEnter)
	s.waitFor("kitbag › Status", "Repository", "Last check")
	s.press(keyEscape, keyVimDown, keyVimUp, keyVimDown, keyEnter)
	s.client.phases <- "pull"
	s.waitFor("Syncing: pulling the repository")
	close(s.client.phases)
	s.waitFor("2 targets changed locally")
	if screen := s.final(); !s.m.home || strings.Contains(screen, "Repository") || strings.Contains(screen, "Connection") {
		t.Fatal(screen)
	}
}

func TestHomeMouseOpensEachScreen(t *testing.T) {
	t.Parallel()
	projection := homeProjection(t, termWidth, termHeight)
	layout := projection.homeLayout()
	for i, title := range []string{"Sync now", "Agents", "Drift", "Settings", "Activity"} {
		t.Run(title, func(t *testing.T) {
			t.Parallel()
			s := startHome(t, newFake())
			s.waitFor("2 targets changed locally")
			hit := layout.menu[i]
			s.click(hit.X+2, hit.Y)
			if i == 0 {
				s.client.phases <- "apply"
				s.waitFor("Syncing: applying changes")
				close(s.client.phases)
			} else {
				s.waitFor("kitbag › " + title)
				s.press(keyEscape)
				s.waitFor("? help · q quit")
			}
			s.final()
			if !s.m.home {
				t.Fatal("mouse destination did not return home")
			}
		})
	}
}

func homeProjection(t *testing.T, width, height int) *Model {
	t.Helper()
	m := New(Options{NoColour: true, Version: "0.1.2", Clock: screenClock(t)})
	t.Cleanup(m.Close)
	m.width, m.height = width, height
	m.app.Init()
	m.app.Update(model.StatusLoaded{Seq: 1, Snapshot: newFake().status})
	return m
}

func TestHomeGoldenScreens(t *testing.T) {
	t.Parallel()
	for _, size := range [][2]int{{120, 35}, {80, 24}, {60, 20}, {40, 10}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			t.Parallel()
			m := homeProjection(t, size[0], size[1])
			m.app.Update(model.Refresh{})
			m.app.Update(model.StatusLoaded{Seq: 2, Snapshot: model.StatusSnapshot{Repository: "/repo/kitbag", Git: model.GitState{Branch: "main"}, Daemon: true, Drift: model.DriftCounts{InSync: 4}, LastCheck: &model.LastCheck{At: clock().Add(-2 * time.Minute), NextAt: clock().Add(time.Minute), Outcome: "ok", Interval: time.Minute}}})
			m.app.Update(model.AgentsLoaded{Seq: 2, Snapshot: model.AgentsSnapshot{Instances: make([]model.AgentInstance, 4)}})
			for _, profile := range []Profile{termenv.Ascii, termenv.TrueColor} {
				m.styles = newStyles(profile)
				screen := m.View()
				assertScreenFits(t, screen, size[0], size[1])
				profileName := "color"
				if profile == termenv.Ascii {
					profileName = "no-color"
				}
				name := fmt.Sprintf("v2-home-%dx%d-%s", size[0], size[1], profileName)
				golden(t, name, screen)
			}
		})
	}
}

func TestHomeLogoNarrowAndNoColour(t *testing.T) {
	t.Parallel()
	for _, size := range [][2]int{{120, 35}, {80, 24}, {60, 20}, {40, 10}} {
		m := homeProjection(t, size[0], size[1])
		for _, profile := range []Profile{termenv.Ascii, termenv.TrueColor} {
			m.styles = newStyles(profile)
			lines := m.logo()
			for _, line := range lines {
				if ansi.StringWidth(line) > size[0] {
					t.Fatal("logo overflow", line)
				}
			}
			assertPlainLogo(t, m, lines, size[0], size[1])
			if profile == termenv.Ascii && ansiPattern.MatchString(m.View()) {
				t.Fatal("NO_COLOR emitted ANSI")
			}
		}
	}
}

func TestHomeMouseStraysAndStatusClick(t *testing.T) {
	t.Parallel()
	m := homeProjection(t, 80, 24)
	layout := m.homeLayout()
	for _, msg := range []tea.MouseMsg{
		{X: 0, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress},
		{X: layout.menu[1].X, Y: layout.menu[1].Y, Button: tea.MouseButtonRight, Action: tea.MouseActionPress},
		{X: layout.menu[1].X, Y: layout.menu[1].Y, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease},
	} {
		m.mouse(msg)
	}
	if !m.home {
		t.Fatal("stray click opened a screen")
	}
	m.mouse(tea.MouseMsg{X: 40, Y: layout.status.Y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.home || m.tab != statusTab {
		t.Fatal("status click did not open Status")
	}
}

func assertPlainLogo(t *testing.T, m *Model, lines []string, width, height int) {
	t.Helper()
	plain := m.styles.mono || width < pixelMinWidth || height < pixelMinHeight
	if plain && (len(lines) != 1 || !strings.Contains(lines[0], "kitbag")) {
		t.Fatal("plain logo missing", lines)
	}
}
