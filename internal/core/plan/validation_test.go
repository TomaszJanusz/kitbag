package plan_test

import (
	"errors"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/target"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

func TestRejectConflictingDeclarations(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"different source", "different content", "case", "unicode case", "ancestor", "owner", "relative", "escape", "repository", "kind", "backup", "merger", "generated source"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			a, b := conflictingDeclarations(scenario)
			_, err := plan.Build(fake.New(), request(a, b))
			if !errors.Is(err, fs.ErrInvalid) {
				t.Fatalf("unsafe declarations accepted: %v", err)
			}
		})
	}
}

func conflictingDeclarations(scenario string) (target.Target, target.Target) {
	a, b := declaration("same"), declaration("same")
	switch scenario {
	case "different source":
		b.SourcePath = path("repo", "other")
	case "different content":
		a.Kind = target.Generated
		b.Kind = target.Generated
		a.SourcePath = ""
		b.SourcePath = ""
		a.Content = []byte("a")
		b.Content = []byte("b")
	case "case":
		b.Path = path("home", "SAME")
	case "unicode case":
		a.Path = path("home", "ſkill")
		b.Path = path("home", "Skill")
	case "ancestor":
		b.Path = filepath.Join(a.Path, "child")
	default:
		b = invalidDeclaration(b, scenario)
	}

	return a, b
}

func TestDestinationParentAndCaseObstacles(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"link parent", "file parent", "case leaf", "case parent", "unicode leaf"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			h := fake.New()
			d := declaration("parent/skill")
			write(t, h, d.SourcePath, "source")
			prepareParent(t, h, &d, scenario)
			got := build(t, h, request(d))
			assertKinds(t, got, plan.Skip)
			assertClass(t, got, plan.UnmanagedObstacle)
			if scenario == "link parent" {
				want := "symbolic link in destination parents: " + path("home", "parent") + " -> " + path("outside")
				if got.Operations[0].Warning != want {
					t.Fatalf("warning = %q; want %q", got.Operations[0].Warning, want)
				}
			}
		})
	}
}

func prepareParent(t *testing.T, h *fake.Host, d *target.Target, scenario string) {
	t.Helper()
	switch scenario {
	case "link parent":
		link(t, h, path("outside"), filepath.Dir(d.Path))
	case "file parent":
		write(t, h, filepath.Dir(d.Path), "obstacle")
	case "case leaf":
		write(t, h, filepath.Join(filepath.Dir(d.Path), "SKILL"), "obstacle")
	case "case parent":
		write(t, h, path("home", "PARENT", "other"), "obstacle")
	case "unicode leaf":
		d.Path = path("home", "ſkill")
		write(t, h, path("home", "Skill"), "obstacle")
	}
}

func TestSourceLinkIsUnsupportedAndNeverFollowed(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"final", "parent", "child"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			h := fake.New()
			d := declaration("skill")
			write(t, h, path("outside", "file"), "outside")
			switch scenario {
			case "final":
				link(t, h, path("outside", "file"), d.SourcePath)
			case "parent":
				link(t, h, path("outside"), path("repo"))
				d.SourcePath = path("repo", "file")
			case "child":
				link(t, h, path("outside", "file"), filepath.Join(d.SourcePath, "link"))
			}
			got := build(t, h, request(d))
			assertKinds(t, got, plan.Skip)
		})
	}
}

func TestUnreadableWorldFailsWithoutPartialPlan(t *testing.T) {
	t.Parallel()
	for _, destination := range []bool{false, true} {
		t.Run(stringFlag(destination), func(t *testing.T) {
			t.Parallel()
			h := fake.New()
			d := declaration("private")
			write(t, h, d.SourcePath, "source")
			name := d.SourcePath
			if destination {
				name = d.Path
				write(t, h, name, "current")
			}
			if err := h.Chmod(name, 0); err != nil {
				t.Fatal(err)
			}
			got, err := plan.Build(h, request(d))
			if !errors.Is(err, fs.ErrPermission) || len(got.Operations) != 0 {
				t.Fatalf("Build = %v, %v", got, err)
			}
		})
	}
}

func stringFlag(value bool) string {
	if value {
		return "destination"
	}
	return "source"
}

func TestInvalidRequest(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"nil manifest", "manifest schema", "host unresolved", "relative root", "state schema", "duplicate state", "unsafe state", "state source"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			req := invalidRequest(scenario)
			if _, err := plan.Build(fake.New(), req); !errors.Is(err, fs.ErrInvalid) {
				t.Fatalf("invalid request accepted: %v", err)
			}
		})
	}
}

func invalidRequest(scenario string) plan.Request {
	req := request()
	d := declaration("old")
	old := tracked(d, plan.HashBytes(nil), state.Copy)
	switch scenario {
	case "nil manifest":
		req.Manifest = nil
	case "manifest schema":
		req.Manifest.Schema = 2
	case "host unresolved":
		req.Manifest.Hosts = map[string]manifest.Host{"unresolved": {}}
	case "relative root":
		req.Repository = "relative"
	case "state schema":
		req.State.SchemaVersion = state.CurrentVersion + 1
	case "duplicate state":
		req.State.Targets = []state.Target{old, old}
	case "unsafe state":
		old.Path = path("repo", "file")
		req.State.Targets = []state.Target{old}
	case "state source":
		old.SourcePath = ""
		req.State.Targets = []state.Target{old}
	}
	return req
}

func invalidDeclaration(b target.Target, scenario string) target.Target {
	switch scenario {
	case "owner":
		b.Owners = nil
	case "relative":
		b.Path = "relative"
	case "escape":
		b.SourcePath = path("outside")
	case "repository":
		b.Path = path("repo", "file")
	case "kind":
		b.Kind = "unknown"
	case "backup":
		b.Backup = "always"
	case "merger":
		b.Kind = target.Merged
	case "generated source":
		b.Kind = target.Generated
	}
	return b
}

func TestUnsupportedDeclarationProtectsPreviousTarget(t *testing.T) {
	t.Parallel()
	h := fake.New()
	d := declaration("unsupported")
	d.Warning = "format unsupported"
	write(t, h, d.Path, "original")
	req := request(d)
	req.State.Targets = []state.Target{tracked(d, plan.HashBytes([]byte("original")), state.Copy)}
	got := build(t, h, req)
	assertKinds(t, got, plan.Skip)
	applyTestPlan(t, h, &req, got, state.Copy)
	content, err := h.ReadFile(d.Path)
	if err != nil || string(content) != "original" || len(req.State.Targets) != 1 {
		t.Fatalf("unsupported target changed: %s %v", content, err)
	}
}

func TestDirectoryShapeChangeRequiresAdoption(t *testing.T) {
	t.Parallel()
	h := fake.New()
	d := declaration("directory")
	write(t, h, filepath.Join(d.SourcePath, "SKILL.md"), "old")
	req := request(d)
	applyTestPlan(t, h, &req, build(t, h, req), state.Copy)
	removeTestTree(t, h, d.SourcePath)
	write(t, h, d.SourcePath, "now a file")
	got := build(t, h, req)
	assertKinds(t, got, plan.Skip)
	assertClass(t, got, plan.Conflict)
}
