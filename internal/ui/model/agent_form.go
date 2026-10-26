package model

import (
	"slices"
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// AgentStep is one step of the agent form.
type AgentStep string

// Form steps. Add uses all; edit starts at the directory; remove at preview.
const (
	AgentStepKind     AgentStep = "kind"
	AgentStepName     AgentStep = "name"
	AgentStepDir      AgentStep = "dir"
	AgentStepSettings AgentStep = "settings"
	AgentStepPreview  AgentStep = "preview"
	AgentStepConfirm  AgentStep = "confirm"
	AgentStepResult   AgentStep = "result"
)

// Settings choices on the settings step, in option order.
const (
	settingsUnchanged = iota
	settingsOn
	settingsOff
)

// genericKind needs explicit roots, which the form cannot collect.
const genericKind = "generic"

// AgentForm adds, edits or removes one agent instance: draft steps validated
// on every edit with the manifest grammar and the last agents listing, a
// sequence-numbered preview, then a confirmed apply guarded by the
// preview's revision. The server validates again authoritatively.
type AgentForm struct {
	problem *Error
	preview *AgentPreview
	applied *AgentApplied
	draft   AgentChange
	// currentDir is the edited or removed instance's directory, for display.
	currentDir string
	steps      []AgentStep
	snapshot   AgentsSnapshot
	step       int
	issued     uint64
	open       bool
	touched    bool
	asking     bool
	loading    bool
	applying   bool
	stale      bool
}

// NewAgentForm returns a closed form.
func NewAgentForm() *AgentForm { return &AgentForm{} }

// Open reports whether the form is shown.
func (m *AgentForm) Open() bool { return m.open }

func formSteps(op AgentOp) []AgentStep {
	switch op {
	case AgentAdd:
		return []AgentStep{AgentStepKind, AgentStepName, AgentStepDir, AgentStepSettings, AgentStepPreview, AgentStepConfirm, AgentStepResult}
	case AgentEdit:
		return []AgentStep{AgentStepDir, AgentStepSettings, AgentStepPreview, AgentStepConfirm, AgentStepResult}
	case AgentRemove:
		return []AgentStep{AgentStepPreview, AgentStepConfirm, AgentStepResult}
	}
	return nil
}

func (m *AgentForm) current() AgentStep {
	if !m.open {
		return ""
	}
	return m.steps[m.step]
}

// Update applies msg and returns follow-up commands. Intents are ignored
// while the form is closed, except OpenAgentForm.
func (m *AgentForm) Update(msg Msg) []Cmd {
	switch msg := msg.(type) {
	case AgentsLoaded:
		if msg.Err == nil {
			m.snapshot = msg.Snapshot
		}
	case OpenAgentForm:
		return m.start(msg)
	case AgentPreviewed:
		m.previewed(msg)
	case AgentChangeApplied:
		return m.finished(msg)
	case EditAgentField, ChooseAgentOption, AgentFormNext, AgentFormBack, CancelAgentForm:
		if m.open && !m.applying {
			return m.intent(msg)
		}
	}
	return nil
}

func (m *AgentForm) intent(msg Msg) []Cmd {
	if _, cancel := msg.(CancelAgentForm); cancel {
		m.cancel()
		return nil
	}
	if _, back := msg.(AgentFormBack); !back || m.step > 0 {
		m.asking = false
	}
	switch msg := msg.(type) {
	case EditAgentField:
		m.edit(msg.Value)
	case ChooseAgentOption:
		m.choose(msg.Index)
	case AgentFormNext:
		return m.next()
	case AgentFormBack:
		return m.back()
	}
	return nil
}

func (m *AgentForm) start(msg OpenAgentForm) []Cmd {
	steps := formSteps(msg.Op)
	if steps == nil {
		return nil
	}
	*m = AgentForm{snapshot: m.snapshot, issued: m.issued, open: true, steps: steps}
	m.draft = AgentChange{Op: msg.Op, Kind: msg.Kind, Target: msg.Target, Name: msg.Name}
	if msg.Op == AgentAdd {
		m.draft.Dir = msg.Dir
	} else {
		m.currentDir = msg.Dir
	}
	if m.current() == AgentStepPreview {
		return m.requestPreview()
	}
	return nil
}

func (m *AgentForm) requestPreview() []Cmd {
	m.issued++
	m.preview, m.problem, m.loading = nil, nil, true
	return []Cmd{PreviewAgent{Seq: m.issued, Change: m.draft}}
}

// discard drops the preview and any answer still on its way.
func (m *AgentForm) discard() {
	m.issued++
	m.preview, m.loading, m.stale = nil, false, false
}

func (m *AgentForm) edit(value string) {
	switch m.current() {
	case AgentStepName:
		m.draft.Name = value
	case AgentStepDir:
		m.draft.Dir = value
	case AgentStepKind, AgentStepSettings, AgentStepPreview, AgentStepConfirm, AgentStepResult:
		return
	}
	m.touched = true
	m.discard()
}

func (m *AgentForm) choose(index int) {
	switch m.current() {
	case AgentStepKind:
		kinds := m.kinds()
		if index < 0 || index >= len(kinds) {
			return
		}
		m.draft.Kind = kinds[index]
	case AgentStepDir:
		hints := m.hints()
		if index < 0 || index >= len(hints) {
			return
		}
		m.draft.Dir = hints[index].Dir
	case AgentStepSettings:
		if !m.chooseSettings(index) {
			return
		}
	case AgentStepName, AgentStepPreview, AgentStepConfirm, AgentStepResult:
		return
	}
	m.touched = true
	m.discard()
}

func (m *AgentForm) chooseSettings(index int) bool {
	on, off := true, false
	switch index {
	case settingsUnchanged:
		m.draft.Settings = nil
	case settingsOn:
		m.draft.Settings = &on
	case settingsOff:
		m.draft.Settings = &off
	default:
		return false
	}
	return true
}

func (m *AgentForm) next() []Cmd {
	switch m.current() {
	case AgentStepResult:
		m.close()
	case AgentStepConfirm:
		if m.preview != nil {
			m.applying, m.problem, m.stale = true, nil, false
			return []Cmd{ApplyAgent{Change: m.draft, Revision: m.preview.Revision}}
		}
	case AgentStepPreview:
		if m.preview != nil && m.problem == nil {
			m.step++
		}
	case AgentStepKind, AgentStepName, AgentStepDir, AgentStepSettings:
		return m.nextDraft()
	}
	return nil
}

// nextDraft leaves a valid draft step; entering preview requests it.
func (m *AgentForm) nextDraft() []Cmd {
	if m.invalid() != nil {
		return nil
	}
	m.step++
	if m.current() == AgentStepPreview {
		return m.requestPreview()
	}
	return nil
}

func (m *AgentForm) back() []Cmd {
	switch step := m.current(); {
	case m.step == 0 || step == AgentStepResult:
		m.cancel()
	case step == AgentStepConfirm:
		m.step--
		m.problem = nil
	default:
		m.step--
		m.problem = nil
		m.discard()
	}
	return nil
}

// cancel closes at once without user input or on the result; otherwise it
// first asks, and a second cancel discards the draft.
func (m *AgentForm) cancel() {
	if m.touched && !m.asking && m.current() != AgentStepResult {
		m.asking = true
		return
	}
	m.close()
}

func (m *AgentForm) close() {
	*m = AgentForm{snapshot: m.snapshot, issued: m.issued + 1}
}

func (m *AgentForm) previewed(msg AgentPreviewed) {
	if !m.open || msg.Seq != m.issued || m.current() != AgentStepPreview {
		return
	}
	m.loading = false
	if msg.Err != nil {
		m.problem = Classify(msg.Err)
		return
	}
	preview := msg.Preview
	m.preview = &preview
}

func (m *AgentForm) finished(msg AgentChangeApplied) []Cmd {
	if !m.open || !m.applying {
		return nil
	}
	m.applying = false
	if problem := Classify(msg.Err); problem != nil {
		if problem.Kind != ErrConflict {
			m.problem = problem
			return nil
		}
		m.step = slices.Index(m.steps, AgentStepPreview)
		cmds := m.requestPreview()
		m.stale = true
		return cmds
	}
	applied := msg.Applied
	m.applied = &applied
	m.step = len(m.steps) - 1
	return nil
}

// kinds offers the listed agent kinds; generic is listed but invalid.
func (m *AgentForm) kinds() []string { return m.snapshot.Kinds }

// hints are the undeclared directories found for the draft kind.
func (m *AgentForm) hints() []AgentCandidate {
	var result []AgentCandidate
	for _, c := range m.snapshot.Candidates {
		if c.Kind == m.draft.Kind {
			result = append(result, c)
		}
	}
	return result
}

// invalid validates the current step's draft; nil allows Next.
func (m *AgentForm) invalid() *text.Message {
	var problem text.Message
	switch m.current() {
	case AgentStepKind:
		problem = m.invalidKind()
	case AgentStepName:
		problem = m.invalidName()
	case AgentStepDir:
		problem = m.invalidDir()
	case AgentStepSettings, AgentStepPreview, AgentStepConfirm, AgentStepResult:
	}
	if problem.Key == "" {
		return nil
	}
	return &problem
}

func (m *AgentForm) invalidKind() text.Message {
	switch kind := m.draft.Kind; {
	case kind == genericKind:
		return text.New(text.WizardGeneric)
	case kind == "" || (len(m.snapshot.Kinds) > 0 && !slices.Contains(m.snapshot.Kinds, kind)):
		return text.New(text.AgentInvalidKind)
	}
	return text.Message{}
}

func (m *AgentForm) invalidName() text.Message {
	if manifest.CheckName(m.draft.Name) != nil {
		return text.New(text.AgentInvalidName)
	}
	for _, i := range m.snapshot.Instances {
		if !i.Implicit && i.Kind == m.draft.Kind && i.Name == m.draft.Name && (m.draft.Target == "" || i.Target == m.draft.Target) {
			return text.New(text.AgentInvalidNameTaken, text.Plain(i.Target), text.Plain(i.Name))
		}
	}
	return text.Message{}
}

func (m *AgentForm) invalidDir() text.Message {
	dir := m.draft.Dir
	switch {
	case dir == "" && m.draft.Op == AgentEdit:
		return text.Message{}
	case strings.HasPrefix(dir, "/"):
	case !strings.HasPrefix(dir, "~/") || manifest.CheckHomePath(dir) != nil:
		return text.New(text.AgentInvalidDir)
	}
	if problem := m.outsideHome(dir); problem.Key != "" {
		return problem
	}
	return m.duplicate(dir)
}

// outsideHome refuses a directory listed outside HOME.
func (m *AgentForm) outsideHome(dir string) text.Message {
	for _, c := range m.snapshot.Candidates {
		if c.Presence == PresenceOutsideHome && sameDir(c.Dir, dir) {
			return text.New(text.AgentInvalidOutside, text.Plain(dir))
		}
	}
	return text.Message{}
}

// duplicate refuses a directory another instance of the kind declares.
func (m *AgentForm) duplicate(dir string) text.Message {
	for _, i := range m.snapshot.Instances {
		if i.Implicit || m.edited(i) || i.Kind != m.kind() || !sameDir(i.Dir, dir) {
			continue
		}
		return text.New(text.AgentInvalidDuplicate, text.Plain(dir), text.Plain(i.Target+"/"+i.Name))
	}
	return text.Message{}
}

// edited reports whether i is the instance an edit changes.
func (m *AgentForm) edited(i AgentInstance) bool {
	return m.draft.Op == AgentEdit && i.Name == m.draft.Name && (i.Target == m.draft.Target || i.Kind == m.draft.Target)
}

// kind is the draft's agent kind; edits name only a target or kind.
func (m *AgentForm) kind() string {
	if m.draft.Kind != "" {
		return m.draft.Kind
	}
	for _, i := range m.snapshot.Instances {
		if i.Target == m.draft.Target || i.Kind == m.draft.Target {
			return i.Kind
		}
	}
	return m.draft.Target
}

// sameDir compares a listed absolute directory with a typed one: absolute
// paths case-insensitively, ~/ paths by their case-insensitive suffix.
func sameDir(absolute, typed string) bool {
	absolute, typed = strings.ToLower(strings.TrimSuffix(absolute, "/")), strings.ToLower(strings.TrimSuffix(typed, "/"))
	if rel, ok := strings.CutPrefix(typed, "~/"); ok {
		return strings.HasSuffix(absolute, "/"+rel)
	}
	return absolute == typed
}

// warnings explain a draft directory listed as missing.
func (m *AgentForm) warnings() []text.Message {
	if m.current() != AgentStepDir || m.draft.Dir == "" {
		return nil
	}
	for _, c := range append(m.listed(), m.snapshot.Candidates...) {
		if c.Presence == PresenceMissing && sameDir(c.Dir, m.draft.Dir) {
			return []text.Message{text.New(text.AgentWarnMissingDir, text.Plain(m.draft.Dir))}
		}
	}
	return nil
}

func (m *AgentForm) listed() []AgentCandidate {
	result := make([]AgentCandidate, 0, len(m.snapshot.Instances))
	for _, i := range m.snapshot.Instances {
		result = append(result, i.AgentCandidate)
	}
	return result
}
