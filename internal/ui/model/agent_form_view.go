package model

import (
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
)

// shortCommit is the abbreviated commit name shown in results.
const shortCommit = 12

// AgentOption is one choice on the kind, directory or settings step.
type AgentOption struct {
	Label, Detail text.Message
	Selected      bool
}

// AgentPreviewView is the loaded preview: the manifest diff lines verbatim,
// the generated commit title and the publication route. Next lists the
// commands that propose a change the publish mode does not write (pr).
type AgentPreviewView struct {
	Dropped  *text.Message
	Title    text.Message
	Commit   text.Message
	Route    text.Message
	Diff     []string
	Next     []text.Message
	Warnings []text.Message
}

// AgentResultView is the outcome of an applied change. Next lists commands
// that finish a manual or unpublished change; Released reports a directory
// handed back by unmanage rules. Published is set when the commit reached
// the remote.
type AgentResultView struct {
	Sync      *text.Message
	Headline  text.Message
	Commit    string
	Next      []text.Message
	Released  []text.Message
	Warnings  []text.Message
	Published bool
}

// AgentFormView is everything a renderer shows for the agent form. Field is
// the text being edited when TextEntry is set; Placeholder shows the current
// directory of an edited instance. Confirm is set on the confirm step and
// dispatches the same intent as Next.
type AgentFormView struct {
	Problem     *text.Message
	Invalid     *text.Message
	Preview     *AgentPreviewView
	Result      *AgentResultView
	Confirm     *Control
	Heading     text.Message
	Title       text.Message
	Explanation text.Message
	Op          AgentOp
	Step        AgentStep
	Field       string
	Placeholder string
	Steps       []AgentStep
	Options     []AgentOption
	Warnings    []text.Message
	Back        Control
	Next        Control
	Cancel      Control
	StepIndex   int
	Open        bool
	TextEntry   bool
	Discarding  bool
	Busy        bool
}

// View projects the current state; a closed form has only Open false.
func (m *AgentForm) View() AgentFormView {
	if !m.open {
		return AgentFormView{}
	}
	step := m.current()
	view := AgentFormView{
		Open: true, Op: m.draft.Op, Step: step, Steps: m.steps, StepIndex: m.step, Heading: m.heading(),
		Title: stepText(step, false), Explanation: stepText(step, true), Invalid: m.invalid(),
		Warnings: m.warnings(), Discarding: m.asking, Busy: m.loading || m.applying, Problem: m.problemText(),
		TextEntry: step == AgentStepName || step == AgentStepDir, Options: m.options(), Placeholder: m.currentDir,
	}
	view.Field = m.field()
	if m.preview != nil && (step == AgentStepPreview || step == AgentStepConfirm) {
		view.Preview = previewView(*m.preview)
	}
	if m.applied != nil {
		view.Result = resultView(*m.applied)
	}
	m.controls(&view)
	return view
}

func (m *AgentForm) field() string {
	switch m.current() {
	case AgentStepName:
		return m.draft.Name
	case AgentStepDir:
		return m.draft.Dir
	case AgentStepKind, AgentStepSettings, AgentStepPreview, AgentStepConfirm, AgentStepResult:
	}
	return ""
}

func (m *AgentForm) heading() text.Message {
	instance := text.Plain(m.draft.Target + "/" + m.draft.Name)
	switch m.draft.Op {
	case AgentEdit:
		return text.New(text.WizardTitleEdit, instance)
	case AgentRemove:
		return text.New(text.WizardTitleRemove, instance)
	case AgentAdd:
	}
	return text.New(text.WizardTitleAdd)
}

func stepText(step AgentStep, explain bool) text.Message {
	titles := map[AgentStep][2]text.Key{
		AgentStepKind: {text.WizardStepKind, text.WizardExplainKind}, AgentStepName: {text.WizardStepName, text.WizardExplainName},
		AgentStepDir: {text.WizardStepDir, text.WizardExplainDir}, AgentStepSettings: {text.WizardStepSettings, text.WizardExplainSettings},
		AgentStepPreview: {text.WizardStepPreview, text.WizardExplainPreview}, AgentStepConfirm: {text.WizardStepConfirm, text.WizardExplainConfirm},
		AgentStepResult: {text.WizardStepResult, text.WizardExplainResult},
	}
	keys := titles[step]
	if explain {
		return text.New(keys[1])
	}
	return text.New(keys[0])
}

func (m *AgentForm) problemText() *text.Message {
	var problem text.Message
	switch {
	case m.stale:
		problem = text.New(text.AgentErrorStale)
	case m.problem == nil:
		return nil
	case m.problem.Detail != "":
		problem = text.New(text.AgentProblem, ProblemText(m.problem), text.Plain(m.problem.Detail))
	default:
		problem = ProblemText(m.problem)
	}
	return &problem
}

func (m *AgentForm) options() []AgentOption {
	var options []AgentOption
	switch m.current() {
	case AgentStepKind:
		for _, kind := range m.kinds() {
			options = append(options, AgentOption{Label: text.Plain(kind), Selected: kind == m.draft.Kind})
		}
	case AgentStepDir:
		for _, c := range m.hints() {
			detail := text.New(text.WizardHint, AgentSourceText(c.Source, c.Variable), AgentPresenceText(c.Presence))
			options = append(options, AgentOption{Label: text.Plain(c.Dir), Detail: detail, Selected: c.Dir == m.draft.Dir})
		}
	case AgentStepSettings:
		options = m.settingsOptions()
	case AgentStepName, AgentStepPreview, AgentStepConfirm, AgentStepResult:
	}
	return options
}

func (m *AgentForm) settingsOptions() []AgentOption {
	unchanged := text.WizardSettingsDefault
	if m.draft.Op == AgentEdit {
		unchanged = text.WizardSettingsKeep
	}
	s := m.draft.Settings
	return []AgentOption{
		{Label: text.New(unchanged), Selected: s == nil},
		{Label: text.New(text.WizardSettingsOn), Selected: s != nil && *s},
		{Label: text.New(text.WizardSettingsOff), Selected: s != nil && !*s},
	}
}

func (m *AgentForm) controls(view *AgentFormView) {
	step := m.current()
	view.Next = m.nextControl(view.Invalid == nil)
	view.Back = Control{Intent: AgentFormBack{}, Label: text.New(text.WizardBack), Enabled: !m.applying && m.step > 0 && step != AgentStepResult}
	view.Cancel = Control{Intent: CancelAgentForm{}, Label: text.New(text.WizardCancel), Enabled: !m.applying}
	if step == AgentStepConfirm {
		confirm := view.Next
		view.Confirm = &confirm
	}
}

func (m *AgentForm) nextControl(valid bool) Control {
	label, enabled := text.WizardNext, !m.loading && !m.applying && valid
	switch m.current() {
	case AgentStepPreview:
		enabled = enabled && m.preview != nil && m.problem == nil
	case AgentStepConfirm:
		label, enabled = text.WizardApply, !m.applying && m.preview != nil
	case AgentStepResult:
		label = text.WizardClose
	case AgentStepKind, AgentStepName, AgentStepDir, AgentStepSettings:
	}
	return Control{Intent: AgentFormNext{}, Label: text.New(label), Enabled: enabled}
}

func previewView(p AgentPreview) *AgentPreviewView {
	view := &AgentPreviewView{Title: text.Plain(p.Title), Commit: text.New(text.WizardCommit, text.Plain(p.Title)), Diff: p.Diff, Next: plain(p.Publication.Next), Warnings: plain(p.Warnings)}
	view.Route = text.New(text.PublishRouteDirect, text.Plain(p.Publication.Branch))
	if p.Publication.Mode != "" && p.Publication.Mode != "direct" {
		view.Route = text.New(text.PublishRouteManual)
	}
	if p.Dropped != "" {
		dropped := text.New(text.WizardDropped, text.Plain(p.Dropped))
		view.Dropped = &dropped
	}
	return view
}

func resultView(a AgentApplied) *AgentResultView {
	p := a.Publication
	commit := p.Commit[:min(len(p.Commit), shortCommit)]
	subject := text.Plain(strings.TrimPrefix(a.Title, "chore(kitbag): "))
	view := &AgentResultView{Commit: p.Commit, Next: plain(p.Next), Warnings: plain(a.Warnings), Published: p.State == PublishPublished}
	switch p.State {
	case PublishPublished:
		view.Headline = text.New(text.AgentResultPublished, subject, text.Plain(commit))
	case PublishNotPublished:
		view.Headline = text.New(text.AgentResultLocal, text.Plain(commit), text.Plain(p.Reason))
	case PublishManual:
		view.Headline = text.New(text.AgentResultManual, text.Plain(p.Reason))
	case PublishConflict, PublishDryRun:
		view.Headline = text.New(text.AgentResultNotApplied, text.Plain(p.Reason))
	}
	if r := a.Released; r != nil {
		view.Released = append([]text.Message{text.New(text.AgentResultReleased, text.Plain(r.Dir))}, plain(r.Changed)...)
		view.Warnings = append(view.Warnings, plain(r.Warnings)...)
	}
	if s := a.Sync; s != nil {
		sync := text.New(text.AgentResultSync, ResultText(s.ExitCode), number(s.Changed))
		view.Sync = &sync
		view.Warnings = append(view.Warnings, plain(s.Warnings)...)
	}
	return view
}
