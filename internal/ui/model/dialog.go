package model

import "github.com/TomaszJanusz/kitbag/internal/ui/model/text"

// DialogView describes a window without terminal geometry or toolkit state.
// IDs are stable across steps. A renderer sends field edits and button intents
// back to the owning model; only that model validates and advances a wizard.
//
// Focus indexes count Fields, then Options, then Buttons. Body lines follow
// the fields and precede the options. Wide asks for a wider window, for
// example to show diffs.
type DialogView struct {
	Cancel         Msg
	ID             string
	Title, Message text.Message
	Fields         []DialogField
	Body           []DialogLine
	Options        []DialogOption
	Buttons        []Control
	Step, Steps    int
	InitialFocus   int
	CloseOnAction  bool
	Wide           bool
}

// Tone classifies a dialog body line. Renderers style it, and mark warnings
// and problems in text too, so the meaning survives without colour.
type Tone int

// Body line tones. ToneCode lines are verbatim (diffs, commands): renderers
// truncate instead of wrapping them.
const (
	ToneNormal Tone = iota
	ToneHeading
	ToneHint
	ToneSuccess
	ToneWarning
	ToneProblem
	ToneCode
)

// DialogLine is one paragraph of a window body; an empty Text is a blank line.
type DialogLine struct {
	Text text.Message
	Tone Tone
}

// DialogOption is one choice of a window's choice list. Choosing it dispatches
// Intent to the owning model, which reprojects Selected.
type DialogOption struct {
	Intent        Msg
	Label, Detail text.Message
	Selected      bool
}

// DialogField is a text field. Value is a draft supplied by its owning model.
// Sensitive drafts are masked by renderers; stored secrets must not be supplied.
type DialogField struct {
	ID, Value          string
	Label, Hint        text.Message
	Enabled, Sensitive bool
}

// EditDialogField forwards a draft change to the model owning Dialog.
type EditDialogField struct{ Dialog, Field, Value string }

// DismissDialog cancels a window owned by a shared model.
type DismissDialog struct{ ID string }

// RequestUnsetSetting asks for confirmation before removing a stored value.
type RequestUnsetSetting struct{ Key string }

func (EditDialogField) isMsg()     {}
func (DismissDialog) isMsg()       {}
func (RequestUnsetSetting) isMsg() {}

// ConfirmDialog offers Yes/No and initially focuses No. Enter activates the
// focused button; Esc and outside clicks send cancel.
func ConfirmDialog(id string, title, message text.Message, confirm, cancel Msg) DialogView {
	return DialogView{
		ID: id, Title: title, Message: message, Cancel: cancel, InitialFocus: 1, CloseOnAction: true,
		Buttons: []Control{
			{Intent: confirm, Label: text.New(text.DialogYes), Enabled: true},
			{Intent: cancel, Label: text.New(text.DialogNo), Enabled: true},
		},
	}
}

// FormDialog supplies Back/Next/Finish labels and a step indicator. The owning
// model supplies enabled controls, validation, cancel policy and field drafts.
func FormDialog(id string, title text.Message, step, steps int, fields []DialogField, back, next Control, cancel Msg) DialogView {
	back.Label = text.New(text.DialogBack)
	next.Label = text.New(text.DialogNext)
	if step == steps {
		next.Label = text.New(text.DialogFinish)
	}
	return DialogView{
		ID: id, Title: title, Step: step, Steps: steps, Fields: fields,
		Buttons: []Control{back, next}, Cancel: cancel,
	}
}
