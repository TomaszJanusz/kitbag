package apply_test

import (
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/target"
)

func TestRechecksChangedDestinationSourceAndParents(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"file", "link", "parent", "case alias", "source", "instance"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			h, store := testHost(t)
			d := declaration("file")
			write(t, h, d.SourcePath, "desired")
			d.InstanceDir = "/home/instance"
			must(t, h.MkdirAll(d.InstanceDir, 0o700))
			must(t, store.WithLock(func(s *state.Session) error {
				db, err := s.Load()
				if err != nil {
					return err
				}
				req := request(d)
				req.State = db
				p, err := plan.Build(h, req)
				if err != nil {
					return err
				}
				changePrecondition(t, h, d, scenario)
				result, err := apply.Execute(h, s, p, options("copy"))
				if err != nil {
					return err
				}
				assertCode(t, result, apply.Conflict)
				after, err := s.Load()
				if err != nil {
					return err
				}
				if len(after.Targets) != 0 || after.Pending != nil {
					t.Fatal("conflict recorded state")
				}
				if scenario == "file" {
					content(t, h, d.Path, "unmanaged")
				}
				return nil
			}))
		})
	}
}

func TestBackupRechecksAndBlocksReplacement(t *testing.T) {
	t.Parallel()
	h, store := testHost(t)
	d := declaration("file")
	write(t, h, d.SourcePath, "desired")
	write(t, h, d.Path, "original")
	must(t, store.WithLock(func(s *state.Session) error {
		db, err := s.Load()
		if err != nil {
			return err
		}
		req := request(d)
		req.State = db
		p, err := plan.Build(h, req)
		if err != nil {
			return err
		}
		write(t, h, d.Path, "new unmanaged content")
		result, err := apply.Execute(h, s, p, options("copy"))
		if err != nil {
			return err
		}
		assertCode(t, result, apply.Conflict)
		if len(result.Changed) != 0 {
			t.Fatal("applied replacement after failed backup")
		}
		content(t, h, d.Path, "new unmanaged content")
		return nil
	}))
}

func changePrecondition(t *testing.T, h *fake.Host, d target.Target, scenario string) {
	t.Helper()
	switch scenario {
	case "file":
		write(t, h, d.Path, "unmanaged")
	case "link":
		must(t, h.Link("/foreign", d.Path))
	case "parent":
		must(t, h.Remove(d.InstanceDir))
		must(t, h.Link("/repo", d.InstanceDir))
	case "case alias":
		write(t, h, "/home/instance/FILE", "unmanaged")
	case "source":
		write(t, h, d.SourcePath, "changed source")
	case "instance":
		must(t, h.Remove(d.InstanceDir))
	}
}

func TestBackupNamespaceCreatedAfterPlanning(t *testing.T) {
	t.Parallel()
	h, store := testHost(t)
	d := declaration("file")
	write(t, h, d.SourcePath, "desired")
	write(t, h, d.Path, "original")
	must(t, store.WithLock(func(s *state.Session) error {
		db, err := s.Load()
		if err != nil {
			return err
		}
		req := request(d)
		req.State = db
		p, err := plan.Build(h, req)
		if err != nil {
			return err
		}
		write(t, h, d.Path+".before-kitbag.external", "existing backup")
		result, err := apply.Execute(h, s, p, options("copy"))
		if err != nil {
			return err
		}
		assertCode(t, result, apply.Conflict)
		content(t, h, d.Path, "original")
		if len(result.Changed) != 0 {
			t.Fatal("backup raced namespace check")
		}
		return nil
	}))
}

func TestCannotReplaceStateOrLockDirectory(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"/home/.local/state/kitbag/state.json", "/home/.local/state/kitbag/state.lock", "/home/.local/state"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			h, store := testHost(t)
			d := declaration("file")
			d.Path = path
			write(t, h, d.SourcePath, "cannot replace state")
			_, err := apply.Run(h, store, request(d), options("copy"))
			if apply.ErrorCode(err) != apply.Usage {
				t.Fatalf("state destination = %v", err)
			}
			if db := database(t, store); db.Pending != nil || len(db.Targets) != 0 {
				t.Fatalf("state changed: %+v", db)
			}
		})
	}
}
