package tui

import (
	"bytes"
	"flag"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/muesli/termenv"

	"github.com/TomaszJanusz/kitbag/internal/ui/model"
)

// updateFlag rewrites the golden screens: go test ./internal/ui/tui -args -update-screens.
const updateFlag = "update-screens"

func TestMain(m *testing.M) {
	flag.Bool(updateFlag, false, "rewrite golden screens in testdata")
	flag.Parse()
	os.Exit(m.Run())
}

const (
	termWidth  = 80
	termHeight = 24
	waitLimit  = 5 * time.Second
)

func clock() time.Time { return time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC) }

type session struct {
	t      *testing.T
	tm     *teatest.TestModel
	m      *Model
	client *fakeClient
}

func start(t *testing.T, client *fakeClient, options ...func(*Options)) *session {
	t.Helper()
	opts := Options{Runner: model.Runner{Client: client, Now: clock}, Profile: termenv.Ascii}
	for _, option := range options {
		option(&opts)
	}
	m := New(opts)
	s := &session{t: t, m: m, client: client, tm: startModel(t, m)}
	t.Cleanup(func() {
		if err := s.tm.Quit(); err != nil {
			t.Error(err)
		}
		s.tm.WaitFinished(t, teatest.WithFinalTimeout(waitLimit))
		m.Close()
	})
	return s
}

func startModel(t *testing.T, m *Model) *teatest.TestModel {
	t.Helper()
	return teatest.NewTestModel(t, m, teatest.WithInitialTermSize(termWidth, termHeight))
}

// ansiPattern matches CSI and OSC escape sequences.
var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07]*\x07|\x1b[=>]`)

func plainText(b []byte) string { return ansiPattern.ReplaceAllString(string(b), "") }

// waitFor waits until the output stream has shown every fragment.
func (s *session) waitFor(fragments ...string) {
	s.t.Helper()
	teatest.WaitFor(s.t, s.tm.Output(), func(b []byte) bool {
		screen := plainText(b)
		for _, fragment := range fragments {
			if !strings.Contains(screen, fragment) {
				return false
			}
		}
		return true
	}, teatest.WithDuration(waitLimit), teatest.WithCheckInterval(5*time.Millisecond))
}

func (s *session) press(keys ...string) {
	for _, key := range keys {
		s.tm.Send(keyMsg(key))
	}
}

func keyMsg(key string) tea.KeyMsg {
	named := map[string]tea.KeyType{
		keyEnter: tea.KeyEnter, keyEscape: tea.KeyEsc, keyUp: tea.KeyUp, keyDown: tea.KeyDown, keyLeft: tea.KeyLeft,
		keyRight: tea.KeyRight, keyNext: tea.KeyTab, keyPrevious: tea.KeyShiftTab, keyCtrlC: tea.KeyCtrlC,
		keyPageUp: tea.KeyPgUp, keyPageDown: tea.KeyPgDown, "backspace": tea.KeyBackspace, "ctrl+a": tea.KeyCtrlA, "ctrl+k": tea.KeyCtrlK,
	}
	if kind, ok := named[key]; ok {
		return tea.KeyMsg{Type: kind}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
}

// final quits (Ctrl-C also works while editing) and returns the last screen
// as it was displayed.
func (s *session) final() string {
	s.t.Helper()
	s.press(keyCtrlC)
	final, ok := s.tm.FinalModel(s.t, teatest.WithFinalTimeout(waitLimit)).(*Model)
	if !ok {
		s.t.Fatal("unexpected final model")
	}
	final.quitting = false
	return final.View()
}

// golden compares an ANSI-stripped screen with testdata/<name>.golden.
func golden(t *testing.T, name, screen string) {
	t.Helper()
	lines := strings.Split(plainText([]byte(screen)), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
	}
	got := []byte(strings.Join(lines, "\n") + "\n")
	path := filepath.Join("testdata", name+".golden")
	if flag.Lookup(updateFlag).Value.String() == "true" {
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("screen %s differs; rerun with -args -%s after review\n--- got ---\n%s--- want ---\n%s", name, updateFlag, got, want)
	}
}

// idleInput is a keyboard that never types; it is closed after the test.
func idleInput(t *testing.T) io.Reader {
	t.Helper()
	reader, writer := io.Pipe()
	t.Cleanup(func() {
		if err := writer.Close(); err != nil {
			t.Error(err)
		}
	})
	return reader
}
