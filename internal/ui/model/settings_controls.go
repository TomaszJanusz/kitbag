package model

import (
	"slices"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
)

// SettingEditor identifies a shared form control, independently of its renderer.
type SettingEditor int

// Settings controls. Custom text is offered only for hosts and durations.
const (
	SettingText SettingEditor = iota
	SettingSelect
	SettingToggle
	SettingDuration
	SettingHost
	SettingReadOnly
)

// SettingOption is a selectable value or an entry point to custom text.
type SettingOption struct {
	Value    string
	Label    text.Message
	Selected bool
	Custom   bool
}

func advancedSetting(path string) bool {
	return slices.Contains([]string{"repo.branch", "git.backend", "daemon.debounce", "ui.color"}, path)
}

func (f *field) visible(advanced bool) bool {
	switch f.key.Path {
	case "schema", "daemon.release_url", "ui.notifications", "ui.theme":
		return false
	case "migration.legacy_roots":
		return f.current.LegacyDetected || (f.current.Value != "" && f.current.Value != "[]")
	default:
		return advanced || !advancedSetting(f.key.Path)
	}
}

func (f *field) editor() SettingEditor {
	switch {
	case f.key.Path == "host":
		return SettingHost
	case f.key.Path == "migration.legacy_roots":
		return SettingReadOnly
	case f.key.Path == "sync.poll_interval" || f.key.Path == "daemon.debounce":
		return SettingDuration
	case f.key.Type == "boolean":
		return SettingToggle
	case len(f.key.Allowed) > 0:
		return SettingSelect
	default:
		return SettingText
	}
}

func (f *field) choices() []string {
	switch f.editor() {
	case SettingHost:
		return f.current.Hosts
	case SettingDuration:
		if f.key.Path == "daemon.debounce" {
			return []string{"100ms", "250ms", "500ms", "1s", "5s", "30s", "1m"}
		}
		return []string{"30s", "1m", "5m", "15m", "1h"}
	case SettingSelect, SettingToggle, SettingText, SettingReadOnly:
		return f.key.Allowed
	}
	return nil
}

func (f *field) options() []SettingOption {
	var options []SettingOption
	for _, value := range f.choices() {
		label := text.Plain(value)
		if f.editor() == SettingToggle {
			key := text.SettingOff
			if value == "true" {
				key = text.SettingOn
			}
			label = text.New(key)
		}
		options = append(options, SettingOption{Value: value, Label: label, Selected: !f.custom && f.matches(value)})
	}
	if f.editor() == SettingHost || f.editor() == SettingDuration {
		label := text.SettingCustom
		if f.editor() == SettingHost {
			label = text.SettingOther
		}
		selected := f.custom || !slices.ContainsFunc(options, func(o SettingOption) bool { return o.Selected })
		options = append(options, SettingOption{Label: text.New(label), Custom: true, Selected: selected})
	}
	return options
}

func (f *field) matches(value string) bool {
	if f.editor() != SettingDuration {
		return f.draft == value
	}
	a, aErr := time.ParseDuration(f.draft)
	b, bErr := time.ParseDuration(value)
	return aErr == nil && bErr == nil && a == b
}

func (m *Settings) begin(key string) {
	f := m.field(key)
	if f == nil || f.saving {
		return
	}
	f.custom = false
	f.custom = slices.ContainsFunc(f.options(), func(o SettingOption) bool { return o.Custom && o.Selected })
}

func (m *Settings) step(key string, delta int) {
	f := m.field(key)
	if f == nil || f.saving {
		return
	}
	options := f.options()
	if len(options) == 0 {
		return
	}
	index := slices.IndexFunc(options, func(o SettingOption) bool { return o.Selected })
	// Unknown closed-set values start with the first option in either direction.
	if index < 0 {
		m.choose(key, 0)
		return
	}
	m.choose(key, ((index+delta)%len(options)+len(options))%len(options))
}

func (m *Settings) choose(key string, index int) {
	f := m.field(key)
	if f == nil || f.saving {
		return
	}
	options := f.options()
	if index < 0 || index >= len(options) {
		return
	}
	option := options[index]
	f.custom = option.Custom
	if !option.Custom {
		m.edit(EditSetting{Key: key, Value: option.Value})
	}
}

func (f *field) hint() *text.Message {
	var hint text.Message
	switch f.key.Path {
	case "git.identity.name", "git.identity.email":
		hint = text.New(text.SettingGitUnavailable)
		if f.current.GitValue != "" {
			hint = text.New(text.SettingGitValue, text.Plain(f.current.GitValue))
		}
	case "migration.legacy_roots":
		hint = text.New(text.SettingLegacyCLI)
	default:
		return nil
	}
	return &hint
}
