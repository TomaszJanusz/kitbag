package model

import (
	"slices"
	"strings"
	"testing"
)

// openWithHosts opens the form on a manifest naming hosts mac and ci.
func openWithHosts(t *testing.T, open OpenAgentForm) *AgentForm {
	t.Helper()
	snapshot := agentFixture()
	snapshot.KnownHosts = []string{"ci", "mac"}
	m := NewAgentForm()
	m.Update(AgentsLoaded{Seq: 1, Snapshot: snapshot})
	m.Update(open)
	expect(t, m.Open(), "form not open")
	return m
}

// toHosts drives an add to the hosts step.
func toHosts(t *testing.T) *AgentForm {
	t.Helper()
	m := openWithHosts(t, OpenAgentForm{Op: AgentAdd, Kind: "claude", Name: "work", Dir: "~/.claude-work"})
	m.Update(AgentFormNext{})
	m.Update(AgentFormNext{})
	m.Update(AgentFormNext{})
	return m
}

func optionLabels(v AgentFormView) []string {
	labels := make([]string, 0, len(v.Options))
	for _, option := range v.Options {
		labels = append(labels, render(option.Label))
	}
	return labels
}

// TestAgentFormHostsStep shows the choices: this machine only by default,
// all machines, or a typed list.
func TestAgentFormHostsStep(t *testing.T) {
	t.Parallel()
	m := toHosts(t)
	v := m.View()
	expect(t, v.Step == AgentStepHosts && len(v.Steps) == 8 && render(v.Title) == "Machines" && v.TextEntry && !m.Dialog().Fields[0].Enabled, "hosts step %+v", v)
	expect(t, strings.Join(optionLabels(v), "|") == "This machine only (mac)|All machines|Choose machines…" && v.Options[0].Selected, "options %q", optionLabels(v))
	expect(t, render(v.Options[2].Detail) == "Machines named in kitbag.toml: ci, mac", "known hosts %q", render(v.Options[2].Detail))
	checkForm(t, m)
	expect(t, m.draft.Hosts == "mac" && !m.draft.AllHosts, "default draft %+v", m.draft)

	m.Update(ChooseAgentOption{Index: 1})
	expect(t, m.draft.Hosts == "" && !m.draft.AllHosts, "all machines on add sends nothing: %+v", m.draft)
}

// TestAgentFormHostsPick validates a typed list per keystroke and sends it.
func TestAgentFormHostsPick(t *testing.T) {
	t.Parallel()
	m := toHosts(t)
	m.Update(ChooseAgentOption{Index: 2})
	v := m.View()
	expect(t, v.TextEntry && v.Field == "" && invalidText(m) == "Enter at least one host name." && !v.Next.Enabled, "empty pick %+v", v)
	d := m.Dialog()
	expect(t, len(d.Fields) == 1 && d.Fields[0].ID == "hosts" && d.Fields[0].Enabled && render(d.Fields[0].Hint) == "Host names separated by commas, for example mac, runner02.", "field %+v", d.Fields)
	for _, tc := range []struct{ value, want string }{
		{"mac, my mac", "my mac is not a valid host name: use letters, digits, dots, underscores or hyphens."},
		{"mac,mac", "mac is listed twice."},
		{" , ", "Enter at least one host name."},
		{" mac , ci ", ""},
	} {
		m.Update(EditDialogField{Dialog: AgentFormDialog, Value: tc.value})
		expect(t, invalidText(m) == tc.want, "%q: invalid %q, want %q", tc.value, invalidText(m), tc.want)
	}
	expect(t, m.draft.Hosts == "mac,ci", "typed draft %+v", m.draft)
	expect(t, len(m.View().Warnings) == 0, "known hosts must not warn")
	m.Update(EditAgentField{Value: "mac, macbok"})
	warnings := m.View().Warnings
	expect(t, len(warnings) == 1 && render(warnings[0]) == "macbok is neither this machine nor named in kitbag.toml; check its spelling.", "unknown host warning %+v", warnings)
	checkForm(t, m)

	m.Update(AgentFormNext{})
	expect(t, m.View().Step == AgentStepSettings, "settings follows hosts")
	m.Update(AgentFormBack{})
	m.Update(ChooseAgentOption{Index: 1})
	m.Update(ChooseAgentOption{Index: 2})
	expect(t, m.View().Field == "mac, macbok", "back and scope changes lost the picked field")
	m.Update(AgentFormNext{})
	preview := as[PreviewAgent](t, only(t, m.Update(AgentFormNext{})))
	expect(t, preview.Change.Hosts == "mac,macbok", "preview change %+v", preview.Change)
}

// TestAgentFormHostsEdit keeps the current machines by default; all
// machines removes the restriction.
func TestAgentFormHostsEdit(t *testing.T) {
	t.Parallel()
	m := openWithHosts(t, OpenAgentForm{Op: AgentEdit, Kind: "claude", Target: "claude", Name: "second", Dir: "/home/u/.claude_2"})
	m.Update(AgentFormNext{})
	v := m.View()
	expect(t, v.Step == AgentStepHosts && len(v.Steps) == 6, "edit hosts step %+v", v)
	expect(t, strings.Join(optionLabels(v), "|") == "Keep the current machines (only on mac)|This machine only (mac)|All machines|Choose machines…" && v.Options[0].Selected, "options %q", optionLabels(v))
	expect(t, m.draft.Hosts == "" && !m.draft.AllHosts, "keep sends nothing: %+v", m.draft)
	m.Update(ChooseAgentOption{Index: 2})
	expect(t, m.draft.AllHosts && m.draft.Hosts == "", "all machines on edit: %+v", m.draft)
	m.Update(ChooseAgentOption{Index: 9})
	expect(t, m.draft.AllHosts, "out of range choice ignored")
	m.Update(AgentFormNext{})
	preview := as[PreviewAgent](t, only(t, m.Update(AgentFormNext{})))
	expect(t, preview.Change.AllHosts && preview.Change.Hosts == "", "edit preview %+v", preview.Change)
}

// TestAgentFormHostsStepAlwaysOffered offers the step on add and edit of a
// schema-1 manifest, and remove sends no hosts.
func TestAgentFormHostsStepAlwaysOffered(t *testing.T) {
	t.Parallel()
	for _, op := range []AgentOp{AgentAdd, AgentEdit} {
		m, _ := openForm(t, OpenAgentForm{Op: op, Kind: "claude", Target: "claude", Name: "second"})
		expect(t, slices.Contains(m.View().Steps, AgentStepHosts), "%s has no hosts step: %v", op, m.View().Steps)
	}
	m, cmds := openForm(t, OpenAgentForm{Op: AgentRemove, Kind: "claude", Target: "claude", Name: "second"})
	preview := as[PreviewAgent](t, only(t, cmds))
	expect(t, !slices.Contains(m.View().Steps, AgentStepHosts) && preview.Change.Hosts == "" && !preview.Change.AllHosts, "remove %+v", preview.Change)
}

// TestAgentFormHostsWithoutHostName offers no "this machine" choice when
// the machine's host name is unknown, and all machines is the add default.
func TestAgentFormHostsWithoutHostName(t *testing.T) {
	t.Parallel()
	snapshot := agentFixture()
	snapshot.Host = ""
	m := NewAgentForm()
	m.Update(AgentsLoaded{Seq: 1, Snapshot: snapshot})
	m.Update(OpenAgentForm{Op: AgentAdd, Kind: "claude", Name: "work", Dir: "~/.claude-work"})
	m.Update(AgentFormNext{})
	m.Update(AgentFormNext{})
	m.Update(AgentFormNext{})
	v := m.View()
	expect(t, v.Step == AgentStepHosts && strings.Join(optionLabels(v), "|") == "All machines|Choose machines…" && v.Options[0].Selected, "options %q", optionLabels(v))
	expect(t, m.draft.Hosts == "" && !m.draft.AllHosts, "draft %+v", m.draft)
}
