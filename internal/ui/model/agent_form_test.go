package model

import (
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
)

func openForm(t *testing.T, open OpenAgentForm) (*AgentForm, []Cmd) {
	t.Helper()
	m := NewAgentForm()
	m.Update(AgentsLoaded{Seq: 1, Snapshot: agentFixture()})
	cmds := m.Update(open)
	expect(t, m.Open(), "form not open")
	return m, cmds
}

// invalidText renders the current step's validation message, or "".
func invalidText(m *AgentForm) string {
	if invalid := m.View().Invalid; invalid != nil {
		return render(*invalid)
	}
	return ""
}

func formTexts(v AgentFormView) []text.Message {
	messages := []text.Message{v.Heading, v.Title, v.Explanation, v.Back.Label, v.Next.Label, v.Cancel.Label}
	messages = append(messages, v.Warnings...)
	for _, o := range v.Options {
		messages = append(messages, o.Label, o.Detail)
	}
	for _, m := range []*text.Message{v.Problem, v.Invalid} {
		if m != nil {
			messages = append(messages, *m)
		}
	}
	if p := v.Preview; p != nil {
		messages = append(append(messages, p.Title, p.Commit, p.Route), p.Warnings...)
		if p.Dropped != nil {
			messages = append(messages, *p.Dropped)
		}
	}
	if r := v.Result; r != nil {
		messages = append(append(append(append(messages, r.Headline), r.Next...), r.Released...), r.Warnings...)
		if r.Sync != nil {
			messages = append(messages, *r.Sync)
		}
	}
	return messages
}

// checkForm checks every text of the view and that no key falls through.
func checkForm(t *testing.T, m *AgentForm) {
	t.Helper()
	messages := formTexts(m.View())
	checkText(t, messages...)
	for _, message := range messages {
		rendered := render(message)
		expect(t, !strings.Contains(rendered, "wizard.") && !strings.Contains(rendered, "agent.") && !strings.Contains(rendered, "publish."), "raw key %q", rendered)
	}
}

// toSettings drives an add draft to the settings step.
func toSettings(t *testing.T) *AgentForm {
	t.Helper()
	m, _ := openForm(t, OpenAgentForm{Op: AgentAdd})
	m.Update(ChooseAgentOption{Index: 0})
	m.Update(AgentFormNext{})
	m.Update(EditAgentField{Value: "work"})
	m.Update(AgentFormNext{})
	m.Update(ChooseAgentOption{Index: 0})
	m.Update(AgentFormNext{})
	expect(t, m.View().Step == AgentStepSettings, "step=%s", m.View().Step)
	return m
}

func TestAgentFormAddPath(t *testing.T) {
	t.Parallel()
	m := toSettings(t)
	checkForm(t, m)
	m.Update(ChooseAgentOption{Index: settingsOff})
	preview := as[PreviewAgent](t, only(t, m.Update(AgentFormNext{})))
	c := preview.Change
	expect(t, c.Op == AgentAdd && c.Kind == "claude" && c.Name == "work" && c.Dir == "/home/u/.claude-work" && c.Settings != nil && !*c.Settings, "change=%+v", c)
	expect(t, m.View().Busy && !m.View().Next.Enabled, "previewing must disable next")
	m.Update(AgentPreviewed{Seq: preview.Seq, Preview: AgentPreview{Revision: "r1", Title: "chore(kitbag): add", Diff: []string{"+x"}, Dropped: "~/.old", Publication: AgentPublication{Mode: "direct", Branch: "main"}}})
	expect(t, render(m.View().Preview.Route) == "Publication route: direct push to main.", "route")
	checkForm(t, m)
	checkConfirm(t, m)
}

func checkConfirm(t *testing.T, m *AgentForm) {
	t.Helper()
	m.Update(AgentFormNext{})
	view := m.View()
	expect(t, view.Step == AgentStepConfirm && view.Confirm != nil && render(view.Confirm.Label) == "Apply", "confirm %+v", view.Confirm)
	apply := as[ApplyAgent](t, only(t, m.Update(AgentFormNext{})))
	expect(t, apply.Revision == "r1" && apply.Change.Name == "work", "apply=%+v", apply)
	expect(t, len(m.Update(AgentFormBack{})) == 0 && m.View().Step == AgentStepConfirm, "intents ignored while applying")
	checkPublished(t, m)
}

func checkPublished(t *testing.T, m *AgentForm) {
	t.Helper()
	applied := AgentApplied{AgentPreview: AgentPreview{Title: "chore(kitbag): add claude instance work", Publication: AgentPublication{State: PublishPublished, Commit: "0123456789abcdef"}}, Sync: &AgentSyncReport{Changed: 2}, Released: &AgentRelease{Dir: "~/.old", Changed: []string{"discard /x"}}}
	m.Update(AgentChangeApplied{Applied: applied})
	expect(t, render(m.View().Result.Headline) == "Published add claude instance work as 0123456789ab.", "headline=%q", render(m.View().Result.Headline))
	expect(t, render(*m.View().Result.Sync) == "Sync succeeded, 2 changed", "sync")
	checkForm(t, m)
	m.Update(AgentFormNext{})
	expect(t, !m.Open() && !m.View().Open, "result next closes")
}

func TestAgentFormValidation(t *testing.T) {
	t.Parallel()
	m, _ := openForm(t, OpenAgentForm{Op: AgentAdd})
	expect(t, invalidText(m) == "Choose an agent.", "kind=%q", invalidText(m))
	m.Update(ChooseAgentOption{Index: 2})
	expect(t, strings.HasPrefix(invalidText(m), "Generic agents"), "generic=%q", invalidText(m))
	expect(t, len(m.Update(AgentFormNext{})) == 0 && m.View().Step == AgentStepKind, "invalid kind must not advance")
	m.Update(ChooseAgentOption{Index: 0})
	m.Update(AgentFormNext{})
	for _, name := range [][2]string{
		{"Bad Name", "Use up to 64"}, {"main", "Target claude already has an instance named main."}, {"work", ""},
	} {
		value, want := name[0], name[1]
		m.Update(EditAgentField{Value: value})
		expect(t, strings.HasPrefix(invalidText(m), want) && (want == "") == (invalidText(m) == ""), "name %q: %q", value, invalidText(m))
	}
	m.Update(AgentFormNext{})
	expect(t, m.View().Step == AgentStepDir, "valid name advances")
	for value, want := range map[string]string{
		"relative": "Use a directory", "~/../x": "Use a directory", "": "Use a directory",
		"/srv/claude": "/srv/claude is outside", "~/.CLAUDE_2": "~/.CLAUDE_2 is already declared as claude/second.",
		"/home/u/.claude": "/home/u/.claude is already declared as claude/main.", "~/.claude-new": "",
	} {
		m.Update(EditAgentField{Value: value})
		expect(t, strings.HasPrefix(invalidText(m), want) && (want == "") == (invalidText(m) == ""), "dir %q: %q", value, invalidText(m))
		checkForm(t, m)
	}
}

func TestAgentFormHintsAndMissingWarning(t *testing.T) {
	t.Parallel()
	m, _ := openForm(t, OpenAgentForm{Op: AgentAdd, Kind: "codex"})
	m.Update(AgentFormNext{})
	m.Update(EditAgentField{Value: "alt"})
	m.Update(AgentFormNext{})
	options := m.View().Options
	expect(t, len(options) == 1 && render(options[0].Detail) == "found by name, missing", "hints=%+v", options)
	m.Update(ChooseAgentOption{Index: 0})
	view := m.View()
	expect(t, view.Field == "/home/u/.codex-alt" && view.Options[0].Selected && view.Invalid == nil, "hint chosen %+v", view)
	expect(t, len(view.Warnings) == 1 && strings.HasSuffix(render(view.Warnings[0]), "until the agent creates it."), "warning")
	m.Update(ChooseAgentOption{Index: 5})
	expect(t, m.View().Field == "/home/u/.codex-alt", "out of range choice ignored")
	checkForm(t, m)
}

func TestAgentFormStalePreviews(t *testing.T) {
	t.Parallel()
	m := toSettings(t)
	first := as[PreviewAgent](t, only(t, m.Update(AgentFormNext{})))
	m.Update(AgentFormBack{})
	m.Update(ChooseAgentOption{Index: settingsOn})
	second := as[PreviewAgent](t, only(t, m.Update(AgentFormNext{})))
	expect(t, second.Seq > first.Seq && *second.Change.Settings, "new preview for the changed draft")
	m.Update(AgentPreviewed{Seq: first.Seq, Preview: AgentPreview{Revision: "old"}})
	expect(t, m.View().Preview == nil, "stale preview applied")
	m.Update(AgentPreviewed{Seq: second.Seq, Err: &Error{Kind: ErrInvalid, Detail: "target claude has no settings fragment"}})
	expect(t, render(*m.View().Problem) == "The configuration or request is invalid: target claude has no settings fragment", "problem")
	expect(t, len(m.Update(AgentFormNext{})) == 0 && m.View().Step == AgentStepPreview, "failed preview must not advance")
	m.Update(AgentFormBack{})
	expect(t, m.View().Step == AgentStepSettings && m.View().Problem == nil, "back clears the problem")
}

func confirmed(t *testing.T) *AgentForm {
	t.Helper()
	m, cmds := openForm(t, OpenAgentForm{Op: AgentRemove, Kind: "claude", Target: "claude", Name: "second", Dir: "/home/u/.claude_2"})
	preview := as[PreviewAgent](t, only(t, cmds))
	expect(t, preview.Change == AgentChange{Op: AgentRemove, Kind: "claude", Target: "claude", Name: "second"}, "remove preview=%+v", preview.Change)
	m.Update(AgentPreviewed{Seq: preview.Seq, Preview: AgentPreview{Revision: "r1"}})
	m.Update(AgentFormNext{})
	as[ApplyAgent](t, only(t, m.Update(AgentFormNext{})))
	return m
}

func TestAgentFormApplyFailures(t *testing.T) {
	t.Parallel()
	m := confirmed(t)
	expect(t, render(m.View().Heading) == "Remove claude/second", "heading")
	again := as[PreviewAgent](t, only(t, m.Update(AgentChangeApplied{Err: &Error{Kind: ErrConflict}})))
	view := m.View()
	expect(t, view.Step == AgentStepPreview && render(*view.Problem) == "kitbag.toml changed since the preview. Review the new preview.", "conflict %+v", view.Problem)
	m.Update(AgentPreviewed{Seq: again.Seq, Preview: AgentPreview{Revision: "r2"}})
	m.Update(AgentFormNext{})
	expect(t, as[ApplyAgent](t, only(t, m.Update(AgentFormNext{}))).Revision == "r2", "apply the new revision")
	m.Update(AgentChangeApplied{Err: &Error{Kind: ErrOperation}})
	expect(t, m.View().Step == AgentStepConfirm && render(*m.View().Problem) == "The operation failed", "other failure stays")
	checkForm(t, m)
	expect(t, len(m.Update(AgentChangeApplied{})) == 0 && m.View().Result == nil, "unexpected result ignored")
}

func TestAgentFormResults(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		state PublishState
		want  string
	}{
		{PublishNotPublished, "Committed abc on this machine but not published: rejected"},
		{PublishManual, "Not written: rejected"},
		{PublishConflict, "Not applied: rejected"},
	} {
		state, want := c.state, c.want
		m := confirmed(t)
		m.Update(AgentChangeApplied{Applied: AgentApplied{AgentPreview: AgentPreview{Publication: AgentPublication{State: state, Commit: "abc", Reason: "rejected", Next: []string{"git push"}}}}})
		view := m.View()
		expect(t, render(view.Result.Headline) == want && len(view.Result.Next) == 1 && !view.Back.Enabled, "%s: %q", state, render(view.Result.Headline))
		checkForm(t, m)
		m.Update(CancelAgentForm{})
		expect(t, !m.Open(), "cancel closes the result")
	}
}

func TestAgentFormCancelAndBack(t *testing.T) {
	t.Parallel()
	m, _ := openForm(t, OpenAgentForm{Op: AgentAdd})
	m.Update(CancelAgentForm{})
	expect(t, !m.Open(), "untouched form closes at once")
	m, _ = openForm(t, OpenAgentForm{Op: AgentAdd})
	m.Update(ChooseAgentOption{Index: 0})
	m.Update(CancelAgentForm{})
	expect(t, m.Open() && m.View().Discarding, "a draft asks first")
	checkForm(t, m)
	m.Update(ChooseAgentOption{Index: 1})
	expect(t, !m.View().Discarding, "another intent clears the question")
	m.Update(AgentFormBack{})
	m.Update(AgentFormBack{})
	expect(t, !m.Open(), "back on the first step cancels")
	expect(t, len(m.Update(AgentFormNext{})) == 0 && len(m.Update(AgentPreviewed{Seq: 99})) == 0, "closed form ignores intents")
	expect(t, len(m.Update(OpenAgentForm{Op: "rename"})) == 0 && !m.Open(), "unknown op")
}

func TestAgentFormEdit(t *testing.T) {
	t.Parallel()
	m, cmds := openForm(t, OpenAgentForm{Op: AgentEdit, Kind: "claude", Target: "claude", Name: "second", Dir: "/home/u/.claude_2"})
	view := m.View()
	expect(t, len(cmds) == 0 && view.Step == AgentStepDir && view.TextEntry && view.Placeholder == "/home/u/.claude_2" && view.Invalid == nil, "edit start %+v", view)
	expect(t, len(view.Steps) == 5 && render(view.Heading) == "Edit claude/second", "edit steps")
	m.Update(EditAgentField{Value: "~/.claude_2"})
	expect(t, invalidText(m) == "", "own directory is not a duplicate")
	m.Update(EditAgentField{Value: "~/.claude"})
	expect(t, invalidText(m) == "~/.claude is already declared as claude/main.", "dup=%q", invalidText(m))
	m.Update(EditAgentField{Value: ""})
	m.Update(AgentFormNext{})
	expect(t, render(m.View().Options[0].Label) == "Keep the current setting", "keep option")
	m.Update(ChooseAgentOption{Index: settingsOn})
	preview := as[PreviewAgent](t, only(t, m.Update(AgentFormNext{})))
	expect(t, preview.Change.Op == AgentEdit && preview.Change.Dir == "" && *preview.Change.Settings, "edit change=%+v", preview.Change)
	checkForm(t, m)
}

func TestAgentFormAdoptionSkipsKnownStepsAndBack(t *testing.T) {
	t.Parallel()
	m, _ := openForm(t, OpenAgentForm{Op: AgentAdd, Adopt: true, Kind: "claude", Dir: "/home/u/.claude-work"})
	expect(t, m.View().Step == AgentStepName && len(m.View().Steps) == 5, "adoption steps %+v", m.View().Steps)
	m.Update(EditAgentField{Value: "work"})
	m.Update(AgentFormNext{})
	expect(t, m.View().Step == AgentStepSettings, "known directory was not skipped")
	m.Update(AgentFormBack{})
	expect(t, m.View().Step == AgentStepName && m.View().Field == "work", "back lost name")
	m.Update(AgentFormNext{})
	cmd := as[PreviewAgent](t, only(t, m.Update(AgentFormNext{})))
	expect(t, cmd.Change.Kind == "claude" && cmd.Change.Dir == "/home/u/.claude-work", "lost candidate %+v", cmd.Change)
}

func TestAgentFormAdoptionDoesNotSkipInvalidDirectory(t *testing.T) {
	t.Parallel()
	m, _ := openForm(t, OpenAgentForm{Op: AgentAdd, Adopt: true, Kind: "claude", Dir: "/srv/claude"})
	expect(t, m.View().Step == AgentStepKind && len(m.View().Steps) == 7, "invalid directory skipped validation")
}
