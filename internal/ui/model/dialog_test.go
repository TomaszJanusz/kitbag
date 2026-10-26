package model

import (
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
)

func TestUnsetConfirmationIsModelOwned(t *testing.T) {
	t.Parallel()
	m := NewSettings()
	client := &fakeClient{settings: []SettingValue{{Key: "host", Value: "laptop", Source: SourceFile}}}
	settle(t, m, client, m.Init())
	expect(t, len(m.Update(RequestUnsetSetting{Key: "host"})) == 0, "request performed a write")
	dialog := m.View().Dialog
	expect(t, dialog != nil, "stored value has no confirmation")
	checkText(t, dialog.Title, dialog.Message, dialog.Buttons[0].Label, dialog.Buttons[1].Label)
	expect(t, dialog.InitialFocus == 1, "confirmation should default to No")
	m.Update(dialog.Cancel)
	expect(t, m.View().Dialog == nil, "cancel kept the window open")
	expect(t, len(client.writes) == 0, "cancel wrote configuration")
	m.Update(RequestUnsetSetting{Key: "host"})
	intent := m.View().Dialog.Buttons[0].Intent
	cmds := m.Update(intent)
	expect(t, m.View().Dialog == nil, "confirmation did not close")
	write := as[WriteSetting](t, only(t, cmds))
	expect(t, write.Key == "host" && write.Unset, "confirmation changed unset semantics: %+v", write)
}

func TestUnsetDoesNotOfferUnavailableActions(t *testing.T) {
	t.Parallel()
	for _, source := range []SourceKind{SourceDefault, SourceEnvironment, SourceFlag} {
		m := NewSettings()
		client := &fakeClient{settings: []SettingValue{{Key: "host", Value: "laptop", Source: source}}}
		settle(t, m, client, m.Init())
		for _, key := range []string{"host", "unknown"} {
			cmds := m.Update(RequestUnsetSetting{Key: key})
			expect(t, len(cmds) == 0 && m.View().Dialog == nil, "unset offered for %s/%s", source, key)
		}
	}
}

func TestFormPresetUsesSharedLabelsAndStepState(t *testing.T) {
	t.Parallel()
	for _, step := range []int{1, 2, 5} {
		view := FormDialog("wizard", text.Plain("Agent"), step, 5, nil, Control{Enabled: step > 1}, Control{Enabled: true}, DismissDialog{ID: "wizard"})
		checkText(t, view.Title, view.Buttons[0].Label, view.Buttons[1].Label)
		expect(t, view.Step == step && view.Steps == 5, "step changed: %+v", view)
		want := "Next"
		if step == 5 {
			want = "Finish"
		}
		expect(t, render(view.Buttons[1].Label) == want, "step %d label=%s", step, render(view.Buttons[1].Label))
		expect(t, !view.CloseOnAction, "form closed before model accepted the action")
	}
}
