package workflow_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/core/agents"
	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/core/discover"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/core/workflow"
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/target"
	"github.com/TomaszJanusz/kitbag/internal/vcs"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

const agentManifest = "schema = 1\n\n[targets.claude]\nagent = \"claude\"\nsettings = \"claude/settings.json\"\n\n" +
	"[targets.claude.instances.main]\ndir = \"~/.claude\"\n"

// publisher is a Git fake that records commits, pushes and drops.
type publisher struct {
	vcs.Backend
	fetchErr  error
	dropErr   error
	onForward func()
	onDrop    func()
	commitErr error
	pushErrs  []error
	messages  []string
	dropped   []string
	status    vcs.Status
	pushes    int
	fetches   int
	forwards  int
}

func (p *publisher) Status(context.Context, string) (vcs.Status, error) { return p.status, nil }
func (p *publisher) Head(context.Context, string) (vcs.Head, error)     { return p.status.Head, nil }

func (p *publisher) Fetch(context.Context, string) error {
	p.fetches++
	if p.fetches > 1 {
		return p.fetchErr
	}
	return nil
}

func (p *publisher) FastForward(context.Context, string) error {
	p.forwards++
	if p.onForward != nil {
		p.onForward()
	}
	return nil
}

func (p *publisher) Commit(_ context.Context, _ string, opts vcs.CommitOptions) (string, error) {
	if p.commitErr != nil {
		return "", p.commitErr
	}
	p.messages = append(p.messages, opts.Message)
	return fmt.Sprintf("c%d", len(p.messages)), nil
}

func (p *publisher) Push(context.Context, string, vcs.PushOptions) error {
	p.pushes++
	if len(p.pushErrs) == 0 {
		return nil
	}
	err := p.pushErrs[0]
	p.pushErrs = p.pushErrs[1:]
	return err
}

func (p *publisher) DropCommit(_ context.Context, _, commit string) error {
	if p.dropErr != nil {
		return p.dropErr
	}
	p.dropped = append(p.dropped, commit)
	if p.onDrop != nil {
		p.onDrop()
	}
	return nil
}

type agentFixture struct {
	host *fake.Host
	git  *publisher
	path string
	s    workflow.Service
}

func agentSetup(t *testing.T, manifestText string) *agentFixture {
	t.Helper()
	home := filepath.Join(string(filepath.Separator), "home", "test")
	h := fake.New(fake.WithEnv(map[string]string{"HOME": home, "KITBAG_HOST": "mac"}))
	store, err := state.New(h, h, state.Options{})
	if err != nil {
		t.Fatal(err)
	}
	f := &agentFixture{host: h, git: &publisher{status: vcs.Status{Head: vcs.Head{Branch: "main", Commit: "abc"}, Upstream: "origin/main"}}}
	repo := filepath.Join(home, "repo")
	f.path = filepath.Join(repo, "kitbag.toml")
	table := discover.Table{{Agent: "claude", Variable: "CLAUDE_CONFIG_DIR", DefaultDir: "~/.claude", Home: func(env platform.Env) (string, error) { return env.AgentHome("CLAUDE_CONFIG_DIR", ".claude") }}}
	f.s = workflow.Service{Host: h, Backend: f.git, Store: store, Repository: repo, Version: "0.1.0", Kinds: table, Now: func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) }}
	f.s.Adapters = func(*manifest.Manifest, *[]string) ([]target.Adapter, error) {
		return []target.Adapter{fileAdapter{home: home}}, nil
	}
	write(t, h, f.path, manifestText)
	// Dropping the local commit restores the committed-over manifest.
	f.git.onDrop = func() { write(t, h, f.path, manifestText) }
	write(t, h, filepath.Join(repo, "claude", "settings.json"), "{}")
	for _, dir := range []string{".claude", ".claude_2"} {
		if err = h.MkdirAll(filepath.Join(home, dir), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func (f *agentFixture) manifest(t *testing.T) string {
	t.Helper()
	data, err := f.host.ReadFile(f.path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// expect fails the test when ok is false, showing got.
func expect(t *testing.T, ok bool, what string, got any) {
	t.Helper()
	if !ok {
		t.Fatalf("%s: %+v", what, got)
	}
}

func addSecond() agents.Change {
	return agents.Change{Op: agents.Add, Kind: "claude", Name: "second", Dir: "~/.claude_2"}
}

func TestChangeAgentDryRunWritesNothing(t *testing.T) {
	t.Parallel()
	f := agentSetup(t, agentManifest)
	r, err := f.s.ChangeAgent(context.Background(), addSecond(), workflow.AgentOptions{DryRun: true})
	expect(t, err == nil, "error", err)
	expect(t, r.Publication.State == workflow.PublishPreview && r.Publication.Mode == "direct" && r.Publication.Branch == "main", "publication", r.Publication)
	expect(t, r.DryRun && r.AgentChangeSchema == 1 && r.RevisionAfter != "" && r.Code() == apply.OK, "result", r)
	expect(t, f.manifest(t) == agentManifest && len(f.git.messages) == 0 && f.git.pushes == 0 && f.git.fetches == 0, "dry run wrote", f.git)
}

func TestChangeAgentRefusesUnsafeWorkingCopies(t *testing.T) {
	t.Parallel()
	for name, status := range map[string]vcs.Status{
		"dirty":       {Head: vcs.Head{Branch: "main", Commit: "abc"}, Upstream: "origin/main", Changes: []vcs.Change{{Path: "x", Kind: vcs.ChangeUntracked}}},
		"ahead":       {Head: vcs.Head{Branch: "main", Commit: "abc"}, Upstream: "origin/main", Ahead: 1},
		"detached":    {Head: vcs.Head{Commit: "abc"}},
		"no upstream": {Head: vcs.Head{Branch: "main", Commit: "abc"}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := agentSetup(t, agentManifest)
			f.git.status = status
			_, err := f.s.ChangeAgent(context.Background(), addSecond(), workflow.AgentOptions{})
			if !errors.Is(err, fs.ErrInvalid) || apply.ErrorCode(err) != apply.Usage {
				t.Fatalf("got %v", err)
			}
			if f.manifest(t) != agentManifest || len(f.git.messages) != 0 {
				t.Fatal("refused change wrote")
			}
		})
	}
}

func TestChangeAgentStaleRevisionConflicts(t *testing.T) {
	t.Parallel()
	f := agentSetup(t, agentManifest)
	_, err := f.s.ChangeAgent(context.Background(), addSecond(), workflow.AgentOptions{Revision: "stale"})
	if !errors.Is(err, apply.ErrConflict) || apply.ErrorCode(err) != apply.Conflict || f.manifest(t) != agentManifest {
		t.Fatalf("got %v", err)
	}
	if _, err = f.s.ChangeAgent(context.Background(), addSecond(), workflow.AgentOptions{Revision: agents.Revision([]byte(agentManifest))}); err != nil {
		t.Fatalf("current revision: %v", err)
	}
}

func TestChangeAgentPullRequestModeWritesNothing(t *testing.T) {
	t.Parallel()
	text := agentManifest + "\n[publish]\nmode = \"pr\"\n"
	f := agentSetup(t, text)
	r, err := f.s.ChangeAgent(context.Background(), addSecond(), workflow.AgentOptions{})
	if err != nil || r.Publication.State != workflow.Manual || len(r.Publication.Next) == 0 || r.Code() != apply.OK {
		t.Fatalf("%+v, %v", r.Publication, err)
	}
	if f.manifest(t) != text || len(f.git.messages) != 0 || f.git.pushes != 0 || r.SyncResult != nil {
		t.Fatal("pr mode wrote")
	}
}

func TestChangeAgentPublishesAndSyncs(t *testing.T) {
	t.Parallel()
	f := agentSetup(t, agentManifest)
	r, err := f.s.ChangeAgent(context.Background(), addSecond(), workflow.AgentOptions{})
	expect(t, err == nil, "error", err)
	expect(t, r.Publication.State == workflow.Published && r.Publication.Commit == "c1" && r.Publication.Attempts == 1, "publication", r.Publication)
	expect(t, slices.Equal(f.git.messages, []string{r.Plan.Message}) && f.git.pushes == 1, "git", f.git)
	expect(t, strings.Contains(r.Plan.Message, "Kitbag-Host: mac"), "message", r.Plan.Message)
	expect(t, r.SyncResult != nil && r.Sync != nil && r.Sync.ExitCode == apply.OK, "sync", r.Sync)
	expect(t, r.Released == nil && r.Code() == apply.OK, "result", r)
	written := f.manifest(t)
	expect(t, strings.Contains(written, "[targets.claude.instances.second]") && r.RevisionAfter == agents.Revision([]byte(written)), "manifest", written)
}

func TestChangeAgentReplaysOneRejection(t *testing.T) {
	t.Parallel()
	f := agentSetup(t, agentManifest)
	f.git.pushErrs = []error{fmt.Errorf("push: %w", vcs.ErrPushRejected)}
	upstream := agentManifest + "# edited upstream\n"
	f.git.onForward = func() { write(t, f.host, f.path, upstream) }
	r, err := f.s.ChangeAgent(context.Background(), addSecond(), workflow.AgentOptions{})
	if err != nil || r.Publication.State != workflow.Published || r.Publication.Attempts != 2 || r.Publication.Commit != "c2" {
		t.Fatalf("%+v, %v", r.Publication, err)
	}
	if !slices.Equal(f.git.dropped, []string{"c1"}) || f.git.forwards != 1 || len(f.git.messages) != 2 {
		t.Fatalf("replay %+v", f.git)
	}
	if got := f.manifest(t); !strings.Contains(got, "# edited upstream") || !strings.Contains(got, "instances.second") {
		t.Fatalf("replayed manifest %s", got)
	}
}

func TestChangeAgentGivesUpAfterSecondRejection(t *testing.T) {
	t.Parallel()
	f := agentSetup(t, agentManifest)
	rejected := fmt.Errorf("push: %w", vcs.ErrPushRejected)
	f.git.pushErrs = []error{rejected, rejected}
	r, err := f.s.ChangeAgent(context.Background(), addSecond(), workflow.AgentOptions{})
	if err != nil || r.Publication.State != workflow.NotPublished || r.Publication.Commit != "c2" || r.Code() != apply.Warnings {
		t.Fatalf("%+v, %v", r.Publication, err)
	}
	if len(r.Publication.Next) != 2 || !strings.Contains(r.Publication.Next[0], "pull --rebase") || r.SyncResult != nil {
		t.Fatalf("next %q", r.Publication.Next)
	}
}

func TestChangeAgentOtherPushFailureKeepsCommit(t *testing.T) {
	t.Parallel()
	f := agentSetup(t, agentManifest)
	f.git.pushErrs = []error{errors.New("network down")}
	r, err := f.s.ChangeAgent(context.Background(), addSecond(), workflow.AgentOptions{})
	if err != nil || r.Publication.State != workflow.NotPublished || r.Publication.Reason != "network down" || len(f.git.dropped) != 0 {
		t.Fatalf("%+v, %v", r.Publication, err)
	}
}

func TestChangeAgentReplayConflict(t *testing.T) {
	t.Parallel()
	f := agentSetup(t, agentManifest)
	f.git.pushErrs = []error{fmt.Errorf("push: %w", vcs.ErrPushRejected)}
	upstream := agentManifest + "\n[targets.claude.instances.second]\ndir = \"~/.claude_other\"\n"
	f.git.onForward = func() { write(t, f.host, f.path, upstream) }
	r, err := f.s.ChangeAgent(context.Background(), addSecond(), workflow.AgentOptions{})
	if !errors.Is(err, apply.ErrConflict) || r.Publication.State != workflow.Superseded || r.Publication.Commit != "" {
		t.Fatalf("%+v, %v", r.Publication, err)
	}
	if f.manifest(t) != upstream || len(f.git.messages) != 1 || f.git.pushes != 1 {
		t.Fatalf("left behind: %+v", f.git)
	}
}

func TestChangeAgentCommitFailureRestoresManifest(t *testing.T) {
	t.Parallel()
	f := agentSetup(t, agentManifest)
	f.git.commitErr = errors.New("hook refused")
	_, err := f.s.ChangeAgent(context.Background(), addSecond(), workflow.AgentOptions{})
	if err == nil || !strings.Contains(err.Error(), "hook refused") || f.manifest(t) != agentManifest || f.git.pushes != 0 {
		t.Fatalf("got %v", err)
	}
}

func TestChangeAgentRemoveReleasesDirectory(t *testing.T) {
	t.Parallel()
	f := agentSetup(t, agentManifest+"\n[targets.claude.instances.second]\ndir = \"~/.claude_2\"\n")
	r, err := f.s.ChangeAgent(context.Background(), agents.Change{Op: agents.Remove, Target: "claude", Name: "second"}, workflow.AgentOptions{})
	if err != nil || r.Publication.State != workflow.Published || r.Released == nil || r.Released.Dir != "~/.claude_2" || r.ReleaseResult == nil {
		t.Fatalf("%+v, %v", r, err)
	}
	if strings.Contains(f.manifest(t), "second") || r.SyncResult == nil {
		t.Fatal("not removed or not synchronized")
	}
}

func TestChangeAgentNeedsClock(t *testing.T) {
	t.Parallel()
	f := agentSetup(t, agentManifest)
	f.s.Now = nil
	if _, err := f.s.ChangeAgent(context.Background(), addSecond(), workflow.AgentOptions{}); !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("got %v", err)
	}
}

func TestChangeAgentRefusesOtherPublishBranch(t *testing.T) {
	t.Parallel()
	text := agentManifest + "\n[publish]\nbranch = \"release\"\n"
	f := agentSetup(t, text)
	_, err := f.s.ChangeAgent(context.Background(), addSecond(), workflow.AgentOptions{})
	expect(t, errors.Is(err, fs.ErrInvalid) && strings.Contains(err.Error(), "publish.branch"), "error", err)
	expect(t, f.manifest(t) == text && len(f.git.messages) == 0, "wrote", f.git)
}

func TestChangeAgentHostPublishOverride(t *testing.T) {
	t.Parallel()
	f := agentSetup(t, agentManifest+"\n[hosts.mac]\npublish = { mode = \"pr\" }\n")
	r, err := f.s.ChangeAgent(context.Background(), addSecond(), workflow.AgentOptions{DryRun: true})
	expect(t, err == nil && r.Publication.Mode == "pr", "publication", r.Publication)
}

func TestChangeAgentReplayFailuresKeepCommit(t *testing.T) {
	t.Parallel()
	for name, configure := range map[string]func(*publisher){
		"fetch": func(p *publisher) { p.fetchErr = errors.New("offline") },
		"drop":  func(p *publisher) { p.dropErr = errors.New("cannot drop") },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := agentSetup(t, agentManifest)
			f.git.pushErrs = []error{fmt.Errorf("push: %w", vcs.ErrPushRejected)}
			configure(f.git)
			r, err := f.s.ChangeAgent(context.Background(), addSecond(), workflow.AgentOptions{})
			expect(t, err != nil && r.Publication.State == workflow.NotPublished && r.Publication.Commit == "c1", "publication", r.Publication)
			expect(t, len(r.Publication.Next) == 2, "next", r.Publication.Next)
		})
	}
}
