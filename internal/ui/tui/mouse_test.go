package tui

import (
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/TomaszJanusz/kitbag/internal/ui/model"
)

// Screen lines at 80×24: 0 tab bar, 1 blank, then the body. The Drift tab has
// its heading on line 2, a blank line 3 and the two targets on lines 4 and 5.
const (
	driftFirstRow  = 4
	driftSecondRow = 5
	driftTabX      = 13 // inside " 2 Drift "
	settingsTabX   = 24 // inside " 3 Settings "
	logTabX        = 38 // inside " 4 Activity "
	separatorX     = 10 // the blank between " 1 Status " and "[2 Drift]" (end of the first tab)
)

func (s *session) mouse(button tea.MouseButton, action tea.MouseAction, x, y int) {
	s.tm.Send(tea.MouseMsg{X: x, Y: y, Button: button, Action: action})
}

func (s *session) click(x, y int) { s.mouse(tea.MouseButtonLeft, tea.MouseActionPress, x, y) }

func (s *session) wheel(button tea.MouseButton) { s.mouse(button, tea.MouseActionPress, 0, 3) }

func TestClickingATabSwitchesToIt(t *testing.T) {
	t.Parallel()
	s := start(t, newFake())
	s.waitFor("[1 Status]")
	s.click(driftTabX, 0)
	s.waitFor("[2 Drift]", "2 targets differ")
	s.click(settingsTabX, 0)
	s.waitFor("[3 Settings]", "Precedence:")
	s.click(logTabX, 0)
	s.waitFor("[4 Activity]", "Connected to the daemon")
	s.click(1, 0)
	s.waitFor("[1 Status]", "Daemon, live updates")
}

func TestClickingATabIsLimitedToTheTabBar(t *testing.T) {
	t.Parallel()
	s := start(t, newFake())
	s.press("2")
	s.waitFor("[2 Drift]")
	s.click(separatorX, 0)                                                         // the blank between "[1 Status]" and "[2 Drift]"
	s.click(settingsTabX, 1)                                                       // the blank line under the bar
	s.mouse(tea.MouseButtonRight, tea.MouseActionPress, settingsTabX, 0)           // not the primary button
	s.mouse(tea.MouseButtonLeft, tea.MouseActionRelease, settingsTabX, 0)          // a release is not a click
	s.mouse(tea.MouseButtonLeft, tea.MouseActionMotion, settingsTabX, 0)           // a drag is not a click
	s.mouse(tea.MouseButtonLeft, tea.MouseActionPress, termWidth+settingsTabX, 0)  // outside the screen
	s.mouse(tea.MouseButtonLeft, tea.MouseActionPress, settingsTabX, termHeight+3) // outside the screen
	screen := s.final()
	if !strings.Contains(screen, "[2 Drift]") || strings.Contains(screen, "[1 Status]") || strings.Contains(screen, "[3 Settings]") {
		t.Fatalf("a stray mouse event switched tabs:\n%s", screen)
	}
}

func TestClickingNarrowTabBarUsesTheShortLabels(t *testing.T) {
	t.Parallel()
	s := start(t, newFake())
	s.tm.Send(tea.WindowSizeMsg{Width: MinWidth, Height: termHeight})
	s.waitFor("[1 Status]")
	// "[1 Status] 2 3 4": the digit 3 sits at column 13.
	s.click(13, 0)
	s.waitFor("[3 Settings]")
}

func TestClickingADriftRowSelectsThenOpensIt(t *testing.T) {
	t.Parallel()
	s := start(t, newFake())
	s.press("2")
	s.waitFor("> /home/user/.claude/CLAUDE.md")
	s.click(5, driftSecondRow)
	s.waitFor("> /home/user/.codex/AGENTS.md")
	s.click(5, driftSecondRow)
	s.waitFor("Planned actions", "Backup: /home/user/.local/state/kitbag/backup/AGENTS.md")
	// A click in the open detail changes nothing; Esc still closes it.
	s.click(5, driftFirstRow)
	s.press(keyEscape)
	s.waitFor("Enter: details")
	s.click(5, driftFirstRow)
	s.waitFor("> /home/user/.claude/CLAUDE.md")
	if screen := s.final(); strings.Contains(screen, "Planned actions") {
		t.Fatalf("selecting another row opened the detail:\n%s", screen)
	}
}

func TestClickingOutsideTheDriftRowsKeepsTheSelection(t *testing.T) {
	t.Parallel()
	s := start(t, newFake())
	s.press("2")
	s.waitFor("> /home/user/.claude/CLAUDE.md")
	s.click(5, 2)                // heading
	s.click(5, driftSecondRow+1) // below the last row
	s.click(5, termHeight-1)     // footer
	s.mouse(tea.MouseButtonRight, tea.MouseActionPress, 5, driftSecondRow)
	s.mouse(tea.MouseButtonLeft, tea.MouseActionRelease, 5, driftSecondRow)
	screen := s.final()
	if !strings.Contains(screen, "> /home/user/.claude/CLAUDE.md") || strings.Contains(screen, "Planned actions") {
		t.Fatalf("a stray click moved the selection or opened a detail:\n%s", screen)
	}
}

func TestClickingASettingSelectsItWithoutEditing(t *testing.T) {
	t.Parallel()
	s := start(t, newFake())
	s.press("3")
	s.waitFor("Precedence:", "host  laptop · config.toml")
	// Settings rows follow the heading and a blank line, like the drift list.
	s.click(5, 6)
	s.click(5, 6)
	s.waitFor("> repo.url")
	screen := s.final()
	if strings.Contains(screen, "Editing:") {
		t.Fatalf("a click started editing:\n%s", screen)
	}
	select {
	case got := <-s.client.writes:
		t.Fatalf("a click wrote a setting: %v", got)
	default:
	}
}

func TestWheelScrollsTheDriftList(t *testing.T) {
	t.Parallel()
	s := start(t, newFake())
	s.press("2")
	s.waitFor("> /home/user/.claude/CLAUDE.md")
	s.wheel(tea.MouseButtonWheelDown)
	s.waitFor("> /home/user/.codex/AGENTS.md")
	s.wheel(tea.MouseButtonWheelUp)
	s.waitFor("> /home/user/.claude/CLAUDE.md")
	// The wheel never moves the selection on other tabs or under the detail.
	s.wheel(tea.MouseButtonWheelDown)
	s.waitFor("> /home/user/.codex/AGENTS.md")
	s.press(keyEnter)
	s.waitFor("Planned actions")
	s.wheel(tea.MouseButtonWheelUp)
	s.press(keyEscape)
	s.waitFor("> /home/user/.codex/AGENTS.md")
	s.press("1")
	s.waitFor("Daemon, live updates")
	s.wheel(tea.MouseButtonWheelUp)
	s.press("2")
	if screen := s.final(); !strings.Contains(screen, "> /home/user/.codex/AGENTS.md") {
		t.Fatalf("the wheel on the Status tab moved the drift selection:\n%s", screen)
	}
}

func TestWheelScrollsTheActivityLog(t *testing.T) {
	t.Parallel()
	s := start(t, newFake())
	s.tm.Send(tea.WindowSizeMsg{Width: termWidth, Height: MinHeight})
	s.press("4")
	s.waitFor("Following new entries")
	for range 8 {
		s.client.events <- model.Event{Kind: model.EventDriftDetected}
	}
	s.waitFor("warn The daemon detected drift")
	s.wheel(tea.MouseButtonWheelUp)
	s.waitFor("Scrolling; press f to follow new entries")
	s.wheel(tea.MouseButtonWheelDown)
	s.wheel(tea.MouseButtonWheelDown)
	s.waitFor("Following new entries")
	s.wheel(tea.MouseButtonWheelUp)
	s.waitFor("Scrolling; press f to follow new entries")
	// Other buttons, releases and the help overlay leave the log alone.
	s.press(keyFollow)
	s.waitFor("Following new entries")
	s.mouse(tea.MouseButtonWheelUp, tea.MouseActionRelease, 0, 3)
	s.mouse(tea.MouseButtonNone, tea.MouseActionPress, 0, 3)
	s.press(keyHelp)
	s.waitFor("Keyboard")
	s.wheel(tea.MouseButtonWheelUp)
	s.press(keyHelp)
	if screen := s.final(); !strings.Contains(screen, "Following new entries") {
		t.Fatalf("an ignored mouse event paused the log:\n%s", screen)
	}
}

func TestMouseIsIgnoredWhileEditingAndInHelp(t *testing.T) {
	t.Parallel()
	s := start(t, newFake())
	s.press("3")
	s.waitFor("Precedence:")
	s.press(keyEnter)
	s.waitFor("←/→/Space or click: choose")
	s.click(driftTabX, 0)
	s.click(5, 6)
	s.press(keyEscape, keyHelp)
	s.waitFor("Keyboard")
	s.click(driftTabX, 0)
	s.press(keyHelp)
	if screen := s.final(); !strings.Contains(screen, "[3 Settings]") {
		t.Fatalf("the mouse acted under an overlay:\n%s", screen)
	}
}

func TestHelpAndFooterMentionTheMouse(t *testing.T) {
	t.Parallel()
	s := start(t, newFake())
	s.waitFor("? help", "a advanced", "mouse: click, wheel")
	s.press(keyHelp)
	s.waitFor("Mouse: click a tab or row", "wheel: scroll")
}

// syncBuffer is an output safe to read while the program writes to it.
type syncBuffer struct {
	buf strings.Builder
	mu  sync.Mutex
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// Terminal mode switches: Bubble Tea turns on cell-motion reporting (1002) with
// SGR coordinates (1006) and must turn both off again.
const (
	mouseOn  = "\x1b[?1002h"
	sgrOn    = "\x1b[?1006h"
	mouseOff = "\x1b[?1002l"
	sgrOff   = "\x1b[?1006l"
)

// Without a terminal Bubble Tea reports no size, so only the mode switches can
// be checked here; the click mapping is covered through teatest above.
func TestRunTurnsMouseReportingOnAndOffAgain(t *testing.T) {
	t.Parallel()
	client := newFake()
	reader, writer := io.Pipe()
	var output syncBuffer
	done := make(chan error, 1)
	go func() { done <- Run(t.Context(), Options{Runner: model.Runner{Client: client}}, reader, &output) }()
	for client.subscribed.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	if _, err := writer.Write([]byte("q")); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	assertMouseReleased(t, output.String())
}

func TestRunReleasesTheMouseAfterAPanic(t *testing.T) {
	t.Parallel()
	client := newFake()
	client.panicStatus = true
	var output syncBuffer
	err := Run(t.Context(), Options{Runner: model.Runner{Client: client}}, idleInput(t), &output)
	if !errors.Is(err, tea.ErrProgramPanic) {
		t.Fatalf("err=%v", err)
	}
	assertMouseReleased(t, output.String())
}

func assertMouseReleased(t *testing.T, output string) {
	t.Helper()
	for _, sequence := range []string{mouseOn, sgrOn, mouseOff, sgrOff} {
		if !strings.Contains(output, sequence) {
			t.Fatalf("missing %q in %q", sequence, output)
		}
	}
	if strings.LastIndex(output, mouseOff) < strings.LastIndex(output, mouseOn) {
		t.Fatalf("mouse reporting left on: %q", output)
	}
}
