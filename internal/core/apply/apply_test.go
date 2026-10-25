package apply_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/target"
)

func TestFirstRepeatAndBackupOnce(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"link", "copy"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			h, store := testHost(t)
			d := declaration("file")
			write(t, h, d.SourcePath, "desired")
			write(t, h, d.Path, "original")
			first := run(t, h, store, request(d), options(mode))
			assertCode(t, first, apply.OK)
			if len(first.Changed) != 2 {
				t.Fatalf("first = %+v", first)
			}
			assertCode(t, run(t, h, store, request(d), options(mode)), apply.OK)
			if result := run(t, h, store, request(d), options(mode)); len(result.Changed) != 0 {
				t.Fatalf("repeat = %+v", result)
			}
			backup := d.Path + ".before-kitbag." + clock().Format(apply.BackupTimeLayout) + "-0"
			content(t, h, backup, "original")
			content(t, h, d.Path, "desired")
			db := database(t, store)
			if len(db.Targets) != 1 || db.Targets[0].Kind != state.Kind(mode) || db.Pending != nil {
				t.Fatalf("state = %+v", db)
			}
			// Loss of ownership must not authorize a second unmanaged backup.
			must(t, store.WithLock(func(s *state.Session) error { db.Targets = nil; db.History = nil; return s.Save(db) }))
			assertCode(t, run(t, h, store, request(d), options(mode)), apply.Conflict)
			content(t, h, backup, "original")
		})
	}
}

func TestCopyDriftAndForce(t *testing.T) {
	t.Parallel()
	h, store := testHost(t)
	d := declaration("file")
	write(t, h, d.SourcePath, "original")
	assertCode(t, run(t, h, store, request(d), options("copy")), apply.OK)
	write(t, h, d.Path, "local")
	assertCode(t, run(t, h, store, request(d), options("copy")), apply.Warnings)
	content(t, h, d.Path, "local")
	forced := options("copy")
	forced.Force = true
	assertCode(t, run(t, h, store, request(d), forced), apply.OK)
	content(t, h, d.Path, "original")
	entries, err := h.ReadDir("/home/instance")
	must(t, err)
	backups := 0
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".before-kitbag.") {
			backups++
			content(t, h, "/home/instance/"+entry.Name(), "local")
		}
	}
	if backups != 1 {
		t.Fatalf("backups = %d", backups)
	}
}

func TestMissingInstanceAndDirectoryObstacle(t *testing.T) {
	t.Parallel()
	h, store := testHost(t)
	missing, blocked, good := declaration("missing"), declaration("blocked"), declaration("good")
	missing.InstanceDir = "/home/missing"
	missing.Path = "/home/missing/file"
	for _, d := range []target.Target{missing, blocked, good} {
		write(t, h, d.SourcePath, "desired")
	}
	write(t, h, blocked.Path+"/keep", "local")
	result := run(t, h, store, request(missing, blocked, good), options(""))
	assertCode(t, result, apply.Conflict)
	if len(result.Warnings) != 1 || len(result.Conflicts) != 1 || len(result.Changed) != 1 {
		t.Fatalf("report = %+v", result)
	}
	content(t, h, blocked.Path+"/keep", "local")
	content(t, h, good.Path, "desired")
	result = run(t, h, store, request(missing, good), options(""))
	assertCode(t, result, apply.Warnings)
}

func TestLegacyRepair(t *testing.T) {
	t.Parallel()
	for i, root := range []string{"/old/agent-config", "/home/.agents/skills"} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()
			h, store := testHost(t)
			d := declaration("skill")
			write(t, h, d.SourcePath, "new")
			write(t, h, root+"/skill", "old")
			must(t, h.MkdirAll("/home/instance", 0o700))
			must(t, h.Link(root+"/skill", d.Path))
			req := request(d)
			req.LegacyRoots = []string{root}
			assertCode(t, run(t, h, store, req, options("")), apply.OK)
			link, err := h.ReadLink(d.Path)
			must(t, err)
			if link != d.SourcePath {
				t.Fatalf("link = %q", link)
			}
			content(t, h, root+"/skill", "old")
		})
	}
}

func TestGeneratedAndMetadataOnly(t *testing.T) {
	t.Parallel()
	h, store := testHost(t)
	d := declaration("generated")
	d.Kind = target.Generated
	d.SourcePath = ""
	d.Content = []byte("rendered")
	assertCode(t, run(t, h, store, request(d), options("link")), apply.OK)
	content(t, h, d.Path, "rendered")
	d.Owners = append(d.Owners, state.Owner{Primitive: "instructions", AgentInstance: "second"})
	result := run(t, h, store, request(d), options("link"))
	if len(result.Changed) != 1 || result.Changed[0].Kind != plan.Record {
		t.Fatalf("metadata = %+v", result)
	}
	if len(database(t, store).Targets[0].Owners) != 2 {
		t.Fatal("owners not saved")
	}
}

func TestOwnedCopyTreeForcePreservesLocalContent(t *testing.T) {
	t.Parallel()
	h, store := testHost(t)
	d := declaration("tree")
	write(t, h, d.SourcePath+"/file", "repository")
	assertCode(t, run(t, h, store, request(d), options("copy")), apply.OK)
	write(t, h, d.Path+"/file", "local edit")
	assertCode(t, run(t, h, store, request(d), options("copy")), apply.Warnings)
	force := options("copy")
	force.Force = true
	assertCode(t, run(t, h, store, request(d), force), apply.OK)
	content(t, h, d.Path+"/file", "repository")
	backup := d.Path + ".before-kitbag." + clock().Format(apply.BackupTimeLayout) + "-1/file"
	content(t, h, backup, "local edit")
}

func TestOwnedCopyRepairedFromLegacyLink(t *testing.T) {
	t.Parallel()
	h, store := testHost(t)
	d := declaration("file")
	write(t, h, d.SourcePath, "desired")
	assertCode(t, run(t, h, store, request(d), options("copy")), apply.OK)
	must(t, h.Link("/legacy/file", d.Path))
	req := request(d)
	req.LegacyRoots = []string{"/legacy"}
	assertCode(t, run(t, h, store, req, options("link")), apply.OK)
	content(t, h, d.Path, "desired")
}

func TestLegacyDanglingRuleAndLiveForeignLink(t *testing.T) {
	t.Parallel()
	h, store := testHost(t)
	dangling, foreign := declaration("dangling"), declaration("foreign")
	write(t, h, dangling.SourcePath, "desired")
	write(t, h, foreign.SourcePath, "desired")
	write(t, h, "/foreign/live", "unmanaged")
	must(t, h.MkdirAll("/home/instance", 0o700))
	must(t, h.Link("/foreign/missing", dangling.Path))
	must(t, h.Link("/foreign/live", foreign.Path))
	req := request(dangling, foreign)
	req.LegacyRoots = []string{"/legacy"}
	result := run(t, h, store, req, options("link"))
	assertCode(t, result, apply.Conflict)
	content(t, h, dangling.Path, "desired")
	content(t, h, foreign.Path, "unmanaged")
}
