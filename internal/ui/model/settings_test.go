package model

import (
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/settingskey"
	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
)

func settingsTexts(v SettingsView) []text.Message {
	messages := []text.Message{v.Title, v.Headline, v.Advanced.Label}
	if v.Problem != nil {
		messages = append(messages, *v.Problem)
	}
	for _, f := range v.Fields {
		messages = append(messages, f.Description, f.Value, f.Source, f.Default)
		for _, optional := range []*text.Message{f.Problem, f.Override, f.Hint} {
			if optional != nil {
				messages = append(messages, *optional)
			}
		}
		for _, option := range f.Options {
			messages = append(messages, option.Label)
		}
		for _, action := range f.Actions {
			messages = append(messages, action.Label)
		}
	}
	return messages
}

func fieldView(t *testing.T, m *Settings, path string) FieldView {
	t.Helper()
	for _, f := range m.View().Fields {
		if f.Path == path {
			return f
		}
	}
	t.Fatalf("no field %s", path)
	return FieldView{}
}

func storedSettings() []SettingValue {
	return []SettingValue{
		{Key: "repo.url", Value: "git@example.invalid:me/config.git", Source: SourceFile},
		{Key: "sync.mode", Value: "copy", Source: SourceEnvironment, Name: "KITBAG_MODE"},
		{Key: "host", Value: "laptop", Source: SourceFlag, Name: "--host"},
		{Key: "sync.poll_interval", Value: "60s", Source: SourceDefault},
		{Key: "not.in.registry", Value: "ignored", Source: SourceFile},
	}
}

func TestSettingsFormFollowsRegistry(t *testing.T) {
	t.Parallel()
	m := NewSettings()
	cmds := m.Init()
	view := m.View()
	expect(t, view.Phase == Loading && view.Headline.Key == text.SettingsLoading, "loading=%+v", view)
	expect(t, len(view.Fields) == 8, "fields=%d", len(view.Fields))
	for _, f := range view.Fields {
		key, err := settingskey.Find(f.Path)
		expect(t, err == nil && f.Type == key.Type, "field=%+v", f)
		expect(t, len(f.Actions) == 0, "%s has actions before loading", key.Path)
		expect(t, render(f.Description) != "", "%s description missing", key.Path)
	}
	settle(t, m, &fakeClient{settingsErr: &Error{Kind: ErrInvalid}}, cmds)
	view = m.View()
	expect(t, view.Phase == Failed && view.Headline.Key == text.ErrorInvalid, "failed=%+v", view)
	settle(t, m, &fakeClient{settings: storedSettings()}, m.Update(Refresh{}))
	view = m.View()
	expect(t, view.Phase == Ready && view.Problem == nil, "ready=%+v", view)
	expect(t, render(view.Headline) == "Precedence: "+settingskey.Precedence(), "headline=%q", render(view.Headline))
	checkText(t, settingsTexts(view)...)
}

func readySettings(t *testing.T, client *fakeClient) *Settings {
	t.Helper()
	m := NewSettings()
	settle(t, m, client, m.Init())
	return m
}

func TestSettingsSensitiveValuesAreRedacted(t *testing.T) {
	t.Parallel()
	m := readySettings(t, &fakeClient{settings: storedSettings()})
	url := fieldView(t, m, "repo.url")
	expect(t, render(url.Value) == "[redacted]" && url.Sensitive, "secret shown: %+v", url)
	expect(t, url.Draft == "", "secret prefilled into the input: %q", url.Draft)
	expect(t, render(url.Source) == "config.toml" && url.Override == nil, "url source=%+v", url)
	m.Update(EditSetting{Key: "repo.url", Value: "https://example.invalid/new.git"})
	url = fieldView(t, m, "repo.url")
	expect(t, url.Draft == "https://example.invalid/new.git", "an edited secret draft must stay visible to its author")
	checkText(t, settingsTexts(m.View())...)
}

func TestSettingsSourcesAndOverrides(t *testing.T) {
	t.Parallel()
	m := readySettings(t, &fakeClient{settings: storedSettings()})
	m.Update(ToggleAdvancedSettings{})
	mode := fieldView(t, m, "sync.mode")
	expect(t, render(mode.Value) == "copy" && mode.Draft == "copy", "env field=%+v", mode)
	expect(t, render(*mode.Override) == "environment KITBAG_MODE overrides config.toml", "override=%q", render(*mode.Override))
	host := fieldView(t, m, "host")
	expect(t, render(host.Source) == "flag --host" && host.Override != nil, "flag field=%+v", host)
	poll := fieldView(t, m, "sync.poll_interval")
	expect(t, render(poll.Source) == "default" && render(poll.Default) == "Default: 60s", "default field=%+v", poll)
	branch := fieldView(t, m, "repo.branch")
	expect(t, render(branch.Value) == "unset", "unset value=%q", render(branch.Value))
	expect(t, render(branch.Default) == "Default: remote default branch", "unset default=%q", render(branch.Default))
	expect(t, sourceText(SettingValue{Source: "future"}).Text == "future", "unknown source hidden")
	checkText(t, settingsTexts(m.View())...)
}

func TestSettingsValidationBlocksSaving(t *testing.T) {
	t.Parallel()
	m := readySettings(t, &fakeClient{settings: storedSettings()})
	m.Update(ToggleAdvancedSettings{})
	for _, tc := range []struct{ key, value, problem string }{
		{"sync.mode", "symlink", "Allowed: link, copy"},
		{"sync.poll_interval", "500ms", "Allowed: duration from 1s to 24h (ms, s, m, h)"},
		{"host", "-bad", `Allowed: ^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`},
		{"repo.branch", "", `Allowed: ^[A-Za-z0-9][A-Za-z0-9/_.-]{0,254}$`},
	} {
		m.Update(EditSetting{Key: tc.key, Value: tc.value})
		f := fieldView(t, m, tc.key)
		expect(t, f.Problem != nil && render(*f.Problem) == tc.problem, "%s=%q problem=%v", tc.key, tc.value, f.Problem)
		expect(t, !controlEnabled(f.Actions, SaveSetting{Key: tc.key}), "invalid %s offered for saving", tc.key)
		expect(t, m.Update(SaveSetting{Key: tc.key}) == nil, "invalid %s saved", tc.key)
	}
	// Bypassing the edit path still cannot save an invalid draft.
	m.field("sync.mode").invalid = nil
	expect(t, m.Update(SaveSetting{Key: "sync.mode"}) == nil, "saved without validation")
	m.Update(RevertSetting{Key: "sync.mode"})
	f := fieldView(t, m, "sync.mode")
	expect(t, f.Draft == "copy" && f.Problem == nil && len(f.Actions) == 0, "revert=%+v", f)
	expect(t, m.Update(SaveSetting{Key: "sync.mode"}) == nil, "unchanged setting saved")
	expect(t, m.Update(SaveSetting{Key: "unknown"}) == nil, "unknown setting saved")
	checkText(t, settingsTexts(m.View())...)
}

func TestSettingsSave(t *testing.T) {
	t.Parallel()
	client := &fakeClient{settings: storedSettings()}
	m := readySettings(t, client)
	m.Update(EditSetting{Key: "sync.poll_interval", Value: "5m"})
	m.Update(EditSetting{Key: "repo.path", Value: "~/dark"})
	cmds := m.Update(SaveSetting{Key: "sync.poll_interval"})
	expect(t, only(t, cmds) == (WriteSetting{Key: "sync.poll_interval", Value: "5m"}), "save cmds=%v", cmds)
	saving := fieldView(t, m, "sync.poll_interval")
	expect(t, saving.Saving && saving.Value.Key == text.SettingSaving, "saving=%+v", saving)
	expect(t, m.Update(SaveSetting{Key: "sync.poll_interval"}) == nil, "saved twice")
	m.Update(EditSetting{Key: "sync.poll_interval", Value: "9m"})
	client.settings = append(storedSettings(), SettingValue{Key: "sync.poll_interval", Value: "5m", Source: SourceFile})
	settle(t, m, client, cmds)
	expect(t, len(client.writes) == 1 && client.writes[0].Value == "5m", "writes=%+v (edits while saving must be ignored)", client.writes)
	saved := fieldView(t, m, "sync.poll_interval")
	expect(t, !saved.Saving && saved.Draft == "5m", "saved=%+v", saved)
	expect(t, render(saved.Source) == "config.toml" && controlEnabled(saved.Actions, UnsetSetting{Key: "sync.poll_interval"}), "saved source=%+v", saved)
	expect(t, fieldView(t, m, "repo.path").Draft == "~/dark", "reload discarded another field's draft")
	checkText(t, settingsTexts(m.View())...)
}

func TestSettingsUnsetOnlyStoredValues(t *testing.T) {
	t.Parallel()
	m := readySettings(t, &fakeClient{settings: storedSettings()})
	expect(t, m.Update(UnsetSetting{Key: "sync.mode"}) == nil, "unset offered for an environment value")
	expect(t, m.Update(UnsetSetting{Key: "unknown"}) == nil, "unset offered for an unknown key")
	cmds := m.Update(UnsetSetting{Key: "repo.url"})
	expect(t, only(t, cmds) == (WriteSetting{Key: "repo.url", Unset: true}), "unset cmds=%v", cmds)
}

func TestSettingsWriteFailureKeepsDraft(t *testing.T) {
	t.Parallel()
	client := &fakeClient{settings: storedSettings(), writeErr: &Error{Kind: ErrInvalid, Detail: "name and email must be configured together"}}
	m := readySettings(t, client)
	m.Update(EditSetting{Key: "git.identity.name", Value: "Ada"})
	settle(t, m, client, m.Update(SaveSetting{Key: "git.identity.name"}))
	f := fieldView(t, m, "git.identity.name")
	expect(t, !f.Saving && f.Draft == "Ada" && f.Actions[0].Enabled, "failed save=%+v", f)
	expect(t, f.Problem != nil && render(*f.Problem) == "Not saved: The configuration or request is invalid", "problem=%v", f.Problem)
	expect(t, m.Update(SettingWritten{Key: "git.identity.name"}) == nil, "a write result for an idle field was applied")
	expect(t, m.Update(SettingWritten{Key: "unknown"}) == nil, "a write result for an unknown field was applied")
	checkText(t, settingsTexts(m.View())...)
}

func TestSettingsDiscardsStaleLoads(t *testing.T) {
	t.Parallel()
	m := NewSettings()
	first := as[LoadSettings](t, only(t, m.Init())).Seq
	second := as[LoadSettings](t, only(t, m.Update(Refresh{}))).Seq
	m.Update(SettingsLoaded{Seq: second, Values: []SettingValue{{Key: "repo.path", Value: "~/light", Source: SourceFile}}})
	m.Update(SettingsLoaded{Seq: first, Values: []SettingValue{{Key: "repo.path", Value: "~/dark", Source: SourceFile}}})
	m.Update(SettingsLoaded{Seq: first, Err: errBoom})
	expect(t, fieldView(t, m, "repo.path").Draft == "~/light" && m.problem == nil, "stale load applied")
}

func TestSettingsOnlyUsefulActions(t *testing.T) {
	t.Parallel()
	m := readySettings(t, &fakeClient{settings: storedSettings()})
	m.Update(ToggleAdvancedSettings{})
	for _, key := range []string{"sync.mode", "host", "sync.poll_interval"} {
		expect(t, len(fieldView(t, m, key).Actions) == 0, "%s has unchanged/inherited actions", key)
	}
	actions := fieldView(t, m, "repo.url").Actions
	expect(t, len(actions) == 1 && actions[0].Intent == (UnsetSetting{Key: "repo.url"}), "stored actions %+v", actions)
	m.Update(EditSetting{Key: "host", Value: "-bad"})
	actions = fieldView(t, m, "host").Actions
	expect(t, len(actions) == 2 && !actions[0].Enabled && actions[1].Enabled, "validation actions %+v", actions)
	m.Update(EditSetting{Key: "host", Value: "new-host"})
	m.Update(SaveSetting{Key: "host"})
	expect(t, len(fieldView(t, m, "host").Actions) == 0, "actions while saving")
}
