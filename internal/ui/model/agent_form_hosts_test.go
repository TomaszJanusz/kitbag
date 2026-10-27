package model

import (
	"strings"
	"testing"
)

// openSchema2 opens the form on a schema-2 manifest naming hosts mac and ci.
func openSchema2(t *testing.T, open OpenAgentForm) *AgentForm {
	t.Helper()
	snapshot := agentFixture()
	snapshot.Schema, snapshot.KnownHosts = 2, []string{"ci", "mac"}
	m := NewAgentForm()
	m.Update(AgentsLoaded{Seq: 1, Snapshot: snapshot})
	m.Update(open)
	expect(t, m.Open(), "form not open")
	return m
}

// toHosts drives a schema-2 add to the hosts step.
func toHosts(t *testing.T) *AgentForm {
	t.Helper()
	m := openSchema2(t, OpenAgentForm{Op: AgentAdd, Kind: "claude", Name: "work", Dir: "~/.claude-work"})
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

// TestAgentFormHostsStep shows the choices on schema 2: this machine only by
// default, or all machines.
func TestAgentFormHostsStep(t *testing.T) {
	t.Parallel()
	m := toHosts(t)
	v := m.View()
	expect(t, v.Step == AgentStepHosts && len(v.Steps) == 8 && render(v.Title) == "Machines" && !v.TextEntry, "hosts step %+v", v)
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
	expect(t, len(d.Fields) == 1 && d.Fields[0].ID == "hosts" && render(d.Fields[0].Hint) == "Host names separated by commas, for example mac, runner02.", "field %+v", d.Fields)
	for value, want := range map[string]string{
		"mac, my mac": "my mac is not a valid host name: use letters, digits, dots, underscores or hyphens.",
		"mac,mac":     "mac is listed twice.",
		" mac , ci ":  "",
	} {
		m.Update(EditDialogField{Dialog: AgentFormDialog, Value: value})
		expect(t, invalidText(m) == want, "%q: invalid %q, want %q", value, invalidText(m), want)
	}
	expect(t, m.draft.Hosts == "mac,ci" && len(m.View().Warnings) == 0, "typed draft %+v", m.draft)
	m.Update(EditAgentField{Value: "mac, macbok"})
	warnings := m.View().Warnings
	expect(t, len(warnings) == 1 && render(warnings[0]) == "macbok is neither this machine nor named in kitbag.toml; check its spelling.", "unknown host warning %+v", warnings)
	checkForm(t, m)

	m.Update(AgentFormNext{})
	expect(t, m.View().Step == AgentStepSettings, "settings follows hosts")
	preview := as[PreviewAgent](t, only(t, m.Update(AgentFormNext{})))
	expect(t, preview.Change.Hosts == "mac,macbok", "preview change %+v", preview.Change)
}

// TestAgentFormHostsEdit keeps the current machines by default; all
// machines removes the restriction.
func TestAgentFormHostsEdit(t *testing.T) {
	t.Parallel()
	m := openSchema2(t, OpenAgentForm{Op: AgentEdit, Kind: "claude", Target: "claude", Name: "second", Dir: "/home/u/.claude_2"})
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

// TestAgentFormSchema1HasNoHostsStep keeps the schema-1 wizard unchanged:
// nothing about hosts is sent, which the server would refuse.
func TestAgentFormSchema1HasNoHostsStep(t *testing.T) {
	t.Parallel()
	for _, op := range []AgentOp{AgentAdd, AgentEdit} {
		m, _ := openForm(t, OpenAgentForm{Op: op, Kind: "claude", Target: "claude", Name: "second"})
		for _, step := range m.View().Steps {
			expect(t, step != AgentStepHosts, "%s on schema 1 has a hosts step", op)
		}
		expect(t, m.draft.Hosts == "" && !m.draft.AllHosts, "schema-1 draft %+v", m.draft)
	}
}
