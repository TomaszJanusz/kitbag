package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/TomaszJanusz/kitbag/internal/ui/model"
	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
)

type (
	actionHit struct {
		control model.Control
		rect    Rect
	}
	actionLayout struct {
		lines []string
		hits  []actionHit
	}
)

func (m *Model) screenControls() []model.Control {
	switch m.tab {
	case statusTab:
		return m.app.Status.View().Actions
	case settingsTab:
		if len(m.app.Settings.View().Fields) > 0 {
			return m.field().Actions
		}
	case driftTab:
		if m.app.Drift.View().Detail != nil {
			return []model.Control{{Label: text.New(text.ActionDetails), Enabled: true}}
		}
	case logTab:
		return []model.Control{{Label: text.New(text.ActionFollow), Enabled: true}}
	case agentsTab:
		return m.app.Agents.View().Actions
	case tabCount:
	}
	return nil
}

func (m *Model) actionLayout() actionLayout {
	controls := m.screenControls()
	if len(controls) == 0 {
		return actionLayout{}
	}
	lines := []string{""}
	hits := make([]actionHit, 0, len(controls))
	x, y := 0, 0
	for i, control := range controls {
		if unset, ok := control.Intent.(model.UnsetSetting); ok {
			control.Intent = model.RequestUnsetSetting(unset)
		}
		if m.height < homeCompactHeight && !control.Enabled {
			continue
		}
		if _, ok := control.Intent.(model.RequestUnsetSetting); ok {
			control.Label = text.New(text.ActionRemoveShort)
		}
		label := m.buttonView(button{control: control, primary: i == 0}, m.width)
		size := ansi.StringWidth(label)
		if x > 0 && x+size > m.width {
			lines = append(lines, "")
			x = 0
			y++
		}
		hits = append(hits, actionHit{rect: Rect{X: x, Y: y, Width: size, Height: 1}, control: control})
		lines[y] += label
		x += size
	}
	top := m.height - 1 - len(lines)
	for i := range hits {
		hits[i].rect.Y += top
	}
	return actionLayout{lines: lines, hits: hits}
}

func (m *Model) bodyHeight() int { return max(m.height-chromeLines-len(m.actionLayout().lines), 1) }

func (m *Model) clickAction(x, y int) bool {
	for _, hit := range m.actionLayout().hits {
		if hit.rect.Contains(x, y) {
			if hit.control.Enabled {
				m.activateAction(hit.control)
			}
			return true
		}
	}
	return false
}

func (m *Model) surfaceLine(line string) string {
	if m.styles.mono {
		return line
	}
	sample := m.styles.screen.Render("x")
	prefix, _, _ := strings.Cut(sample, "x")
	return prefix + strings.ReplaceAll(cellLine(line, m.width), "\x1b[0m", "\x1b[0m"+prefix) + "\x1b[0m"
}

func (m *Model) activateAction(control model.Control) {
	if control.Intent == nil {
		switch m.tab {
		case driftTab:
			m.driftKey(keyEnter)
		case logTab:
			m.logKey(keyFollow)
		case statusTab, settingsTab, agentsTab, tabCount:
		}
		return
	}
	m.dispatch(control.Intent)
	if _, ok := control.Intent.(model.RevertSetting); ok && m.editing {
		m.stopEdit()
	}
}
