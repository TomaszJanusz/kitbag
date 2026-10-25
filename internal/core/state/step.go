package state

import (
	"path/filepath"
	"slices"
	"strings"
)

// AsideDirectory names the directory that holds real skill directories moved
// out of the way by an explicit adoption. It is the only backup location that
// is not a reserved sibling of its destination.
const AsideDirectory = "skills-before-kitbag"

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
	valid := false
	switch s.Action {
	case "output":
		valid = s.After != nil && s.Stage != "" && noRestore(s)
	case "remove", "backup":
		valid = s.After == nil && s.Stage == "" && noRestore(s) && !s.Release
	case "restore":
		valid = validRestore(s)
	}
	if !valid {
		return invalid("pending step action or its metadata")
	}
	return validateAside(s)
}

func noRestore(s *Step) bool { return s.Restored == nil && s.From == "" }

// validRestore requires the restored entry, and a backup to rename exactly
// when that entry is not a link (which is recreated from its text).
func validRestore(s *Step) bool {
	r := s.Restored
	return r != nil && s.Stage == "" && !s.Release && validEntry(*r) && r.Kind != "" && (s.From == "") == (r.Kind == "link")
}

// validateAside requires exactly one place for an existing entry: a backup,
// or (never for a backup step) a discard sibling.
func validateAside(s *Step) error {
	asides := 0
	for _, aside := range []string{s.Backup, s.Discard} {
		if aside != "" {
			asides++
		}
	}
	switch {
	case s.Action == "backup" && s.Discard != "":
		return invalid("pending backup cannot discard")
	case (s.Before.Kind != "") != (asides == 1) || asides > 1:
		return invalid("pending backup missing or ambiguous")
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
	// A link is journaled by its exact text, so discarding an unowned one loses nothing.
	if s.Discard != "" && s.Previous == nil && s.Before.Kind != "link" {
		return invalid("pending discard of unowned entry")
	}
	return nil
}

func validateStepPaths(s *Step) error {
	for _, item := range []struct{ path, prefix string }{{s.Stage, s.Path + ".kitbag-stage."}, {s.Discard, s.Path + ".kitbag-stage."}, {s.Backup, s.Path + ".before-kitbag."}} {
		if item.path == "" {
			continue
		}
		if err := validPath(item.path); err != nil {
			return err
		}
		if !reservedSibling(item.path, s.Path, item.prefix) && (item.path != s.Backup || !aside(s)) {
			return invalid("pending path must be a reserved sibling")
		}
	}
	if s.Stage != "" && s.Stage == s.Discard {
		return invalid("pending stage and discard overlap")
	}
	if s.From != "" {
		return validPath(s.From)
	}
	return nil
}

// aside permits only an explicit backup into the adoption directory.
func aside(s *Step) bool {
	return s.Action == "backup" && slices.Contains(strings.Split(filepath.ToSlash(filepath.Dir(s.Backup)), "/"), AsideDirectory)
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
