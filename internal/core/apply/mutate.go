package apply

import (
	"errors"
	"io/fs"
	"path/filepath"

	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform"
)

const (
	privateDir     fs.FileMode = 0o700
	privateFile    fs.FileMode = 0o600
	executableBits fs.FileMode = 0o111
)

func mutate(fsys platform.FS, s state.Step, op plan.Operation) error {
	if s.Action == "output" {
		if err := stage(fsys, s, op); err != nil {
			return err
		}
	}
	if err := checkSource(fsys, op); err != nil {
		return err
	}
	// Recheck after staging (which may take time) and immediately before rename.
	if err := check(fsys, s.Path, s.Before); err != nil {
		return err
	}
	if err := moveAside(fsys, s, op); err != nil {
		return err
	}
	if s.Action != "output" {
		return nil
	}
	if err := check(fsys, s.Path, state.Entry{}); err != nil {
		return err
	}
	return fsys.Rename(s.Stage, s.Path)
}

func stage(fsys platform.FS, s state.Step, op plan.Operation) error {
	if err := check(fsys, s.Stage, state.Entry{}); err != nil {
		return err
	}
	if err := fsys.MkdirAll(filepath.Dir(s.Stage), privateDir); err != nil {
		return err
	}
	if s.After.Kind == state.Link {
		return fsys.Link(s.After.SourcePath, s.Stage)
	}
	if err := stageBytes(fsys, s, op); err != nil {
		return err
	}
	got, err := plan.HashTree(fsys, s.Stage)
	if err != nil {
		return err
	}
	if got != s.After.WrittenHash {
		return errChanged()
	}
	return checkSource(fsys, op)
}

func copyTree(fsys platform.FS, source, dest string) error {
	entry, err := plan.Inspect(fsys, source)
	if err != nil {
		return err
	}
	if entry.Kind == "file" {
		data, readErr := fsys.ReadFile(source)
		if readErr != nil {
			return readErr
		}
		info, statErr := fsys.Lstat(source)
		if statErr != nil {
			return statErr
		}
		return fsys.WriteAtomic(dest, data, privateFile|(info.Mode().Perm()&executableBits))
	}
	if entry.Kind != "directory" {
		return errChanged()
	}
	if err = fsys.MkdirAll(dest, privateDir); err != nil {
		return err
	}
	children, err := fsys.ReadDir(source)
	if err != nil {
		return err
	}
	for _, child := range children {
		if err = copyTree(fsys, filepath.Join(source, child.Name()), filepath.Join(dest, child.Name())); err != nil {
			return err
		}
	}
	return nil
}

// removeStage removes only the transaction's reserved, previously absent tree.
// It never follows links. User destinations and retained backups are not deleted.
func removeStage(fsys platform.FS, path string) error {
	info, err := fsys.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.IsDir() {
		children, readErr := fsys.ReadDir(path)
		if readErr != nil {
			return readErr
		}
		for _, child := range children {
			if err = removeStage(fsys, filepath.Join(path, child.Name())); err != nil {
				return err
			}
		}
	}
	return fsys.Remove(path)
}

func stageBytes(fsys platform.FS, s state.Step, op plan.Operation) error {
	if op.Kind != plan.Materialize {
		return fsys.WriteAtomic(s.Stage, op.Desired.Content, privateFile)
	}
	return copyTree(fsys, s.After.SourcePath, s.Stage)
}

func moveAside(fsys platform.FS, s state.Step, op plan.Operation) error {
	if op.Kind == plan.Backup {
		if err := backupAvailable(fsys, s.Path); err != nil {
			return err
		}
	}
	if s.Backup == "" {
		return nil
	}
	if err := check(fsys, s.Backup, state.Entry{}); err != nil {
		return err
	}
	return fsys.Rename(s.Path, s.Backup)
}
