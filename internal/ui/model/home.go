package model

import (
	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
)

// HomeAction identifies a destination/action without prescribing renderer keys.
type HomeAction int

// Home menu order is shared by terminal and other renderers.
const (
	HomeSync HomeAction = iota
	HomeAgents
	HomeDrift
	HomeSettings
	HomeActivity
)

// HomeItem is a compact menu row; Value is optional context, never raw state.
type HomeItem struct {
	Label, Shortcut, Value text.Message
	Action                 HomeAction
	Enabled                bool
}

// HomeView keeps the landing screen deliberately small. Full details remain in
// StatusView; warning priority and relative check text belong in this layer.
type HomeView struct {
	Headline, Secondary         text.Message
	Repository, Branch, Version text.Message
	Items                       []HomeItem
	Level, SecondaryLevel       Severity
}

// Home projects the current snapshots without changing any model state.
func (a *App) Home(version string) HomeView {
	s := a.Status
	view := s.View()
	headline, level := s.homeHeadline(view)
	secondary, secondaryLevel := s.homeSecondary(view)
	d := s.snapshot.Drift
	drift := d.UpdateNeeded + d.LocalDrift + d.Conflict + d.UnmanagedObstacle
	driftValue := text.Message{}
	if drift > 0 {
		driftValue = text.New(text.HomeCount, number(drift))
	}
	return HomeView{
		Headline: headline, Level: level, Secondary: secondary, SecondaryLevel: secondaryLevel,
		Repository: text.Plain(s.snapshot.Repository), Branch: text.Plain(s.snapshot.Git.Branch), Version: text.New(text.HomeVersion, text.Plain(version)),
		Items: []HomeItem{
			{Action: HomeSync, Label: text.New(text.ActionSyncNow), Shortcut: text.New(text.HomeSyncKey), Enabled: s.canSync()},
			{Action: HomeAgents, Label: text.New(text.AgentsTitle), Shortcut: text.New(text.HomeAgentsKey), Value: text.New(text.HomeAgentCount, number(len(a.Agents.snapshot.Instances))), Enabled: true},
			{Action: HomeDrift, Label: text.New(text.DriftTitle), Shortcut: text.New(text.HomeDriftKey), Value: driftValue, Enabled: true},
			{Action: HomeSettings, Label: text.New(text.SettingsTitle), Shortcut: text.New(text.HomeSettingsKey), Enabled: true},
			{Action: HomeActivity, Label: text.New(text.LogTitle), Shortcut: text.New(text.HomeActivityKey), Enabled: true},
		},
	}
}

func (m *Status) homeHeadline(view StatusView) (text.Message, Severity) {
	if view.Problem != nil {
		return *view.Problem, Failure
	}
	d := m.snapshot.Drift
	if m.snapshot.ToolTooOld != nil || d.Conflict+d.UnmanagedObstacle > 0 {
		return view.Headline, Failure
	}
	if d.LocalDrift+d.UpdateNeeded > 0 {
		return view.Headline, Warning
	}
	check := m.snapshot.LastCheck
	if m.checkedHeadline(check) {
		return text.New(text.HomeChecked, view.Headline, RelativeText(m.clock, check.At)), Info
	}
	return view.Headline, Info
}

func (m *Status) homeSecondary(view StatusView) (text.Message, Severity) {
	if warning := m.homeWarning(); warning.Key != "" {
		return warning, Warning
	}
	if len(view.Warnings) > 0 {
		return view.Warnings[0], Warning
	}
	for i := len(view.Notices) - 1; i >= 0; i-- {
		if view.Notices[i].Key != text.StatusVersions {
			return view.Notices[i], Info
		}
	}
	check := m.snapshot.LastCheck
	if m.snapshot.Daemon && check != nil {
		return text.New(text.HomeNextCheck, text.Plain(m.clock.Stamp(check.NextAt))), Info
	}
	return text.Message{}, Info
}

func (m *Status) homeWarning() text.Message {
	s := m.snapshot
	switch {
	case s.ServiceRecovering:
		return text.New(text.StatusServiceRecovering)
	case s.Daemon && s.BinaryVersion != "" && s.DaemonVersion != s.BinaryVersion:
		return text.New(text.StatusVersionMismatch)
	case s.ServiceStale:
		return text.New(text.StatusServiceStale)
	default:
		warnings := m.checkWarnings()
		if len(warnings) > 0 {
			return warnings[len(warnings)-1]
		}
		return text.Message{}
	}
}

func (m *Status) checkedHeadline(check *LastCheck) bool {
	return m.known && !m.syncing && !m.otherSync && m.snapshot.Daemon && check != nil && check.Outcome == "ok"
}
