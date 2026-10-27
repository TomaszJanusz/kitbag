package model

import "github.com/TomaszJanusz/kitbag/internal/ui/model/text"

func (m *Status) needsServiceRefresh() bool {
	s := m.snapshot
	return !s.ServiceRecovering && s.ServiceInstalled && (s.ServiceStale || s.Daemon && s.DaemonVersion != s.BinaryVersion)
}

func (m *Status) serviceRefresh(msg Msg) []Cmd {
	switch msg := msg.(type) {
	case RequestServiceRefresh:
		m.confirmRefresh = m.canRefreshService()
	case ConfirmServiceRefresh:
		confirmed := m.confirmRefresh && msg.Accept && m.canRefreshService()
		m.confirmRefresh = false
		if confirmed {
			m.refreshingService, m.refreshed, m.refreshProblem = true, false, nil
			return []Cmd{RunServiceRefresh{}}
		}
	case ServiceRefreshed:
		if m.refreshingService {
			m.refreshingService = false
			m.refreshProblem, m.refreshed = Classify(msg.Err), msg.Err == nil
			return m.load()
		}
	}
	return nil
}

func (m *Status) serviceRefreshView(view *StatusView) {
	m.serviceVersionView(view)
	view.ConfirmRefresh = m.confirmRefresh
	view.Dialog = m.Dialog()
	switch {
	case m.refreshingService:
		view.Notices = append(view.Notices, text.New(text.ServiceRefreshing))
	case m.refreshProblem != nil:
		view.Warnings = append(view.Warnings, ProblemText(m.refreshProblem))
	case m.refreshed:
		view.Notices = append(view.Notices, text.New(text.ServiceRefreshed))
	}
}

// ServiceRefreshDialog is the stable ID of the repair confirmation window.
const ServiceRefreshDialog = "status.service_refresh"

// Dialog returns the repair confirmation while it is open. Yes confirms; No,
// Esc and outside clicks cancel; nothing runs before confirmation.
func (m *Status) Dialog() *DialogView {
	if !m.confirmRefresh {
		return nil
	}
	dialog := ConfirmDialog(ServiceRefreshDialog, text.New(text.ActionServiceRefresh), text.New(text.ServiceRefreshConfirm),
		ConfirmServiceRefresh{Accept: true}, ConfirmServiceRefresh{})
	return &dialog
}

func (m *Status) canRefreshService() bool {
	return m.needsServiceRefresh() && !m.refreshingService && !m.syncing && !m.otherSync
}

func (m *Status) serviceVersionView(view *StatusView) {
	s := m.snapshot
	if s.Daemon && s.BinaryVersion != "" {
		view.Notices = append(view.Notices, text.New(text.StatusVersions, text.Plain(s.DaemonVersion), text.Plain(s.BinaryVersion)))
		if s.DaemonVersion != s.BinaryVersion && !s.ServiceRecovering {
			view.Warnings = append(view.Warnings, text.New(text.StatusVersionMismatch))
		}
	}
	if s.ServiceRecovering {
		view.Notices = append(view.Notices, text.New(text.StatusServiceRecovering))
	}
	if s.ServiceStale && !s.ServiceRecovering {
		view.Warnings = append(view.Warnings, text.New(text.StatusServiceStale))
	}
}
