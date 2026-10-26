package model

import (
	"slices"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
)

func settingPaths(m *Settings) []string {
	paths := make([]string, 0, len(m.View().Fields))
	for _, field := range m.View().Fields {
		paths = append(paths, field.Path)
	}
	return paths
}

func TestSettingsVisibility(t *testing.T) {
	t.Parallel()
	m := readySettings(t, &fakeClient{settings: storedSettings()})
	basic := []string{"host", "repo.url", "repo.path", "git.identity.name", "git.identity.email", "sync.poll_interval", "daemon.update_check", "sync.mode"}
	expect(t, slices.Equal(settingPaths(m), basic), "basic=%v", settingPaths(m))
	m.Update(ToggleAdvancedSettings{})
	advanced := append(slices.Clone(basic), "repo.branch", "git.backend", "daemon.debounce", "ui.color")
	expect(t, slices.Equal(settingPaths(m), advanced) && m.View().Expanded, "advanced=%v", settingPaths(m))
	m.Update(ToggleAdvancedSettings{})
	expect(t, slices.Equal(settingPaths(m), basic), "collapse=%v", settingPaths(m))
	for _, value := range []SettingValue{
		{Key: "migration.legacy_roots", Value: "[]", LegacyDetected: true},
		{Key: "migration.legacy_roots", Value: `["~/old"]`},
	} {
		seq := as[LoadSettings](t, only(t, m.Update(Refresh{}))).Seq
		m.Update(SettingsLoaded{Seq: seq, Values: []SettingValue{value}})
		expect(t, slices.Contains(settingPaths(m), value.Key), "migration hidden: %+v", value)
		expect(t, fieldView(t, m, value.Key).Editor == SettingReadOnly, "migration needs CLI array editing")
	}
	checkText(t, m.View().Advanced.Label)
}

func TestSettingsPickersAndCustomValidation(t *testing.T) {
	t.Parallel()
	m := readySettings(t, &fakeClient{settings: storedSettings()})
	m.Update(BeginSetting{Key: "sync.poll_interval"})
	f := fieldView(t, m, "sync.poll_interval")
	expect(t, f.Options[1].Selected && !f.TextEntry, "60s must match 1m: %+v", f)
	m.Update(StepSetting{Key: f.Path, Delta: 1})
	expect(t, fieldView(t, m, f.Path).Draft == "5m", "step did not pick 5m")
	m.Update(ChooseSetting{Key: f.Path, Index: len(f.Options) - 1})
	expect(t, fieldView(t, m, f.Path).TextEntry, "custom input missing")
	m.Update(EditSetting{Key: f.Path, Value: "25h"})
	expect(t, m.Update(SaveSetting{Key: f.Path}) == nil, "invalid custom duration saved")
	m.Update(EditSetting{Key: f.Path, Value: "90s"})
	expect(t, len(m.Update(SaveSetting{Key: f.Path})) == 1, "valid custom duration blocked")
	m.Update(ToggleAdvancedSettings{})
	debounce := fieldView(t, m, "daemon.debounce")
	for _, option := range debounce.Options {
		if !option.Custom {
			expect(t, m.field(debounce.Path).key.Validate(option.Value) == nil, "invalid preset %s", option.Value)
		}
	}
}

func TestSettingsHostToggleAndHints(t *testing.T) {
	t.Parallel()
	values := append(storedSettings(),
		SettingValue{Key: "host", Value: "MBP-TJ", Hosts: []string{"mac", "runner"}},
		SettingValue{Key: "daemon.update_check", Value: "true"},
		SettingValue{Key: "git.identity.name", GitValue: "Git Author"})
	m := readySettings(t, &fakeClient{settings: values})
	m.Update(BeginSetting{Key: "host"})
	expect(t, fieldView(t, m, "host").TextEntry, "unknown host must offer Other")
	m.Update(ChooseSetting{Key: "host", Index: 0})
	expect(t, fieldView(t, m, "host").Draft == "mac", "host choice missing")
	m.Update(StepSetting{Key: "daemon.update_check", Delta: 1})
	toggle := fieldView(t, m, "daemon.update_check")
	expect(t, toggle.Editor == SettingToggle && toggle.Draft == "false" && !toggle.TextEntry, "toggle=%+v", toggle)
	expect(t, render(*fieldView(t, m, "git.identity.name").Hint) == "From Git config: Git Author", "Git hint missing")
	m.Update(ChooseSetting{Key: "host", Index: -1})
	m.Update(ChooseSetting{Key: "host", Index: 99})
	expect(t, fieldView(t, m, "host").Draft == "mac", "invalid choice changed draft")
	checkText(t, text.New(text.SettingOther), toggle.Options[0].Label, toggle.Options[1].Label)
}
