package model

import (
	"slices"
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// hostsChoice is the scope chosen on the hosts step.
type hostsChoice int

// Scopes, in option order (keep only when editing; here only with a host).
const (
	hostsKeep hostsChoice = iota
	hostsHere
	hostsAll
	hostsPick
)

// schemaHosts is the first manifest schema with per-instance hosts.
const schemaHosts = 2

// hostsDraft is the hosts step's state; text is the typed list for pick.
type hostsDraft struct {
	text   string
	choice hostsChoice
}

// perHost reports whether the manifest can restrict instances to hosts.
func (m *AgentForm) perHost() bool { return m.snapshot.Schema >= schemaHosts }

// defaultHosts keeps an edited instance's hosts; an add uses this machine
// only when the manifest allows it (ADR 12), else all machines.
func (m *AgentForm) defaultHosts() hostsDraft {
	switch {
	case m.draft.Op == AgentEdit:
		return hostsDraft{choice: hostsKeep}
	case m.perHost() && m.snapshot.Host != "":
		return hostsDraft{choice: hostsHere}
	}
	return hostsDraft{choice: hostsAll}
}

// hostsChoices lists the scopes offered on the hosts step.
func (m *AgentForm) hostsChoices() []hostsChoice {
	var choices []hostsChoice
	if m.draft.Op == AgentEdit {
		choices = append(choices, hostsKeep)
	}
	if m.snapshot.Host != "" {
		choices = append(choices, hostsHere)
	}
	return append(choices, hostsAll, hostsPick)
}

func (m *AgentForm) chooseHosts(index int) bool {
	choices := m.hostsChoices()
	if index < 0 || index >= len(choices) {
		return false
	}
	m.hosts.choice = choices[index]
	m.applyHosts()
	return true
}

func (m *AgentForm) editHosts(value string) bool {
	if !m.picking() {
		return false
	}
	m.hosts.text = value
	m.applyHosts()
	return true
}

// picking reports whether the hosts step edits a typed list.
func (m *AgentForm) picking() bool {
	return m.current() == AgentStepHosts && m.perHost() && m.hosts.choice == hostsPick
}

// applyHosts writes the chosen scope into the draft. Nothing is sent for a
// schema-1 manifest, which the server would refuse.
func (m *AgentForm) applyHosts() {
	m.draft.Hosts, m.draft.AllHosts = "", false
	if !m.perHost() {
		return
	}
	switch m.hosts.choice {
	case hostsHere:
		m.draft.Hosts = m.snapshot.Host
	case hostsAll:
		m.draft.AllHosts = m.draft.Op == AgentEdit
	case hostsPick:
		m.draft.Hosts = strings.Join(SplitHosts(m.hosts.text), ",")
	case hostsKeep:
	}
}

// SplitHosts parses a comma-separated host list, dropping blanks.
func SplitHosts(value string) []string {
	var hosts []string
	for _, host := range strings.Split(value, ",") {
		if host = strings.TrimSpace(host); host != "" {
			hosts = append(hosts, host)
		}
	}
	return hosts
}

// invalidHosts validates a typed list with the manifest's host grammar.
func (m *AgentForm) invalidHosts() text.Message {
	if !m.picking() {
		return text.Message{}
	}
	hosts := SplitHosts(m.draft.Hosts)
	if len(hosts) == 0 {
		return text.New(text.AgentInvalidHosts)
	}
	for i, host := range hosts {
		if manifest.CheckHostName(host) != nil {
			return text.New(text.AgentInvalidHost, text.Plain(host))
		}
		if slices.Contains(hosts[:i], host) {
			return text.New(text.AgentInvalidHostTwice, text.Plain(host))
		}
	}
	return text.Message{}
}

// hostsWarnings name chosen hosts that are neither this machine nor named
// in kitbag.toml (likely typos).
func (m *AgentForm) hostsWarnings() []text.Message {
	var warnings []text.Message
	for _, host := range SplitHosts(m.draft.Hosts) {
		if host != m.snapshot.Host && !slices.Contains(m.snapshot.KnownHosts, host) {
			warnings = append(warnings, text.New(text.AgentWarnUnknownHost, text.Plain(host)))
		}
	}
	return warnings
}

// hostsOptions labels the scopes.
func (m *AgentForm) hostsOptions() []AgentOption {
	choices := m.hostsChoices()
	options := make([]AgentOption, 0, len(choices))
	for _, choice := range choices {
		option := AgentOption{Selected: choice == m.hosts.choice}
		switch choice {
		case hostsKeep:
			option.Label = text.New(text.WizardHostsKeep, m.currentScope())
		case hostsHere:
			option.Label = text.New(text.WizardHostsHere, text.Plain(m.snapshot.Host))
		case hostsAll:
			option.Label = text.New(text.WizardHostsAll)
		case hostsPick:
			option.Label = text.New(text.WizardHostsPick)
			if len(m.snapshot.KnownHosts) > 0 {
				option.Detail = text.New(text.WizardHostsKnown, text.Plain(strings.Join(m.snapshot.KnownHosts, ", ")))
			}
		}
		options = append(options, option)
	}
	return options
}

// currentScope describes the edited instance's hosts as the list shows them.
func (m *AgentForm) currentScope() text.Message {
	for _, i := range m.snapshot.Instances {
		if !i.Implicit && m.edited(i) {
			return agentScope(i)
		}
	}
	return text.New(text.AgentsScopeAll)
}
