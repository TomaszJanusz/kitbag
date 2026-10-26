package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// PaneBreakpoint is the minimum width for side-by-side master/detail panes.
const PaneBreakpoint = 100

const (
	paneGap       = 3
	masterShare   = 3
	paneShares    = 5
	centerDivisor = 2
	borderEdges   = 2
)

// Rect is a half-open cell rectangle used for rendering and hit testing.
type Rect struct{ X, Y, Width, Height int }

// Contains reports whether a terminal cell is inside r.
func (r Rect) Contains(x, y int) bool {
	return x >= r.X && y >= r.Y && x < r.X+r.Width && y < r.Y+r.Height
}

// PaneLayout places a master and detail pane; Stacked is true below 100 cells.
type PaneLayout struct {
	Master, Detail Rect
	Stacked        bool
}

// MasterDetail computes responsive geometry. masterWidth=0 chooses a proportion;
// stackedRows controls the list height when panes must be stacked vertically.
func MasterDetail(width, height, masterWidth, stackedRows int) PaneLayout {
	if width < PaneBreakpoint {
		rows := min(max(stackedRows, 1), max(height-1, 1))
		return PaneLayout{Master: Rect{Width: width, Height: rows}, Detail: Rect{Y: rows + 1, Width: width, Height: max(height-rows-1, 0)}, Stacked: true}
	}
	if masterWidth <= 0 {
		masterWidth = width * masterShare / paneShares
	}
	masterWidth = min(max(masterWidth, MinWidth), width-MinWidth-paneGap)
	return PaneLayout{Master: Rect{Width: masterWidth, Height: height}, Detail: Rect{X: masterWidth + paneGap, Width: width - masterWidth - paneGap, Height: height}}
}

// cellLine truncates and pads without counting ANSI escapes as cells.
func cellLine(line string, width int) string {
	line = ansi.Truncate(line, max(width, 0), ellipsis)
	return line + strings.Repeat(" ", max(width-ansi.StringWidth(line), 0))
}

func composePanes(layout PaneLayout, left, right []string) []string {
	if layout.Stacked {
		return append(append(pad(left[:min(len(left), layout.Master.Height)], layout.Master.Height), ""), right[:min(len(right), layout.Detail.Height)]...)
	}
	lines := make([]string, layout.Master.Height)
	for i := range lines {
		lines[i] = cellLine(lineAt(left, i), layout.Master.Width) + " | " + cellLine(lineAt(right, i), layout.Detail.Width)
	}
	return lines
}

func lineAt(lines []string, index int) string {
	if index < len(lines) {
		return lines[index]
	}
	return ""
}
