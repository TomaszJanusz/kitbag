package model

import (
	"slices"
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
)

// ManifestUpgradeDialog is the window ID of the manifest upgrade.
const ManifestUpgradeDialog = "manifest-upgrade"

// ManifestUpgradePreview is a planned schema-1 to schema-2 upgrade that was
// not written; Current means kitbag.toml already uses schema 2. Pass
// Revision to ApplyManifestUpgrade.
type ManifestUpgradePreview struct {
	Revision, Title, MinToolVersion string
	// Conversions describe each change in order; CheckedHosts keep the same
	// active instances before and after.
	Conversions, Diff, Warnings, CheckedHosts []string
	Publication                               AgentPublication
	SchemaBefore                              int
	Current                                   bool
}

// ManifestUpgradeApplied is the outcome of ApplyManifestUpgrade.
type ManifestUpgradeApplied struct {
	Sync *AgentSyncReport
	ManifestUpgradePreview
	ExitCode int
}

// Manifest upgrade intents, commands and results.
type (
	// OpenManifestUpgrade opens the upgrade window and loads its preview.
	OpenManifestUpgrade struct{}
	// ConfirmManifestUpgrade applies the previewed upgrade.
	ConfirmManifestUpgrade struct{}
	// CloseManifestUpgrade closes the window; nothing more is written.
	CloseManifestUpgrade struct{}
	// PreviewManifestUpgrade calls Client.PreviewManifestUpgrade.
	PreviewManifestUpgrade struct{ Seq uint64 }
	// ApplyManifestUpgrade calls Client.ApplyManifestUpgrade.
	ApplyManifestUpgrade struct{ Revision string }
	// ManifestUpgradePreviewed carries the preview for Seq.
	ManifestUpgradePreviewed struct {
		Err     error
		Preview ManifestUpgradePreview
		Seq     uint64
	}
	// ManifestUpgradeDone reports ApplyManifestUpgrade.
	ManifestUpgradeDone struct {
		Err     error
		Applied ManifestUpgradeApplied
	}
)

func (OpenManifestUpgrade) isMsg()      {}
func (ConfirmManifestUpgrade) isMsg()   {}
func (CloseManifestUpgrade) isMsg()     {}
func (ManifestUpgradePreviewed) isMsg() {}
func (ManifestUpgradeDone) isMsg()      {}

func (PreviewManifestUpgrade) isCmd() {}
func (ApplyManifestUpgrade) isCmd()   {}

// ManifestUpgrade previews kitbag.toml's upgrade to schema 2 and, once
// confirmed, applies the previewed revision. A stale revision loads a new
// preview instead of applying.
type ManifestUpgrade struct {
	problem  *Error
	preview  *ManifestUpgradePreview
	applied  *ManifestUpgradeApplied
	issued   uint64
	open     bool
	loading  bool
	applying bool
	stale    bool
}

// NewManifestUpgrade returns a closed upgrade window.
func NewManifestUpgrade() *ManifestUpgrade { return &ManifestUpgrade{} }

// Update applies msg and returns follow-up commands.
func (m *ManifestUpgrade) Update(msg Msg) []Cmd {
	switch msg := msg.(type) {
	case OpenManifestUpgrade:
		*m = ManifestUpgrade{issued: m.issued, open: true}
		return m.requestPreview()
	case CloseManifestUpgrade:
		if !m.applying {
			*m = ManifestUpgrade{issued: m.issued + 1}
		}
	case ConfirmManifestUpgrade:
		return m.confirm()
	case ManifestUpgradePreviewed:
		m.previewed(msg)
	case ManifestUpgradeDone:
		return m.done(msg)
	}
	return nil
}

// confirm applies a loaded preview that still has something to change.
func (m *ManifestUpgrade) confirm() []Cmd {
	if !m.ready() {
		return nil
	}
	m.applying, m.problem, m.stale = true, nil, false
	return []Cmd{ApplyManifestUpgrade{Revision: m.preview.Revision}}
}

// ready reports whether Upgrade can apply the shown preview.
func (m *ManifestUpgrade) ready() bool {
	return m.open && !m.applying && m.preview != nil && !m.preview.Current && m.applied == nil
}

func (m *ManifestUpgrade) previewed(msg ManifestUpgradePreviewed) {
	if !m.open || msg.Seq != m.issued {
		return
	}
	m.loading, m.problem = false, Classify(msg.Err)
	if msg.Err == nil {
		preview := msg.Preview
		m.preview = &preview
	}
}

func (m *ManifestUpgrade) requestPreview() []Cmd {
	m.issued++
	m.preview, m.problem, m.loading = nil, nil, true
	return []Cmd{PreviewManifestUpgrade{Seq: m.issued}}
}

func (m *ManifestUpgrade) done(msg ManifestUpgradeDone) []Cmd {
	if !m.open || !m.applying {
		return nil
	}
	m.applying = false
	if problem := Classify(msg.Err); problem != nil {
		if problem.Kind != ErrConflict {
			m.problem = problem
			return nil
		}
		cmds := m.requestPreview()
		m.stale = true
		return cmds
	}
	applied := msg.Applied
	m.applied = &applied
	return nil
}

// Dialog projects the open window: the conversions, warnings, checked hosts,
// commit, route and diff, then the result. Cancel (Close) and Upgrade are
// the buttons; focus starts on Cancel so a stray Enter never publishes.
func (m *ManifestUpgrade) Dialog() *DialogView {
	if !m.open {
		return nil
	}
	cancel := Control{Intent: CloseManifestUpgrade{}, Label: text.New(text.WizardCancel), Enabled: !m.applying}
	if m.applied != nil {
		cancel.Label = text.New(text.WizardClose)
	}
	apply := Control{Intent: ConfirmManifestUpgrade{}, Label: text.New(text.UpgradeApply), Enabled: m.ready()}
	return &DialogView{
		ID: ManifestUpgradeDialog, Title: text.New(text.UpgradeTitle), Message: text.New(text.UpgradeExplain),
		Body: m.body(), Buttons: []Control{cancel, apply}, Cancel: cancel.Intent, Wide: true,
	}
}

func (m *ManifestUpgrade) body() []DialogLine {
	var body []DialogLine
	switch {
	case m.stale:
		body = append(body, DialogLine{Text: text.New(text.AgentErrorStale), Tone: ToneWarning})
	case m.problem != nil && m.problem.Detail != "":
		body = append(body, DialogLine{Text: text.New(text.AgentProblem, ProblemText(m.problem), text.Plain(m.problem.Detail)), Tone: ToneProblem})
	case m.problem != nil:
		body = append(body, DialogLine{Text: ProblemText(m.problem), Tone: ToneProblem})
	}
	switch {
	case m.loading:
		body = append(body, DialogLine{Text: text.New(text.UpgradeLoading), Tone: ToneHint})
	case m.applying:
		body = append(body, DialogLine{Text: text.New(text.UpgradeApplying), Tone: ToneHint})
	}
	if a := m.applied; a != nil {
		return append(body, upgradeResult(*a)...)
	}
	if p := m.preview; p != nil {
		body = append(body, upgradePreview(*p)...)
	}
	return body
}

func upgradePreview(p ManifestUpgradePreview) []DialogLine {
	if p.Current {
		return []DialogLine{{Text: text.New(text.UpgradeCurrent), Tone: ToneSuccess}}
	}
	body := []DialogLine{{Text: text.New(text.UpgradeChanges), Tone: ToneHeading}}
	body = append(body, lines(ToneNormal, plain(p.Conversions))...)
	body = append(body, lines(ToneWarning, plain(p.Warnings))...)
	// The upgrade also checks one host the file does not name; it stands for
	// every other machine.
	named := slices.DeleteFunc(slices.Clone(p.CheckedHosts), func(host string) bool { return strings.HasPrefix(host, "kitbag-undeclared-host") })
	body = append(body, DialogLine{Text: text.New(text.UpgradeChecked, text.Plain(strings.Join(append(named, "any other machine"), ", ")))})
	preview := previewView(AgentPreview{Title: p.Title, Diff: p.Diff, Publication: p.Publication})
	return append(body, previewBody(*preview, true)...)
}

func upgradeResult(a ManifestUpgradeApplied) []DialogLine {
	view := resultView(AgentApplied{AgentPreview: AgentPreview{Title: a.Title, Warnings: a.Warnings, Publication: a.Publication}, Sync: a.Sync})
	body := resultBody(*view)
	if a.Publication.State == PublishPublished {
		body = append([]DialogLine{{Text: text.New(text.UpgradeDone), Tone: ToneSuccess}}, body...)
	}
	return body
}
