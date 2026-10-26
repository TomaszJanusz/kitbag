package workflow

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/config"
	"github.com/TomaszJanusz/kitbag/internal/core/agents"
	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/core/undo"
	"github.com/TomaszJanusz/kitbag/internal/target"
	"github.com/TomaszJanusz/kitbag/internal/vcs"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// manifestName is the manifest's file name in the working copy.
const manifestName = "kitbag.toml"

// PublicationState says what happened to an agent change in Git.
type PublicationState string

// Publication states.
const (
	// PublishPreview is a dry run: nothing was written.
	PublishPreview PublicationState = "dry_run"
	// Published means the commit is on the remote branch.
	Published PublicationState = "published"
	// NotPublished means the commit exists only in the working copy.
	NotPublished PublicationState = "not_published"
	// Manual means the policy is pr: nothing was written; Next says what to do.
	Manual PublicationState = "manual"
	// Superseded means the replayed change no longer applies to the new
	// upstream manifest; nothing was left behind.
	Superseded PublicationState = "conflict"
)

// Publication reports the commit and how it reached the repository.
type Publication struct {
	State  PublicationState `json:"state"`
	Mode   string           `json:"mode"`
	Branch string           `json:"branch"`
	Commit string           `json:"commit,omitempty"`
	Reason string           `json:"reason,omitempty"`
	// Next lists commands that finish a manual or failed publication.
	Next []string `json:"next"`
	// Attempts counts pushes; a rejected first push is replayed once.
	Attempts int `json:"attempts"`
}

// AgentSync is the JSON projection of the sync run after publishing.
type AgentSync struct {
	Warnings []string   `json:"warnings"`
	Changed  int        `json:"changed"`
	ExitCode apply.Code `json:"exit_code"`
}

// AgentRelease reports how the outputs in a directory the manifest stopped
// declaring were handed back, by the rules of kitbag unmanage.
type AgentRelease struct {
	Dir      string   `json:"dir"`
	Changed  []string `json:"changed"`
	Warnings []string `json:"warnings"`
}

// AgentResult is the agent_change_schema=1 report of one change.
type AgentResult struct {
	// Released is set when a published change dropped a directory.
	Released *AgentRelease `json:"released"`
	// ReleaseResult is the full release report for human output.
	ReleaseResult *apply.Result `json:"-"`
	// Sync is set after a published change was synchronized.
	Sync *AgentSync `json:"sync"`
	// SyncResult is the full sync report for human output.
	SyncResult        *SyncResult `json:"-"`
	Manifest          string      `json:"manifest"`
	RevisionAfter     string      `json:"revision_after,omitempty"`
	Plan              agents.Plan `json:"plan"`
	Publication       Publication `json:"publication"`
	AgentChangeSchema int         `json:"agent_change_schema"`
	DryRun            bool        `json:"dry_run"`
}

// Code is the result's exit classification: a sync's code, 1 when the change
// is committed but not published, 0 for previews and manual instructions.
func (r AgentResult) Code() apply.Code {
	code := apply.OK
	if r.Publication.State == NotPublished {
		code = apply.Warnings
	}
	if r.SyncResult != nil {
		code = max(code, r.SyncResult.Code())
	}
	if r.Sync != nil {
		code = max(code, r.Sync.ExitCode)
	}
	if r.ReleaseResult != nil {
		code = max(code, r.ReleaseResult.Code())
	}
	return code
}

// AgentOptions select how ChangeAgent runs.
type AgentOptions struct {
	// Revision, when set, must equal the manifest revision a preview showed.
	Revision string
	// DryRun plans from the working copy as it is and writes nothing.
	DryRun bool
}

// ChangeAgent plans an instance change and, unless DryRun, publishes it as
// ADR 12's v1 slice does: under the state lock, on a clean working copy on its
// branch, it fast-forwards, writes kitbag.toml, commits with a generated
// message and pushes without force when the host's publish mode is direct.
// A rejected push is replayed once on the fetched upstream. A published
// change is then synchronized in the same lock.
func (s Service) ChangeAgent(ctx context.Context, change agents.Change, opts AgentOptions) (AgentResult, error) {
	result := AgentResult{AgentChangeSchema: 1, DryRun: opts.DryRun, Manifest: filepath.Join(s.Repository, manifestName), Publication: Publication{Next: []string{}}}
	if err := s.compatible(); err != nil {
		return result, err
	}
	if opts.DryRun {
		return result, s.previewAgent(ctx, change, opts, &result)
	}
	if s.Now == nil || s.Now().IsZero() {
		return result, fmt.Errorf("agent change clock: %w", invalidInput())
	}
	err := s.Store.WithLock(func(session *state.Session) error {
		return s.publishAgent(ctx, session, change, opts, &result)
	})
	return result, err
}

func (s Service) previewAgent(ctx context.Context, change agents.Change, opts AgentOptions, result *AgentResult) error {
	_, policy, err := s.agentPlan(change, opts.Revision, result)
	if err != nil {
		return err
	}
	if policy.Branch == "" {
		head, headErr := s.Backend.Head(ctx, s.Repository)
		if headErr != nil {
			return headErr
		}
		policy.Branch = head.Branch
	}
	result.Publication.Mode, result.Publication.Branch = policy.Mode, policy.Branch
	result.Publication.State = PublishPreview
	result.RevisionAfter = agents.Revision(result.Plan.After)
	return nil
}

// agentPlan reads the manifest and prepares change against it.
func (s Service) agentPlan(change agents.Change, revision string, result *AgentResult) ([]byte, manifest.Publish, error) {
	data, err := s.Host.ReadFile(result.Manifest)
	if err != nil {
		return nil, manifest.Publish{}, err
	}
	if revision != "" && revision != agents.Revision(data) {
		return nil, manifest.Publish{}, fmt.Errorf("%s changed since the preview; review the new preview: %w", manifestName, apply.ErrConflict)
	}
	host, _, err := config.Resolve(s.Host, config.Values{"host": s.Config.Host}, "host", s.HostFlag)
	if err != nil {
		return nil, manifest.Publish{}, err
	}
	result.Plan, err = agents.Prepare(agents.Input{Host: s.Host, Kinds: s.Kinds, Filename: result.Manifest, HostName: host, Manifest: data, Change: change})
	if err != nil {
		return nil, manifest.Publish{}, err
	}
	policy, err := s.publishPolicy(data, host)
	return data, policy, err
}

// publishPolicy resolves [publish] and [hosts.<host>.publish]; empty mode
// means direct and an empty branch means the working copy's branch.
func (s Service) publishPolicy(data []byte, host string) (manifest.Publish, error) {
	m, err := manifest.Parse(filepath.Join(s.Repository, manifestName), data)
	if err != nil {
		return manifest.Publish{}, err
	}
	resolved, err := m.ResolveHost(hostEnvironment{s.Host, host}, host)
	if err != nil {
		return manifest.Publish{}, err
	}
	policy := manifest.Publish{Mode: "direct"}
	if resolved.Publish != nil {
		policy = *resolved.Publish
	}
	if policy.Mode == "" {
		policy.Mode = "direct"
	}
	return policy, nil
}

func (s Service) publishAgent(ctx context.Context, session *state.Session, change agents.Change, opts AgentOptions, result *AgentResult) error {
	data, err := s.prepareCheckout(ctx, change, opts, result)
	if err != nil || result.Publication.State == Manual {
		return err
	}
	var before []target.Target
	if result.Plan.Dropped != "" {
		if before, err = s.declarations(); err != nil {
			return err
		}
	}
	if err = s.commitPlan(ctx, data, result); err != nil {
		return err
	}
	if err = s.push(ctx, change, result); err != nil || result.Publication.State != Published {
		return err
	}
	if result.Plan.Dropped != "" {
		if err = s.release(session, result.Plan.Dropped, before, result); err != nil {
			return err
		}
	}
	var synced SyncResult
	err = s.syncSession(ctx, session, true, false, &synced)
	result.SyncResult = &synced
	result.Sync = &AgentSync{ExitCode: max(synced.Code(), apply.ErrorCode(err)), Changed: len(synced.Apply.Changed) + len(synced.Includes.Added), Warnings: append([]string{}, synced.Warnings...)}
	return err
}

// prepareCheckout checks and fast-forwards the working copy, then plans the
// change on it. In pr mode it records the manual publication instead.
func (s Service) prepareCheckout(ctx context.Context, change agents.Change, opts AgentOptions, result *AgentResult) ([]byte, error) {
	status, err := s.cleanCheckout(ctx)
	if err != nil {
		return nil, err
	}
	if _, err = vcs.Pull(ctx, s.Backend, s.Repository); err != nil {
		return nil, err
	}
	data, policy, err := s.agentPlan(change, opts.Revision, result)
	if err != nil {
		return nil, err
	}
	result.Publication.Mode, result.Publication.Branch = policy.Mode, status.Branch
	if policy.Branch != "" && policy.Branch != status.Branch {
		return nil, fmt.Errorf("publish.branch %q differs from the working copy branch %q; agent edits publish only to the checked-out branch: %w", policy.Branch, status.Branch, invalidInput())
	}
	if policy.Mode != "direct" {
		s.manualPublication(result)
	}
	return data, nil
}

// declarations lists the outputs the working copy's manifest declares here.
func (s Service) declarations() ([]target.Target, error) {
	var warnings []string
	m, mode, err := s.intent(&warnings)
	if err != nil {
		return nil, err
	}
	req, err := s.request(m, mode, &warnings)
	if err != nil {
		return nil, err
	}
	return plan.Declarations(s.Host, req)
}

// release hands back what KitBag made in a directory the manifest no longer
// declares, by the rules of kitbag unmanage: unchanged links and copies are
// removed, displaced originals restored, merged settings unmerged, and
// changed outputs are left in place and forgotten, so no later sync touches
// that directory. The directory and other files stay. before are the
// declarations of the previous manifest, which know the settings mergers;
// outputs the new manifest still declares are kept.
func (s Service) release(session *state.Session, dir string, before []target.Target, result *AgentResult) error {
	only, err := s.dropped(dir)
	if err != nil {
		return err
	}
	roots, err := s.legacyRoots()
	if err != nil {
		return err
	}
	outcome, err := undo.Unmanage(s.Host, session, before, undo.Options{Now: s.Now, LegacyRoots: roots, Only: only, ReleaseChanged: true})
	result.ReleaseResult = &outcome.Result
	result.Released = released(dir, outcome)
	return err
}

// dropped accepts the state targets inside dir that the working copy's
// manifest no longer declares.
func (s Service) dropped(dir string) (func(string) bool, error) {
	home, err := s.Host.HomeDir()
	if err != nil {
		return nil, err
	}
	after, err := s.declarations()
	if err != nil {
		return nil, err
	}
	kept := map[string]bool{}
	for _, d := range after {
		kept[d.Path] = true
	}
	root := config.Expand(home, dir)
	return func(path string) bool {
		rel, relErr := filepath.Rel(root, path)
		return relErr == nil && filepath.IsLocal(rel) && !kept[path]
	}, nil
}

func released(dir string, outcome undo.Outcome) *AgentRelease {
	report := &AgentRelease{Dir: dir, Changed: []string{}, Warnings: []string{}}
	for _, op := range outcome.Result.Changed {
		report.Changed = append(report.Changed, string(op.Kind)+" "+op.Path)
	}
	for _, op := range outcome.Plan.Operations {
		if op.Kind == plan.Release && op.Warning != "" {
			report.Warnings = append(report.Warnings, op.Path+": "+op.Warning)
		}
	}
	for _, d := range append(outcome.Result.Warnings, outcome.Result.Conflicts...) {
		report.Warnings = append(report.Warnings, d.Path+": "+d.Message)
	}
	return report
}

// cleanCheckout requires the configured branch with an upstream, no local
// changes and no unpublished commits, so a KitBag commit never mixes with
// other work and never leaves the clone diverged.
func (s Service) cleanCheckout(ctx context.Context) (vcs.Status, error) {
	if err := s.checkBranch(ctx); err != nil {
		return vcs.Status{}, err
	}
	status, err := s.Backend.Status(ctx, s.Repository)
	switch {
	case err != nil:
		return status, err
	case status.Detached():
		return status, fmt.Errorf("the working copy %s has a detached HEAD; check out its branch first: %w", s.Repository, invalidInput())
	case status.Upstream == "" || status.UpstreamGone:
		return status, fmt.Errorf("branch %s of %s has no upstream to publish to: %w", status.Branch, s.Repository, invalidInput())
	case !status.Clean():
		return status, fmt.Errorf("the working copy %s has local changes; commit, push or discard them first so the agent change is a commit of its own: %w", s.Repository, invalidInput())
	case status.Ahead > 0:
		return status, fmt.Errorf("the working copy %s has %d unpublished commit(s); push them first: %w", s.Repository, status.Ahead, invalidInput())
	}
	return status, nil
}

// commitPlan writes the planned manifest and commits it. A failed commit
// (for example a refusing hook) restores the old bytes, so the working copy
// never stays dirty and sync keeps pulling.
func (s Service) commitPlan(ctx context.Context, before []byte, result *AgentResult) error {
	info, err := s.Host.Lstat(result.Manifest)
	if err != nil {
		return err
	}
	perm := info.Mode().Perm()
	if err = s.Host.WriteAtomic(result.Manifest, result.Plan.After, perm); err != nil {
		return err
	}
	commit, err := s.Backend.Commit(ctx, s.Repository, vcs.CommitOptions{Message: result.Plan.Message})
	if err != nil {
		return errors.Join(fmt.Errorf("commit %s: %w", manifestName, err), s.Host.WriteAtomic(result.Manifest, before, perm))
	}
	result.Publication.Commit, result.RevisionAfter = commit, agents.Revision(result.Plan.After)
	return nil
}

// push publishes the commit; a non-fast-forward rejection is replayed once
// on the fetched upstream from the same structured change.
func (s Service) push(ctx context.Context, change agents.Change, result *AgentResult) error {
	for {
		result.Publication.Attempts++
		err := s.Backend.Push(ctx, s.Repository, vcs.PushOptions{})
		switch {
		case err == nil:
			result.Publication.State = Published
			return nil
		case !errors.Is(err, vcs.ErrPushRejected) || result.Publication.Attempts > 1:
			s.notPublished(result, err)
			return nil
		}
		if err = s.replay(ctx, change, result); err != nil {
			return err
		}
	}
}

// replay drops the rejected local commit, fast-forwards to the new upstream
// and commits the same change prepared against the new manifest.
func (s Service) replay(ctx context.Context, change agents.Change, result *AgentResult) error {
	if err := s.Backend.Fetch(ctx, s.Repository); err != nil {
		s.notPublished(result, err)
		return err
	}
	if err := s.Backend.DropCommit(ctx, s.Repository, result.Publication.Commit); err != nil {
		s.notPublished(result, err)
		return err
	}
	result.Publication.Commit = ""
	if err := s.Backend.FastForward(ctx, s.Repository); err != nil {
		s.notPublished(result, err)
		return err
	}
	data, _, err := s.agentPlan(change, "", result)
	if err != nil {
		result.Publication.State = Superseded
		result.Publication.Reason = "the change no longer applies to the updated repository: " + err.Error()
		return fmt.Errorf("%s changed upstream and the change no longer applies (%w): %w", manifestName, err, apply.ErrConflict)
	}
	return s.commitPlan(ctx, data, result)
}

func (s Service) notPublished(result *AgentResult, err error) {
	result.Publication.State, result.Publication.Reason = NotPublished, err.Error()
	if result.Publication.Commit == "" {
		return
	}
	result.Publication.Next = []string{
		"git -C " + shellQuote(s.Repository) + " pull --rebase",
		"git -C " + shellQuote(s.Repository) + " push",
	}
}

// manualPublication explains the pr policy: v1 writes nothing, so the
// working copy stays clean and sync keeps pulling.
func (s Service) manualPublication(result *AgentResult) {
	branch := "kitbag/" + result.Plan.Change.Kind + "-" + result.Plan.Change.Name
	result.Publication.State = Manual
	result.Publication.Reason = "publish mode is pr for this host; agent edits do not open pull requests yet, so nothing was written"
	result.Publication.Next = []string{
		"git -C " + shellQuote(s.Repository) + " switch -c " + branch,
		"apply the diff above to " + result.Manifest,
		"git -C " + shellQuote(s.Repository) + " commit -am " + shellQuote(result.Plan.Title),
		"git -C " + shellQuote(s.Repository) + " push -u origin " + branch + ", then open a pull request",
	}
}

// shellQuote quotes an argument for the POSIX shell commands shown in Next.
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
