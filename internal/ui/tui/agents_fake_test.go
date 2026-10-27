package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/TomaszJanusz/kitbag/internal/ui/model"
)

// fakeHome is the HOME of the scripted agents listing.
const fakeHome = "/home/user"

// agentFake scripts agents.list/add/edit/remove like the daemon: previews
// carry the manifest revision, an apply with an older revision is a conflict
// and a published change updates the listing. A gate, when set, holds every
// apply until it is closed. The zero value lists nothing.
type agentFake struct {
	previewErr error
	applyErr   error
	gate       chan struct{}
	state      model.PublishState
	mode       string
	probed     []model.AgentCandidate
	previews   []model.AgentChange
	applies    []string
	snapshot   model.AgentsSnapshot
	lists      int
	revision   int
	mu         sync.Mutex
}

// withAgents gives the fake a listing: two declared claude instances, the
// implicit codex default and an environment directory outside HOME; a probe
// finds two more directories.
func withAgents(f *fakeClient) *fakeClient {
	f.agents.snapshot = model.AgentsSnapshot{
		Host: "laptop", KnownHosts: []string{"laptop", "mac"}, Schema: 1, Daemon: true,
		Kinds: []string{"claude", "codex", "gemini", "opencode", "generic"},
		Instances: []model.AgentInstance{
			instance("claude", "main", "/.claude", model.PresencePresent, true),
			instance("claude", "second", "/.claude_2", model.PresenceMissing, true),
			{Target: "codex", Name: "default", AgentCandidate: model.AgentCandidate{Kind: "codex", Dir: fakeHome + "/.codex", Source: model.AgentSourceDefault, Presence: model.PresencePresent}, Implicit: true, Enabled: true, ActiveHere: true},
		},
		Candidates: []model.AgentCandidate{
			{Kind: "claude", Dir: "/srv/claude", Source: model.AgentSourceEnvironment, Variable: "CLAUDE_CONFIG_DIR", Presence: model.PresenceOutsideHome},
		},
	}
	f.agents.snapshot.Instances[1].Hosts = []string{"laptop"}
	f.agents.probed = []model.AgentCandidate{
		{Kind: "claude", Dir: fakeHome + "/.claude_3", Source: model.AgentSourceProbe, Presence: model.PresencePresent},
		{Kind: "codex", Dir: fakeHome + "/.codex-work", Source: model.AgentSourceProbe, Presence: model.PresencePresent},
	}
	f.agents.mode = "direct"
	return f
}

func instance(kind, name, dir string, presence model.AgentPresence, settings bool) model.AgentInstance {
	return model.AgentInstance{
		Target: kind, Name: name, Enabled: true, Settings: settings, ActiveHere: true,
		AgentCandidate: model.AgentCandidate{Kind: kind, Dir: fakeHome + dir, Source: model.AgentSourceDeclared, Presence: presence},
	}
}

func (f *fakeClient) Agents(_ context.Context, probe bool) (model.AgentsSnapshot, error) {
	defer f.enter()()
	return f.agents.list(probe), nil
}

// absolute resolves a typed ~/ directory below the fake HOME.
func absolute(dir string) string {
	if rel, ok := strings.CutPrefix(dir, "~/"); ok {
		return fakeHome + "/" + rel
	}
	return dir
}

// tilde shows a directory the way kitbag.toml stores it.
func tilde(dir string) string {
	if rel, ok := strings.CutPrefix(dir, fakeHome+"/"); ok {
		return "~/" + rel
	}
	return dir
}

func (a *agentFake) list(probe bool) model.AgentsSnapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lists++
	snapshot := a.snapshot
	snapshot.Instances = slices.Clone(snapshot.Instances)
	snapshot.Candidates = slices.Clone(snapshot.Candidates)
	if probe {
		for _, c := range a.probed {
			if !a.declared(c.Kind, c.Dir) {
				snapshot.Candidates = append(snapshot.Candidates, c)
			}
		}
	}
	return snapshot
}

func (a *agentFake) declared(kind, dir string) bool {
	return slices.ContainsFunc(a.snapshot.Instances, func(i model.AgentInstance) bool { return i.Kind == kind && i.Dir == dir })
}

func (a *agentFake) find(change model.AgentChange) int {
	return slices.IndexFunc(a.snapshot.Instances, func(i model.AgentInstance) bool {
		return !i.Implicit && i.Name == change.Name && (i.Target == change.Target || i.Kind == change.Target)
	})
}

// plan is the dry run: a title, diff lines and the route of the change.
func (a *agentFake) plan(change model.AgentChange) (model.AgentPreview, error) {
	if change.Settings != nil && change.Kind == "codex" {
		return model.AgentPreview{}, &model.Error{Kind: model.ErrInvalid, Detail: "target codex declares no settings fragment"}
	}
	preview := model.AgentPreview{
		Manifest: "/home/user/.local/share/kitbag/repo/kitbag.toml",
		Change:   change, Revision: fmt.Sprintf("r%d", a.revision),
		Publication: model.AgentPublication{State: model.PublishDryRun, Mode: a.mode, Branch: "main"},
	}
	switch change.Op {
	case model.AgentAdd:
		dir := tilde(absolute(change.Dir))
		preview.Change.Target, preview.Change.Dir = change.Kind, dir
		preview.Title = fmt.Sprintf("chore(kitbag): add %s instance %s at %s", change.Kind, change.Name, dir)
		preview.Diff = []string{"@@ kitbag.toml @@", fmt.Sprintf("+[targets.%s.instances.%s]", change.Kind, change.Name), fmt.Sprintf("+dir = %q", dir)}
	case model.AgentEdit:
		i := a.find(change)
		if i < 0 {
			return model.AgentPreview{}, &model.Error{Kind: model.ErrInvalid, Detail: "no instance " + change.Target + "/" + change.Name}
		}
		old := tilde(a.snapshot.Instances[i].Dir)
		dir := old
		if change.Dir != "" {
			dir = tilde(absolute(change.Dir))
		}
		preview.Title = fmt.Sprintf("chore(kitbag): move %s instance %s to %s", change.Target, change.Name, dir)
		preview.Diff = []string{fmt.Sprintf(" [targets.%s.instances.%s]", change.Target, change.Name), fmt.Sprintf("-dir = %q", old), fmt.Sprintf("+dir = %q", dir)}
		if dir != old {
			preview.Dropped = old
		}
	case model.AgentRemove:
		i := a.find(change)
		if i < 0 {
			return model.AgentPreview{}, &model.Error{Kind: model.ErrInvalid, Detail: "no instance " + change.Target + "/" + change.Name}
		}
		old := tilde(a.snapshot.Instances[i].Dir)
		preview.Title = fmt.Sprintf("chore(kitbag): remove %s instance %s", change.Target, change.Name)
		preview.Diff = []string{fmt.Sprintf("-[targets.%s.instances.%s]", change.Target, change.Name), fmt.Sprintf("-dir = %q", old)}
		preview.Dropped = old
		preview.Change.Dir = old
	}
	return fakePreviewEffects(preview, change.Op), nil
}

func fakePreviewEffects(preview model.AgentPreview, op model.AgentOp) model.AgentPreview {
	preview.Directory = absolute(preview.Change.Dir)
	preview.Effects = []model.AgentEffect{{Path: preview.Directory + "/CLAUDE.md", Kind: "materialize"}}
	if op == model.AgentRemove {
		preview.Effects = nil
	}
	if preview.Dropped != "" {
		preview.Effects = append(preview.Effects, model.AgentEffect{Path: absolute(preview.Dropped) + "/CLAUDE.md", Kind: "discard", Existing: true})
	}
	return preview
}

func (a *agentFake) preview(change model.AgentChange) (model.AgentPreview, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.previews = append(a.previews, change)
	if a.previewErr != nil {
		return model.AgentPreview{}, a.previewErr
	}
	return a.plan(change)
}

func (a *agentFake) apply(change model.AgentChange, revision string) (model.AgentApplied, error) {
	a.mu.Lock()
	gate := a.gate
	a.applies = append(a.applies, revision)
	a.mu.Unlock()
	if gate != nil {
		<-gate
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if revision != fmt.Sprintf("r%d", a.revision) {
		return model.AgentApplied{}, &model.Error{Kind: model.ErrConflict, Detail: "kitbag.toml changed since the preview"}
	}
	if a.applyErr != nil {
		return model.AgentApplied{}, a.applyErr
	}
	preview, err := a.plan(change)
	if err != nil {
		return model.AgentApplied{}, err
	}
	applied := model.AgentApplied{AgentPreview: preview}
	applied.Publication.Attempts = 1
	switch {
	case a.mode == "pr":
		applied.Publication.State, applied.Publication.Reason = model.PublishManual, "publish mode is pr for this host; nothing was written"
		applied.Publication.Next = []string{"git -C '/home/user/.local/share/kitbag/repo' switch -c kitbag/" + change.Kind + "-" + change.Name, "git -C '/home/user/.local/share/kitbag/repo' push -u origin kitbag/" + change.Kind + "-" + change.Name}
		return applied, nil
	case a.state == model.PublishNotPublished:
		applied.Publication.State, applied.Publication.Commit = model.PublishNotPublished, "4f2a9c1d0b7e6a5f"
		applied.Publication.Reason = "push rejected: the remote branch moved twice"
		applied.Publication.Next = []string{"git -C '/home/user/.local/share/kitbag/repo' pull --rebase", "git -C '/home/user/.local/share/kitbag/repo' push"}
		applied.ExitCode = 1
		return applied, nil
	}
	applied.Publication.State, applied.Publication.Commit = model.PublishPublished, "9b8c7d6e5f4a3b2c"
	applied.Sync = &model.AgentSyncReport{Changed: 2}
	a.publish(preview)
	if preview.Dropped != "" {
		applied.Released = &model.AgentRelease{Dir: preview.Dropped, Changed: []string{"discard " + preview.Dropped + "/CLAUDE.md"}}
	}
	return applied, nil
}

// publish changes the listing as the published manifest would.
func (a *agentFake) publish(preview model.AgentPreview) {
	a.revision++
	change := preview.Change
	switch change.Op {
	case model.AgentAdd:
		dir := absolute(change.Dir)
		a.snapshot.Candidates = slices.DeleteFunc(a.snapshot.Candidates, func(c model.AgentCandidate) bool { return c.Dir == dir })
		a.probed = slices.DeleteFunc(a.probed, func(c model.AgentCandidate) bool { return c.Dir == dir })
		a.snapshot.Instances = append(a.snapshot.Instances, instance(change.Kind, change.Name, strings.TrimPrefix(dir, fakeHome), model.PresencePresent, change.Settings == nil || *change.Settings))
	case model.AgentEdit:
		i := a.find(change)
		if change.Dir != "" {
			a.snapshot.Instances[i].Dir = absolute(change.Dir)
		}
		if change.Settings != nil {
			a.snapshot.Instances[i].Settings = *change.Settings
		}
	case model.AgentRemove:
		a.snapshot.Instances = slices.Delete(a.snapshot.Instances, a.find(change), a.find(change)+1)
	}
}

// bump simulates another machine publishing a manifest change.
func (a *agentFake) bump() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.revision++
}

func (a *agentFake) set(update func(*agentFake)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	update(a)
}

func (a *agentFake) calls() (lists int, previews []model.AgentChange, applies []string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.lists, slices.Clone(a.previews), slices.Clone(a.applies)
}

func (a *agentFake) instances() []model.AgentInstance {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.snapshot.Instances)
}

func (f *fakeClient) PreviewAgentChange(_ context.Context, change model.AgentChange) (model.AgentPreview, error) {
	defer f.enter()()
	return f.agents.preview(change)
}

func (f *fakeClient) ApplyAgentChange(_ context.Context, change model.AgentChange, revision string) (model.AgentApplied, error) {
	defer f.enter()()
	return f.agents.apply(change, revision)
}
