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
)

func (s *session) mouse(button tea.MouseButton, action tea.MouseAction, x, y int) {
	s.tm.Send(tea.MouseMsg{X: x, Y: y, Button: button, Action: action})
}

func (s *session) click(x, y int) { s.mouse(tea.MouseButtonLeft, tea.MouseActionPress, x, y) }

func (s *session) wheel(button tea.MouseButton) { s.mouse(button, tea.MouseActionPress, 0, 3) }

func TestBreadcrumbReturnsHome(t *testing.T) {
	t.Parallel()
	s := start(t, newFake())
	s.waitFor("kitbag › Status")
	s.click(1, 0)
	s.waitFor("? help · q quit", "> Sync now")
	if screen := s.final(); !s.m.home {
		t.Fatal(screen)
	}
}

func TestBreadcrumbStrayClicksKeepScreen(t *testing.T) {
	t.Parallel()
	s := start(t, newFake())
	s.press("2")
	s.waitFor("kitbag › Drift")
	s.click(70, 0)
	s.click(5, 1)
	s.mouse(tea.MouseButtonRight, tea.MouseActionPress, 5, 0)
	s.mouse(tea.MouseButtonLeft, tea.MouseActionRelease, 5, 0)
	s.mouse(tea.MouseButtonLeft, tea.MouseActionMotion, 5, 0)
	s.click(termWidth+1, 0)
	s.click(5, termHeight+1)
	if screen := s.final(); s.m.home || s.m.tab != driftTab {
		t.Fatal(screen)
	}
}

func TestNarrowBreadcrumbReturnsHome(t *testing.T) {
	t.Parallel()
	s := start(t, newFake())
	s.tm.Send(tea.WindowSizeMsg{Width: MinWidth, Height: termHeight})
	s.waitFor("kitbag › Status")
	s.click(1, 0)
	s.waitFor("? help · q quit")
	if screen := s.final(); !s.m.home {
		t.Fatal(screen)
	}
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
	s.click(5, 2)                      // heading
	s.click(5, driftSecondRow+1)       // below the last row
	s.click(termWidth-1, termHeight-1) // unused footer area
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
	s := start(t, newFake(), func(o *Options) { o.LogCapacity = 8 })
	s.tm.Send(tea.WindowSizeMsg{Width: termWidth, Height: MinHeight})
	s.press("4")
	s.waitFor("Following new entries")
	for range 8 {
		s.client.events <- model.Event{Kind: model.EventDriftDetected}
	}
	s.waitFor("1 older entries dropped")
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
	s.click(1, 0)
	s.click(5, 6)
	s.press(keyEscape, keyHelp)
	s.waitFor("Keyboard")
	s.click(1, 0)
	if screen := s.final(); !strings.Contains(screen, "kitbag › Settings") {
		t.Fatalf("the mouse acted under an overlay:\n%s", screen)
	}
}

func TestHelpAndFooterMentionTheMouse(t *testing.T) {
	t.Parallel()
	s := start(t, newFake())
	s.waitFor("esc back", "? help")
	s.press(keyHelp, keyPageDown)
	s.waitFor("Mouse: click home rows", "wheel: scroll")
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
