package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/TomaszJanusz/kitbag/internal/ui/model"
)

// driver runs the renderer synchronously: every queued command runs on the
// test goroutine and its results are fed back before the next input, so
// screens at any step are deterministic. Flows that need the real program
// loop use session (teatest) instead.
type driver struct {
	t      *testing.T
	m      *Model
	client *fakeClient
}

func drive(t *testing.T, client *fakeClient, width, height int) *driver {
	t.Helper()
	m := New(Options{Runner: model.Runner{Client: client, Now: clock}, Profile: termenv.Ascii, Clock: screenClock(t)})
	t.Cleanup(m.Close)
	m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	var cmds []model.Cmd
	for _, cmd := range m.app.Init() {
		if _, subscribe := cmd.(model.Subscribe); !subscribe {
			cmds = append(cmds, cmd)
		}
	}
	m.start(cmds)
	d := &driver{t: t, m: m, client: client}
	d.settle()
	return d
}

// settle runs queued commands until none is left.
func (d *driver) settle() {
	for {
		cmd, ok := d.m.queue.pop()
		if !ok {
			return
		}
		var results []model.Msg
		d.m.runner.Run(context.Background(), cmd, func(msg model.Msg) { results = append(results, msg) })
		for _, msg := range results {
			d.m.Update(resultMsg{msg: msg})
		}
	}
}

func (d *driver) press(keys ...string) {
	for _, key := range keys {
		d.m.Update(keyMsg(key))
		d.settle()
	}
}

func (d *driver) send(msg tea.Msg) {
	d.m.Update(msg)
	d.settle()
}

func (d *driver) click(x, y int) {
	d.send(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
}

func (d *driver) screen() string { return plainText([]byte(d.m.View())) }

// clickText clicks the first cell of the first screen occurrence of fragment.
func (d *driver) clickText(fragment string) {
	d.t.Helper()
	for y, line := range strings.Split(d.screen(), "\n") {
		if x := strings.Index(line, fragment); x >= 0 {
			d.click(ansi.StringWidth(line[:x]), y)
			return
		}
	}
	d.t.Fatalf("%q is not on the screen:\n%s", fragment, d.screen())
}

// expect fails unless the screen shows every fragment.
func (d *driver) expect(fragments ...string) {
	d.t.Helper()
	screen := d.screen()
	for _, fragment := range fragments {
		if !strings.Contains(screen, fragment) {
			d.t.Fatalf("screen lacks %q:\n%s", fragment, screen)
		}
	}
}

// refuse fails if the screen shows any fragment.
func (d *driver) refuse(fragments ...string) {
	d.t.Helper()
	screen := d.screen()
	for _, fragment := range fragments {
		if strings.Contains(screen, fragment) {
			d.t.Fatalf("screen shows %q:\n%s", fragment, screen)
		}
	}
}

func (d *driver) step() model.AgentStep { return d.m.app.AgentForm.View().Step }

func (d *driver) golden(name string) {
	d.t.Helper()
	screen := d.m.View()
	assertScreenFits(d.t, screen, d.m.width, d.m.height)
	golden(d.t, name, screen)
}
