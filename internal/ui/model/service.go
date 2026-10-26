package model

import "github.com/TomaszJanusz/kitbag/internal/ui/model/text"

func (m *Status) needsServiceRefresh() bool {
	s := m.snapshot
	return s.ServiceInstalled && (s.ServiceStale || s.Daemon && s.DaemonVersion != s.BinaryVersion)
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
	s := m.snapshot
	if s.Daemon && s.BinaryVersion != "" {
		view.Notices = append(view.Notices, text.New(text.StatusVersions, text.Plain(s.DaemonVersion), text.Plain(s.BinaryVersion)))
		if s.DaemonVersion != s.BinaryVersion {
			view.Warnings = append(view.Warnings, text.New(text.StatusVersionMismatch))
		}
	}
	if s.ServiceStale {
		view.Warnings = append(view.Warnings, text.New(text.StatusServiceStale))
	}
	view.ConfirmRefresh = m.confirmRefresh
	switch {
	case m.confirmRefresh:
		view.Notices = append(view.Notices, text.New(text.ServiceRefreshConfirm))
	case m.refreshingService:
		view.Notices = append(view.Notices, text.New(text.ServiceRefreshing))
	case m.refreshProblem != nil:
		view.Warnings = append(view.Warnings, ProblemText(m.refreshProblem))
	case m.refreshed:
		view.Notices = append(view.Notices, text.New(text.ServiceRefreshed))
	}
}

func (m *Status) canRefreshService() bool {
	return m.needsServiceRefresh() && !m.refreshingService && !m.syncing && !m.otherSync
}
