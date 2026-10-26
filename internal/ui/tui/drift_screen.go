package tui

import (
	"github.com/charmbracelet/x/ansi"
)

type driftScreen struct {
	lines []string
	list  listLayout
	panes PaneLayout
}

func (m *Model) driftScreen(height int) driftScreen {
	panes := MasterDetail(m.width, height, 0, height)
	if panes.Stacked || m.detail {
		return driftScreen{lines: m.driftView(height), list: m.driftLayout(height), panes: panes}
	}
	local := *m
	local.width = panes.Master.Width
	layout := local.driftLayout(height)
	left := local.driftView(height)
	for i := range left {
		left[i] = ansi.Truncate(left[i], panes.Master.Width, ellipsis)
	}
	var right []string
	if detail := m.app.Drift.View().Detail; detail != nil {
		local.width = panes.Detail.Width
		right = local.driftDetail(*detail)
	}
	return driftScreen{lines: composePanes(panes, left, right), list: layout, panes: panes}
}
