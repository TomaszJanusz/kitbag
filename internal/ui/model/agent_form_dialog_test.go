package model

import (
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
)

// dialogTexts collects every message of a window for catalog checks.
func dialogTexts(d DialogView) []text.Message {
	messages := make([]text.Message, 0, 2+2*len(d.Fields)+len(d.Body)+2*len(d.Options)+len(d.Buttons))
	messages = append(messages, d.Title, d.Message)
	for _, f := range d.Fields {
		messages = append(messages, f.Label, f.Hint)
	}
	for _, line := range d.Body {
		messages = append(messages, line.Text)
	}
	for _, o := range d.Options {
		messages = append(messages, o.Label, o.Detail)
	}
	for _, b := range d.Buttons {
		messages = append(messages, b.Label)
	}
	return messages
}

func bodyText(d *DialogView) string {
	lines := make([]string, 0, len(d.Body))
	for _, line := range d.Body {
		lines = append(lines, render(line.Text))
	}
	return strings.Join(lines, "\n")
}

func checkDialog(t *testing.T, d *DialogView) {
	t.Helper()
	expect(t, d != nil && d.ID == AgentFormDialog && d.Wide && len(d.Buttons) > 0, "dialog %+v", d)
	_, next := d.Buttons[len(d.Buttons)-1].Intent.(AgentFormNext)
	_, cancel := d.Cancel.(CancelAgentForm)
	expect(t, next && cancel, "buttons %+v", d.Buttons)
	checkText(t, dialogTexts(*d)...)
}

func TestAgentFormDialogDraftSteps(t *testing.T) {
	t.Parallel()
	m, _ := openForm(t, OpenAgentForm{Op: AgentAdd, Kind: "claude"})
	d := m.Dialog()
	checkDialog(t, d)
	expect(t, render(d.Title) == "Add an agent instance: Agent" && len(d.Fields) == 0, "kind %+v", d)
	expect(t, d.Step == 1 && d.Steps == 7, "steps %d/%d", d.Step, d.Steps)
	expect(t, len(d.Options) == 3 && d.Options[0].Selected && d.InitialFocus == 0, "kind options %+v", d.Options)
	expect(t, d.Options[1].Intent == ChooseAgentOption{Index: 1}, "option intent %+v", d.Options[1])
	m.Update(AgentFormNext{})
	d = m.Dialog()
	expect(t, len(d.Fields) == 1 && d.Fields[0].ID == "name" && d.Fields[0].Enabled && d.InitialFocus == 0, "name %+v", d.Fields)
	m.Update(EditDialogField{Dialog: "other", Value: "ignored"})
	expect(t, m.View().Field == "", "another window edited the draft")
	m.Update(EditDialogField{Dialog: AgentFormDialog, Value: "main"})
	d = m.Dialog()
	expect(t, d.Fields[0].Value == "main" && d.Body[0].Tone == ToneProblem && !d.Buttons[2].Enabled, "invalid name %+v", d.Body)
}

func TestAgentFormDialogDirAndSettings(t *testing.T) {
	t.Parallel()
	m, _ := openForm(t, OpenAgentForm{Op: AgentAdd, Kind: "claude", Name: "work"})
	m.Update(AgentFormNext{})
	m.Update(AgentFormNext{})
	d := m.Dialog()
	expect(t, strings.HasSuffix(bodyText(d), "Directories found on this machine:") && len(d.Options) == 2, "dir %q", bodyText(d))
	m.Update(EditDialogField{Dialog: AgentFormDialog, Value: "~/.claude_2"})
	expect(t, strings.Contains(bodyText(m.Dialog()), "already declared as claude/second"), "duplicate")
	m.Update(ChooseAgentOption{Index: 0})
	m.Update(AgentFormNext{})
	d = m.Dialog()
	expect(t, d.Options[0].Selected && d.InitialFocus == 5 && render(d.Options[0].Label) == "Default (merge shared settings)", "settings %+v", d.Options)
	checkDialog(t, d)
}

func TestAgentFormDialogPreviewToResult(t *testing.T) {
	t.Parallel()
	m := toSettings(t)
	preview := as[PreviewAgent](t, only(t, m.Update(AgentFormNext{})))
	d := m.Dialog()
	expect(t, bodyText(d) == "Preparing the preview…" && !d.Buttons[2].Enabled && d.InitialFocus == 2, "loading %q", bodyText(d))
	m.Update(AgentPreviewed{Seq: preview.Seq, Preview: AgentPreview{
		Revision: "r1", Title: "chore(kitbag): add", Diff: []string{"+a", "-b"}, Dropped: "~/.old", Warnings: []string{"careful"},
		Publication: AgentPublication{Mode: "pr", Next: []string{"git switch -c x"}},
	}})
	d = m.Dialog()
	checkDialog(t, d)
	body := bodyText(d)
	expect(t, strings.Contains(body, "Not written: publish mode is pull request") && strings.Contains(body, "To propose it:\ngit switch -c x"), "preview %q", body)
	expect(t, strings.HasSuffix(body, "Changes to kitbag.toml:\n+a\n-b") && d.InitialFocus == 2 && d.Buttons[2].Enabled, "preview diff %q", body)
	m.Update(AgentFormNext{})
	d = m.Dialog()
	expect(t, d.InitialFocus == 2 && render(d.Buttons[2].Label) == "Apply" && !strings.Contains(bodyText(d), "+a"), "confirm %+v", d)
}

func TestAgentFormDialogApplyAndResult(t *testing.T) {
	t.Parallel()
	m := toSettings(t)
	m.Update(AgentPreviewed{Seq: as[PreviewAgent](t, only(t, m.Update(AgentFormNext{}))).Seq, Preview: AgentPreview{Revision: "r1"}})
	m.Update(AgentFormNext{})
	as[ApplyAgent](t, only(t, m.Update(AgentFormNext{})))
	d := m.Dialog()
	expect(t, strings.Contains(bodyText(d), "Applying and publishing…") && !d.Buttons[0].Enabled && !d.Buttons[1].Enabled && !d.Buttons[2].Enabled, "applying %+v", d.Buttons)
	expect(t, m.DiscardDialog() == nil, "no question while applying")
	m.Update(AgentChangeApplied{Applied: AgentApplied{AgentPreview: AgentPreview{Title: "chore(kitbag): add", Publication: AgentPublication{State: PublishPublished, Commit: "abc"}}, Sync: &AgentSyncReport{Changed: 1}}})
	d = m.Dialog()
	checkDialog(t, d)
	expect(t, d.Body[0].Tone == ToneSuccess && strings.HasSuffix(bodyText(d), "The agents list was reloaded."), "result %q", bodyText(d))
	expect(t, len(d.Buttons) == 1 && render(d.Buttons[0].Label) == "Close" && d.InitialFocus == 0, "close %+v", d.Buttons)
	m.Update(AgentFormNext{})
	expect(t, m.Dialog() == nil, "closed form has a window")
}

func TestAgentFormDialogEditRemoveAndResults(t *testing.T) {
	t.Parallel()
	m, _ := openForm(t, OpenAgentForm{Op: AgentEdit, Kind: "claude", Target: "claude", Name: "second", Dir: "/home/u/.claude_2"})
	d := m.Dialog()
	expect(t, render(d.Fields[0].Hint) == "Now /home/u/.claude_2; leave empty to keep it." && d.Steps == 5, "edit %+v", d.Fields)
	m, cmds := openForm(t, OpenAgentForm{Op: AgentRemove, Kind: "claude", Target: "claude", Name: "second"})
	m.Update(AgentPreviewed{Seq: as[PreviewAgent](t, only(t, cmds)).Seq, Err: &Error{Kind: ErrInvalid, Detail: "refused"}})
	d = m.Dialog()
	expect(t, d.Body[0].Tone == ToneProblem && strings.HasSuffix(render(d.Body[0].Text), ": refused") && len(d.Fields)+len(d.Options) == 0, "refused %+v", d.Body)
	m = confirmed(t)
	m.Update(AgentChangeApplied{Applied: AgentApplied{
		AgentPreview: AgentPreview{Warnings: []string{"w"}, Publication: AgentPublication{State: PublishNotPublished, Commit: "abc", Reason: "rejected", Next: []string{"git push"}}},
		Released:     &AgentRelease{Dir: "~/.claude_2", Changed: []string{"discard x"}},
	}})
	d = m.Dialog()
	expect(t, d.Body[0].Tone == ToneWarning && strings.Contains(bodyText(d), "To propose it:\ngit push\nHanded back ~/.claude_2\ndiscard x\nw"), "result %q", bodyText(d))
	checkDialog(t, d)
}

func TestAgentFormDiscardDialog(t *testing.T) {
	t.Parallel()
	m, _ := openForm(t, OpenAgentForm{Op: AgentAdd})
	expect(t, m.DiscardDialog() == nil, "no question without a draft")
	m.Update(ChooseAgentOption{Index: 0})
	m.Update(CancelAgentForm{})
	d := m.DiscardDialog()
	expect(t, d != nil && d.ID == AgentDiscardDialog && d.InitialFocus == 1 && d.CloseOnAction, "discard %+v", d)
	checkText(t, dialogTexts(*d)...)
	m.Update(d.Cancel)
	expect(t, m.Open() && m.DiscardDialog() == nil && m.View().Field == "" && m.View().Options[0].Selected, "resume keeps the draft")
	m.Update(CancelAgentForm{})
	m.Update(m.DiscardDialog().Buttons[0].Intent)
	expect(t, !m.Open() && m.DiscardDialog() == nil, "yes discards")
	m.Update(ResumeAgentForm{})
	expect(t, !m.Open(), "resume does not reopen")
}

func TestAgentDetailSaysWhatSyncDoes(t *testing.T) {
	t.Parallel()
	m := loadedAgents(t)
	want := []string{
		"installs the shared configuration here", "skips it on this host", "leaves it alone until it is added",
		"leaves it alone; it cannot be declared", "skips it on this host", "leaves it alone until it is added",
	}
	for i, sync := range want {
		m.Update(SelectAgent{Index: i})
		rows := m.View().Detail.Rows
		last := rows[len(rows)-1]
		expect(t, render(last.Label) == "Next sync" && render(last.Value) == sync, "row %d: %q", i, render(last.Value))
	}
	snapshot := agentFixture()
	snapshot.Instances[0].Presence, snapshot.Instances[1].ActiveHere = PresenceSymlink, true
	m.Update(AgentsLoaded{Seq: as[LoadAgents](t, only(t, m.Update(Refresh{}))).Seq, Snapshot: snapshot})
	for i, sync := range []string{"warns and skips it", "warns and skips it until the directory exists"} {
		m.Update(SelectAgent{Index: i})
		rows := m.View().Detail.Rows
		expect(t, render(rows[len(rows)-1].Value) == sync, "row %d: %q", i, render(rows[len(rows)-1].Value))
	}
}

// Every step has a deliberate initial focus. Input steps remain ready for
// editing; after the draft is complete Enter always advances the primary action.
func TestAgentFormInitialFocusEveryStep(t *testing.T) {
	t.Parallel()
	m, _ := openForm(t, OpenAgentForm{Op: AgentAdd, Kind: "claude", Name: "work", Dir: "~/.claude-work"})
	for _, step := range []AgentStep{AgentStepKind, AgentStepName, AgentStepDir, AgentStepSettings, AgentStepPreview, AgentStepConfirm, AgentStepResult} {
		d := m.Dialog()
		expect(t, m.current() == step, "step %s, want %s", m.current(), step)
		assertStepFocus(t, d, step)
		cmds := m.Update(AgentFormNext{})
		switch step {
		case AgentStepSettings:
			m.Update(AgentPreviewed{Seq: as[PreviewAgent](t, only(t, cmds)).Seq, Preview: AgentPreview{Revision: "r1"}})
		case AgentStepConfirm:
			m.Update(AgentChangeApplied{Applied: AgentApplied{}})
		case AgentStepResult:
			expect(t, len(d.Buttons) == 1 && render(d.Buttons[0].Label) == "Close", "result buttons %+v", d.Buttons)
		case AgentStepKind, AgentStepName, AgentStepDir, AgentStepPreview:
		}
	}
}

func assertStepFocus(t *testing.T, d *DialogView, step AgentStep) {
	t.Helper()
	want := 0
	switch step {
	case AgentStepSettings, AgentStepPreview, AgentStepConfirm, AgentStepResult:
		want = len(d.Fields) + len(d.Options) + len(d.Buttons) - 1
	case AgentStepKind, AgentStepName, AgentStepDir:
	}
	expect(t, d.InitialFocus == want, "step %s: focus %d, want %d", step, d.InitialFocus, want)
	if step == AgentStepKind || step == AgentStepResult {
		for _, button := range d.Buttons {
			_, back := button.Intent.(AgentFormBack)
			expect(t, !back, "unusable Back on %s", step)
		}
	}
}
