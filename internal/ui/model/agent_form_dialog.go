package model

import "github.com/TomaszJanusz/kitbag/internal/ui/model/text"

// Window IDs of the agent form and its discard question.
const (
	AgentFormDialog    = "agent-form"
	AgentDiscardDialog = "agent-discard"
)

// Dialog projects the open form into the shared form window: one step per
// window state under a stable ID, the text entry as a field, choices as
// options and the preview or result as body lines. Cancel, Back and Next
// (Apply, Close) are the buttons, Next last. A closed form returns nil.
func (m *AgentForm) Dialog() *DialogView {
	if !m.open {
		return nil
	}
	v := m.View()
	var fields []DialogField
	if v.TextEntry {
		field := DialogField{ID: string(v.Step), Value: v.Field, Label: v.Title, Enabled: !v.Busy}
		if v.Step == AgentStepDir && v.Placeholder != "" {
			field.Hint = text.New(text.WizardCurrentDir, text.Plain(v.Placeholder))
		}
		fields = []DialogField{field}
	}
	dialog := FormDialog(AgentFormDialog, text.New(text.WizardWindowTitle, v.Heading, v.Title), v.StepIndex+1, len(v.Steps), fields, v.Back, v.Next, v.Cancel.Intent)
	dialog.Message, dialog.Wide = v.Explanation, true
	dialog.Buttons = []Control{v.Cancel}
	if v.StepIndex > 0 {
		dialog.Buttons = append(dialog.Buttons, v.Back)
	}
	dialog.Buttons = append(dialog.Buttons, v.Next)
	if v.Step == AgentStepResult {
		dialog.Buttons = []Control{v.Next}
	}
	dialog.Body = formBody(v)
	for i, option := range v.Options {
		dialog.Options = append(dialog.Options, DialogOption{Intent: ChooseAgentOption{Index: i}, Label: option.Label, Detail: option.Detail, Selected: option.Selected})
	}
	dialog.InitialFocus = initialFocus(v, len(fields), len(dialog.Options), len(dialog.Buttons))
	if v.Busy && v.Step == AgentStepConfirm {
		dialog.Buttons = nil
	}
	return &dialog
}

// initialFocus offers the primary action on every ready, valid step. An
// invalid draft starts at its field or options so the user can complete it.
func initialFocus(v AgentFormView, fields, options, buttons int) int {
	if v.Next.Enabled || fields+options == 0 {
		return fields + options + buttons - 1
	}
	return 0
}

func formBody(v AgentFormView) []DialogLine {
	var body []DialogLine
	add := func(tone Tone, messages ...text.Message) {
		for _, message := range messages {
			body = append(body, DialogLine{Text: message, Tone: tone})
		}
	}
	if v.Invalid != nil {
		add(ToneProblem, *v.Invalid)
	}
	add(ToneWarning, v.Warnings...)
	if v.Problem != nil {
		add(ToneProblem, *v.Problem)
	}
	if v.Busy {
		busy := text.WizardPreviewing
		if v.Step == AgentStepConfirm {
			busy = text.WizardApplying
		}
		add(ToneHint, text.New(busy))
	}
	if p := v.Preview; p != nil {
		body = append(body, previewBody(*p, v.Step == AgentStepPreview)...)
	}
	if r := v.Result; r != nil {
		body = append(body, resultBody(*r)...)
	}
	if v.Step == AgentStepDir && len(v.Options) > 0 {
		add(ToneHeading, text.New(text.WizardHintsHeading))
	}
	return body
}

// previewBody shows the commit and route; the preview step adds the diff.
func previewBody(p AgentPreviewView, diff bool) []DialogLine {
	body := []DialogLine{{Text: p.Commit}, {Text: p.Route}}
	if len(p.Next) > 0 {
		body = append(body, DialogLine{Text: text.New(text.WizardNextCommands), Tone: ToneHeading})
		body = append(body, lines(ToneCode, p.Next)...)
	}
	if p.Dropped != nil {
		body = append(body, DialogLine{Text: *p.Dropped, Tone: ToneWarning})
	}
	body = append(body, lines(ToneWarning, p.Warnings)...)
	if diff && len(p.Diff) > 0 {
		body = append(body, DialogLine{}, DialogLine{Text: text.New(text.WizardDiffHeading), Tone: ToneHeading})
		body = append(body, lines(ToneCode, plain(p.Diff))...)
	}
	return body
}

func resultBody(r AgentResultView) []DialogLine {
	tone := ToneWarning
	if r.Published {
		tone = ToneSuccess
	}
	body := []DialogLine{{Text: r.Headline, Tone: tone}}
	if len(r.Next) > 0 {
		body = append(body, DialogLine{Text: text.New(text.WizardNextCommands), Tone: ToneHeading})
		body = append(body, lines(ToneCode, r.Next)...)
	}
	body = append(body, lines(ToneNormal, r.Released)...)
	if r.Sync != nil {
		body = append(body, DialogLine{Text: *r.Sync})
	}
	body = append(body, lines(ToneWarning, r.Warnings)...)
	return append(body, DialogLine{Text: text.New(text.WizardRefreshed), Tone: ToneHint})
}

func lines(tone Tone, messages []text.Message) []DialogLine {
	result := make([]DialogLine, 0, len(messages))
	for _, message := range messages {
		result = append(result, DialogLine{Text: message, Tone: tone})
	}
	return result
}

// DiscardDialog asks before a typed draft is dropped; nil unless asking.
// Yes repeats the cancel (which then discards), No resumes the draft.
func (m *AgentForm) DiscardDialog() *DialogView {
	if !m.open || !m.asking {
		return nil
	}
	dialog := ConfirmDialog(AgentDiscardDialog, text.New(text.WizardDiscardTitle), text.New(text.WizardDiscardConfirm), CancelAgentForm{}, ResumeAgentForm{})
	return &dialog
}
