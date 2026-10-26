// Package tui renders the shared view models (internal/ui/model) in a
// terminal with Bubble Tea and Lip Gloss. It owns no behavior, validation or
// text: keys become model intents, views come from the models and every
// string comes from the text catalog. Commands run one at a time on a worker
// goroutine, the event subscription on another; both end when the program
// quits.
package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/termenv"

	"github.com/TomaszJanusz/kitbag/internal/timefmt"
	"github.com/TomaszJanusz/kitbag/internal/ui/model"
	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
)

// Minimum usable terminal size; smaller terminals get a notice instead.
const (
	MinWidth  = 40
	MinHeight = 10
)

// Profile is a terminal colour capability.
type Profile = termenv.Profile

// Options configures a renderer. Profile is the terminal's colour capability;
// NoColour (NO_COLOR or ui.color=never) forces monochrome output.
type Options struct {
	// Clock sets the zone and relative hints of displayed times (zero: UTC, no hints).
	Clock       timefmt.Clock
	Runner      model.Runner
	Catalog     text.Catalog
	Profile     Profile
	LogCapacity int
	NoColour    bool
}

type tab int

const (
	statusTab tab = iota
	driftTab
	settingsTab
	logTab
	tabCount
)

// resultMsg carries a model.Msg or panicMsg emitted by a worker.
type resultMsg struct{ msg any }

// panicMsg moves a worker panic onto the program goroutine, where Bubble Tea
// recovers it and restores the terminal.
type panicMsg struct{ value any }

// Model is the Bubble Tea model. It is not safe for concurrent use; only the
// program loop calls Init, Update and View.
type Model struct {
	styles   styles
	runner   model.Runner
	ctx      context.Context
	app      *model.App
	cancel   context.CancelFunc
	results  chan any
	workers  *sync.WaitGroup
	queue    *queue
	catalog  text.Catalog
	input    textinput.Model
	height   int
	width    int
	setting  int
	logTop   int
	dropped  int
	tab      tab
	help     bool
	detail   bool
	editing  bool
	follow   bool
	quitting bool
}

// New builds a renderer; Close must be called once the program has ended.
func New(opts Options) *Model {
	ctx, cancel := context.WithCancel(context.Background())
	catalog := opts.Catalog
	if catalog == nil {
		catalog = text.English()
	}
	profile := opts.Profile
	if opts.NoColour {
		profile = termenv.Ascii
	}
	input := textinput.New()
	input.Prompt = marker
	// A static cursor avoids a blink timer redrawing the screen.
	input.Cursor.SetMode(cursor.CursorStatic)
	return &Model{
		ctx: ctx, cancel: cancel, app: model.NewApp(opts.LogCapacity, opts.Clock), runner: opts.Runner, catalog: catalog,
		results: make(chan any), workers: &sync.WaitGroup{}, queue: newQueue(), styles: newStyles(profile), input: input, follow: true,
	}
}

// Init starts the serial worker, loads every snapshot, subscribes to events
// and starts listening.
func (m *Model) Init() tea.Cmd {
	m.workers.Add(1)
	go m.serve()
	m.start(m.app.Init())
	return m.listen()
}

// Close cancels outstanding work, closing the event subscription, and waits
// for every worker to return.
func (m *Model) Close() {
	m.cancel()
	m.workers.Wait()
}

// listen waits for the next worker result; exactly one listener is pending.
func (m *Model) listen() tea.Cmd {
	return func() tea.Msg {
		select {
		case msg := <-m.results:
			return resultMsg{msg: msg}
		case <-m.ctx.Done():
			return nil
		}
	}
}

func (m *Model) dispatch(msg model.Msg) { m.start(m.app.Update(msg)) }

// Update applies one Bubble Tea message.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case resultMsg:
		switch result := msg.msg.(type) {
		case panicMsg:
			panic(result.value)
		case model.Msg:
			m.dispatch(result)
			m.editResult(result)
			m.settle()
		}
		return m, m.listen()
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.input.Width = max(msg.Width-inputMargin, 1)
	case tea.KeyMsg:
		return m, m.key(msg)
	case tea.MouseMsg:
		return m, m.mouse(msg)
	}
	return m, nil
}

func (m *Model) quit() tea.Cmd {
	m.quitting = true
	m.cancel()
	return tea.Quit
}

// TerminalProfile detects the colour capability of a terminal output from
// TERM and COLORTERM; it never queries the terminal.
func TerminalProfile(output io.Writer) Profile {
	return termenv.NewOutput(output).ColorProfile()
}

// Run shows the renderer on a terminal until the user quits or ctx ends. The
// terminal is restored on quit, interrupt and recovered panics.
func Run(ctx context.Context, opts Options, input io.Reader, output io.Writer) error {
	m := New(opts)
	defer m.Close()
	program := tea.NewProgram(m, tea.WithContext(ctx), tea.WithInput(input), tea.WithOutput(output), tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := program.Run()
	switch {
	case err == nil, errors.Is(err, tea.ErrInterrupted):
		return nil
	case errors.Is(err, tea.ErrProgramKilled) && ctx.Err() != nil:
		return ctx.Err()
	default:
		return fmt.Errorf("terminal interface: %w", err)
	}
}

func (m *Model) editResult(result model.Msg) {
	if written, ok := result.(model.SettingWritten); ok && m.editing && written.Key == m.field().Path && written.Err == nil {
		m.stopEdit()
	}
}
