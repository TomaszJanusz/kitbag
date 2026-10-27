package model

import (
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
)

func agentFixture() AgentsSnapshot {
	return AgentsSnapshot{
		Host: "mac", Kinds: []string{"claude", "codex", "generic"}, Warnings: []string{"No manifest warning"},
		Instances: []AgentInstance{
			{Target: "claude", Name: "main", AgentCandidate: AgentCandidate{Kind: "claude", Dir: "/home/u/.claude", Source: AgentSourceDeclared, Presence: PresencePresent}, Enabled: true, Settings: true, ActiveHere: true},
			{Target: "claude", Name: "second", AgentCandidate: AgentCandidate{Kind: "claude", Dir: "/home/u/.claude_2", Source: AgentSourceDeclared, Presence: PresenceMissing}, Hosts: []string{"mac"}, Enabled: true},
			{Target: "codex", Name: "default", AgentCandidate: AgentCandidate{Kind: "codex", Dir: "/home/u/.codex", Source: AgentSourceDefault, Presence: PresencePresent}, Implicit: true, ExcludedHosts: []string{"runner"}},
		},
		Candidates: []AgentCandidate{
			{Kind: "claude", Dir: "/home/u/.claude-work", Source: AgentSourceProbe, Presence: PresencePresent},
			{Kind: "claude", Dir: "/srv/claude", Source: AgentSourceEnvironment, Variable: "CLAUDE_CONFIG_DIR", Presence: PresenceOutsideHome},
			{Kind: "codex", Dir: "/home/u/.codex-alt", Source: AgentSourceProbe, Presence: PresenceMissing},
		},
	}
}

func loadedAgents(t *testing.T) *Agents {
	t.Helper()
	m := NewAgents()
	cmd := only(t, m.Init())
	expect(t, cmd == LoadAgents{Seq: 1}, "init=%#v", cmd)
	m.Update(AgentsLoaded{Seq: 1, Snapshot: agentFixture()})
	return m
}

// agentsTexts collects every message of a view for catalog checks.
func agentsTexts(v AgentsView) []text.Message {
	messages := []text.Message{v.Title, v.Headline}
	messages = append(messages, v.Warnings...)
	for _, row := range v.Rows {
		messages = append(messages, row.Label, row.Dir, row.Source, row.Presence, row.Scope)
	}
	if v.Detail != nil {
		messages = append(messages, v.Detail.Title)
		for _, row := range v.Detail.Rows {
			messages = append(messages, row.Label, row.Value)
		}
	}
	for _, control := range v.Actions {
		messages = append(messages, control.Label)
	}
	if v.Problem != nil {
		messages = append(messages, *v.Problem)
	}
	return messages
}

func TestAgentsRowsGroupedByKind(t *testing.T) {
	t.Parallel()
	view := loadedAgents(t).View()
	labels := make([]string, 0, len(view.Rows))
	for _, row := range view.Rows {
		labels = append(labels, row.Kind+" "+render(row.Label)+" "+render(row.Scope))
	}
	want := []string{
		"claude claude/main all hosts", "claude claude/second only on mac", "claude not declared ", "claude not declared ",
		"codex codex (default directory) all hosts except runner", "codex not declared ",
	}
	expect(t, strings.Join(labels, "|") == strings.Join(want, "|"), "rows=%q", labels)
	expect(t, view.Phase == Ready && view.Rows[0].Selected && view.Rows[0].Declared && !view.Rows[4].Declared && view.Rows[4].Implicit, "flags %+v", view.Rows)
	expect(t, render(view.Headline) == "2 declared instances, 3 other directories found", "headline=%q", render(view.Headline))
	expect(t, render(view.Rows[3].Source) == "from CLAUDE_CONFIG_DIR" && render(view.Rows[3].Presence) == "outside the home directory; cannot be declared", "candidate %+v", view.Rows[3])
	checkText(t, agentsTexts(view)...)
	for _, m := range agentsTexts(view) {
		expect(t, !strings.Contains(render(m), "agents."), "raw key in %q", render(m))
	}
}

func TestAgentsSelectionFollowsDirectory(t *testing.T) {
	t.Parallel()
	m := loadedAgents(t)
	m.Update(MoveAgent{Delta: 2})
	expect(t, m.View().Rows[2].Selected, "move")
	snapshot := agentFixture()
	snapshot.Instances = append([]AgentInstance{{Target: "claude", Name: "a", AgentCandidate: AgentCandidate{Kind: "claude", Dir: "/home/u/.a"}}}, snapshot.Instances...)
	cmd := only(t, m.Update(Refresh{}))
	m.Update(AgentsLoaded{Seq: as[LoadAgents](t, cmd).Seq, Snapshot: snapshot})
	expect(t, m.View().Rows[3].Selected && m.View().Detail != nil, "selection did not follow the directory")
	m.Update(SelectAgent{Index: 99})
	expect(t, m.View().Rows[6].Selected, "select clamps")
	m.Update(AgentsLoaded{Seq: 2, Snapshot: AgentsSnapshot{}})
	view := m.View()
	expect(t, len(view.Rows) == 7 && view.Rows[6].Selected, "stale seq applied: %d rows", len(view.Rows))
}

func TestAgentsLoadsAndFailures(t *testing.T) {
	t.Parallel()
	m := NewAgents()
	m.Init()
	m.Update(AgentsLoaded{Seq: 1, Err: &Error{Kind: ErrUnavailable}})
	expect(t, m.View().Phase == Failed && render(m.View().Headline) == "kitbag is not reachable", "failed")
	probe := only(t, m.Update(ProbeAgents{}))
	expect(t, probe == LoadAgents{Seq: 2, Probe: true}, "probe=%#v", probe)
	m.Update(AgentsLoaded{Seq: 2, Snapshot: AgentsSnapshot{}})
	expect(t, render(m.View().Headline) == "No agents are declared or found on this machine.", "empty")
	m.Update(AgentsLoaded{Seq: as[LoadAgents](t, only(t, m.Update(SyncFinished{}))).Seq, Err: errBoom})
	expect(t, m.View().Phase == Ready && m.View().Problem != nil, "known listing must stay ready")
	for _, msg := range []Msg{EventReceived{Event: Event{Kind: EventConnected}}, EventReceived{Event: Event{Kind: EventSyncFinished}}, AgentChangeApplied{}} {
		expect(t, len(m.Update(msg)) == 1, "%T must reload", msg)
	}
	for _, msg := range []Msg{EventReceived{Event: Event{Kind: EventPollFinished}}, AgentChangeApplied{Err: errBoom}} {
		expect(t, len(m.Update(msg)) == 0, "%T must not reload", msg)
	}
	expect(t, len(m.Update(ProbeAgents{})) == 1 && as[LoadAgents](t, m.Update(Refresh{})[0]).Probe == false, "refresh drops the probe")
}

func TestAgentsActions(t *testing.T) {
	t.Parallel()
	m := loadedAgents(t)
	edit := OpenAgentForm{Op: AgentEdit, Kind: "claude", Target: "claude", Name: "main", Dir: "/home/u/.claude"}
	expect(t, action(t, m.View().Actions, edit).Enabled, "edit of a declared instance")
	expect(t, action(t, m.View().Actions, OpenAgentForm{Op: AgentAdd, Kind: "claude"}).Enabled, "add of its kind")
	m.Update(SelectAgent{Index: 2})
	add := OpenAgentForm{Op: AgentAdd, Kind: "claude", Dir: "/home/u/.claude-work"}
	expect(t, action(t, m.View().Actions, add).Enabled, "add prefilled from a candidate")
	expect(t, len(m.View().Actions) == 3, "remove of a candidate")
	m.Update(SelectAgent{Index: 4})
	expect(t, len(m.View().Actions) == 3, "edit of an implicit default")
	expect(t, action(t, m.View().Actions, ProbeAgents{}).Enabled && action(t, m.View().Actions, Refresh{}).Enabled, "probe and refresh")
	expect(t, len(m.View().Detail.Rows) == 7, "instance detail rows")
	checkText(t, agentsTexts(m.View())...)
}

func TestAgentTextFallbacks(t *testing.T) {
	t.Parallel()
	expect(t, render(AgentSourceText("other", "")) == "other", "unknown source")
	expect(t, render(AgentPresenceText("other")) == "other", "unknown presence")
	for _, p := range []AgentPresence{PresencePresent, PresenceMissing, PresenceNotDirectory, PresenceSymlink, PresenceOutsideHome, PresenceUnreadable} {
		checkText(t, AgentPresenceText(p))
	}
	for _, s := range []AgentSource{AgentSourceDefault, AgentSourceEnvironment, AgentSourceDeclared, AgentSourceProbe} {
		checkText(t, AgentSourceText(s, "X"))
	}
}
