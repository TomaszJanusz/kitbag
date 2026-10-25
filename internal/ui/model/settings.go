package model

import (
	"github.com/TomaszJanusz/kitbag/internal/settingskey"
	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
)

// Settings is a form over the settings registry. Drafts are validated with the
// registry's own rules on every edit; only a valid, changed draft is saved.
// The complete file is validated again when it is written.
type Settings struct {
	problem *Error
	fields  []field
	issued  uint64
	applied uint64
	phase   Phase
	known   bool
}

type field struct {
	invalid *text.Message
	failed  *Error
	current SettingValue
	draft   string
	key     settingskey.Key
	saving  bool
}

// NewSettings returns a loading form with one field per registry key.
func NewSettings() *Settings {
	registry := settingskey.Registry()
	fields := make([]field, 0, len(registry))
	for _, key := range registry {
		fields = append(fields, field{key: key, current: SettingValue{Key: key.Path, Source: SourceDefault}})
	}
	return &Settings{fields: fields}
}

// Init requests the effective settings.
func (m *Settings) Init() []Cmd { return m.load() }

func (m *Settings) load() []Cmd {
	m.issued++
	return []Cmd{LoadSettings{Seq: m.issued}}
}

// Update applies msg and returns follow-up commands.
func (m *Settings) Update(msg Msg) []Cmd {
	switch msg := msg.(type) {
	case Refresh:
		return m.load()
	case SettingsLoaded:
		m.loaded(msg)
	case EditSetting:
		m.edit(msg)
	case SaveSetting:
		return m.save(msg.Key)
	case UnsetSetting:
		return m.unset(msg.Key)
	case RevertSetting:
		if f := m.field(msg.Key); f != nil && !f.saving {
			f.draft, f.invalid, f.failed = f.current.Value, nil, nil
		}
	case SettingWritten:
		return m.written(msg)
	}
	return nil
}

func (m *Settings) field(key string) *field {
	for i := range m.fields {
		if m.fields[i].key.Path == key {
			return &m.fields[i]
		}
	}
	return nil
}

func (m *Settings) loaded(msg SettingsLoaded) {
	if msg.Seq <= m.applied || msg.Seq > m.issued {
		return
	}
	m.applied = msg.Seq
	if msg.Err != nil {
		m.problem = Classify(msg.Err)
		if !m.known {
			m.phase = Failed
		}
		return
	}
	m.known, m.problem, m.phase = true, nil, Ready
	for _, value := range msg.Values {
		f := m.field(value.Key)
		if f == nil {
			continue
		}
		edited := f.draft != f.current.Value
		f.current = value
		if !edited {
			f.draft = value.Value
		}
	}
}

func (m *Settings) edit(msg EditSetting) {
	f := m.field(msg.Key)
	if f == nil || f.saving {
		return
	}
	f.draft, f.failed, f.invalid = msg.Value, nil, nil
	if err := f.key.Validate(msg.Value); err != nil {
		invalid := text.New(text.SettingInvalid, text.Plain(f.key.Constraint()))
		f.invalid = &invalid
	}
}

func (f *field) canSave() bool {
	return f.invalid == nil && !f.saving && f.draft != f.current.Value && f.key.Validate(f.draft) == nil
}

func (m *Settings) save(key string) []Cmd {
	f := m.field(key)
	if !m.known || f == nil || !f.canSave() {
		return nil
	}
	f.saving, f.failed = true, nil
	return []Cmd{WriteSetting{Key: key, Value: f.draft}}
}

func (m *Settings) unset(key string) []Cmd {
	f := m.field(key)
	if !m.known || f == nil || f.saving || f.current.Source != SourceFile {
		return nil
	}
	f.saving, f.failed = true, nil
	return []Cmd{WriteSetting{Key: key, Unset: true}}
}

func (m *Settings) written(msg SettingWritten) []Cmd {
	f := m.field(msg.Key)
	if f == nil || !f.saving {
		return nil
	}
	f.saving, f.failed = false, Classify(msg.Err)
	if f.failed == nil {
		// The reload replaces the draft with the stored value and its source.
		f.draft = f.current.Value
	}
	return m.load()
}

// FieldView is one form row. Choices lists closed value sets for pickers.
type FieldView struct {
	Problem     *text.Message
	Override    *text.Message
	Path        string
	Draft       string
	Type        string
	Description text.Message
	Value       text.Message
	Source      text.Message
	Default     text.Message
	Choices     []string
	Actions     []Control
	Sensitive   bool
	Saving      bool
}

// SettingsView is everything a renderer shows for settings.
type SettingsView struct {
	Problem  *text.Message
	Title    text.Message
	Headline text.Message
	Fields   []FieldView
	Phase    Phase
}

// View projects the current state.
func (m *Settings) View() SettingsView {
	view := SettingsView{Phase: m.phase, Title: text.New(text.SettingsTitle), Headline: text.New(text.SettingsReady, text.Plain(settingskey.Precedence()))}
	switch {
	case !m.known && m.phase == Failed:
		view.Headline = ProblemText(m.problem)
	case !m.known:
		view.Headline = text.New(text.SettingsLoading)
	}
	if m.problem != nil {
		problem := ProblemText(m.problem)
		view.Problem = &problem
	}
	for i := range m.fields {
		view.Fields = append(view.Fields, m.fields[i].view(m.known))
	}
	return view
}

func (f *field) view(known bool) FieldView {
	v := FieldView{
		Path: f.key.Path, Draft: f.draft, Type: f.key.Type, Sensitive: f.key.Sensitive, Saving: f.saving,
		Description: text.New(text.SettingDescription(f.key.Path)), Value: f.value(), Source: sourceText(f.current),
		Default: text.New(text.SettingDefault, text.Plain(f.key.DefaultDescription())), Choices: f.key.Allowed, Problem: f.invalid,
	}
	if f.key.Sensitive && f.draft == f.current.Value {
		// Never prefill an input with a stored secret; Value shows it redacted.
		v.Draft = ""
	}
	if f.failed != nil {
		failed := text.New(text.SettingSaveFailed, ProblemText(f.failed))
		v.Problem = &failed
	}
	if f.current.Source == SourceFlag || f.current.Source == SourceEnvironment {
		override := text.New(text.SettingOverridden, sourceText(f.current))
		v.Override = &override
	}
	v.Actions = []Control{
		{Intent: SaveSetting{Key: f.key.Path}, Label: text.New(text.ActionSave), Enabled: known && f.canSave()},
		{Intent: UnsetSetting{Key: f.key.Path}, Label: text.New(text.ActionUnset), Enabled: known && !f.saving && f.current.Source == SourceFile},
		{Intent: RevertSetting{Key: f.key.Path}, Label: text.New(text.ActionRevert), Enabled: !f.saving && f.draft != f.current.Value},
	}
	return v
}

func (f *field) value() text.Message {
	switch {
	case f.saving:
		return text.New(text.SettingSaving)
	case f.current.Value == "":
		return text.New(text.SettingUnset)
	case f.key.Sensitive:
		return text.New(text.SettingRedacted)
	default:
		return text.Plain(f.current.Value)
	}
}

func sourceText(value SettingValue) text.Message {
	switch value.Source {
	case SourceFlag:
		return text.New(text.SourceFlag, text.Plain(value.Name))
	case SourceEnvironment:
		return text.New(text.SourceEnvironment, text.Plain(value.Name))
	case SourceFile:
		return text.New(text.SourceFile)
	case SourceDefault:
		return text.New(text.SourceDefault)
	default:
		return text.Plain(string(value.Source))
	}
}
