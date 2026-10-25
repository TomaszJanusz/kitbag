package state

import (
	"path/filepath"
	"strings"
)

func validateStep(s *Step) error {
	if s == nil {
		return nil
	}
	if err := validPath(s.Path); err != nil {
		return err
	}
	if s.ID == "" || s.At.IsZero() {
		return invalid("pending step ID or time")
	}
	if err := validateStepAction(s); err != nil {
		return err
	}
	if err := validateEntry(s.Before); err != nil {
		return err
	}
	if err := validateStepPaths(s); err != nil {
		return err
	}
	return validateStepTargets(s)
}

func validateStepAction(s *Step) error {
	switch s.Action {
	case "output":
		if s.After == nil || s.Stage == "" {
			return invalid("pending output missing target/stage")
		}
	case "remove", "backup":
		if s.After != nil || s.Stage != "" {
			return invalid("pending removal/backup has output")
		}
	default:
		return invalid("pending step action")
	}
	if s.Before.Kind != "" && s.Backup == "" {
		return invalid("pending backup missing")
	}
	return nil
}

func validateStepTargets(s *Step) error {
	for _, t := range []*Target{s.After, s.Previous} {
		if t == nil {
			continue
		}
		if err := validateTarget(*t); err != nil {
			return err
		}
		if t.Path != s.Path {
			return invalid("pending target path disagrees")
		}
	}
	return nil
}

func validateStepPaths(s *Step) error {
	for _, item := range []struct{ path, prefix string }{{s.Stage, s.Path + ".kitbag-stage."}, {s.Backup, s.Path + ".before-kitbag."}} {
		if item.path == "" {
			continue
		}
		if err := validPath(item.path); err != nil {
			return err
		}
		if !reservedSibling(item.path, s.Path, item.prefix) {
			return invalid("pending path must be a reserved sibling")
		}
	}
	return nil
}

func reservedSibling(path, destination, prefix string) bool {
	return filepath.Dir(path) == filepath.Dir(destination) && strings.HasPrefix(path, prefix) && len(path) > len(prefix)
}

func validateEntry(e Entry) error {
	if !validEntry(e) {
		return invalid("pending entry metadata")
	}
	return nil
}

func validEntry(e Entry) bool {
	switch e.Kind {
	case "":
		return e.Hash == "" && e.Link == ""
	case "file", "directory":
		return validHash(e.Hash) && e.Link == ""
	case "link":
		return e.Link != "" && !strings.ContainsRune(e.Link, 0) && e.Hash == ""
	default:
		return false
	}
}
