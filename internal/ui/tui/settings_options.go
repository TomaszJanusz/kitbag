package tui

import (
	"github.com/charmbracelet/x/ansi"

	"github.com/TomaszJanusz/kitbag/internal/ui/model"
)

type (
	optionHit    struct{ start, end, y, index int }
	optionLayout struct {
		lines []string
		hits  []optionHit
	}
)

// optionLayout draws options with a shared geometry for pointer hit testing.
// Selection is textual so monochrome terminals have the same controls.
func (m *Model) optionLayout(field model.FieldView) ([]string, optionLayout) {
	layout := optionLayout{}
	line, column, y := "", 0, 0
	for i, option := range field.Options {
		label := " " + m.say(option.Label) + " "
		if option.Selected {
			label = "[" + m.say(option.Label) + "]"
		}
		width := ansi.StringWidth(label)
		if column > 0 && column+width > m.width {
			layout.lines = append(layout.lines, line)
			line, column, y = "", 0, y+1
		}
		layout.hits = append(layout.hits, optionHit{start: column, end: min(column+width, m.width), y: y, index: i})
		line += label
		column += width
	}
	if line != "" {
		layout.lines = append(layout.lines, line)
	}
	return layout.lines, layout
}
