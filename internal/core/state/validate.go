package state

import (
	"encoding/hex"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

func invalid(message string) error {
	return fmt.Errorf("invalid state: %s: %w", message, fs.ErrInvalid)
}

func validPath(path string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsRune(path, 0) {
		return invalid("path must be absolute and clean: " + path)
	}
	return nil
}

func validHash(hash string) bool {
	const digestLength = 64
	digest, ok := strings.CutPrefix(hash, "sha256:")
	if !ok || len(digest) != digestLength || strings.ToLower(digest) != digest {
		return false
	}
	_, err := hex.DecodeString(digest)
	return err == nil
}

func validKind(kind Kind) bool {
	switch kind {
	case Link, Copy, Generated, Merged:
		return true
	default:
		return false
	}
}

func validateTarget(target Target) error {
	if err := validPath(target.Path); err != nil {
		return err
	}
	if !validKind(target.Kind) || !validHash(target.WrittenHash) {
		return invalid("target kind or written hash")
	}
	if err := validateSource(target); err != nil {
		return err
	}
	return validateOwners(target.Owners)
}

func validateSource(target Target) error {
	if target.SourcePath != "" {
		if err := validPath(target.SourcePath); err != nil {
			return err
		}
		if !validHash(target.SourceHash) {
			return invalid("source hash")
		}
	} else if target.SourceHash != "" || target.Kind == Copy || target.Kind == Link {
		return invalid("missing source path")
	}
	return nil
}

func validateOwners(owners []Owner) error {
	if len(owners) == 0 {
		return invalid("missing owners")
	}
	seen := make(map[Owner]bool)
	for _, owner := range owners {
		if owner.Primitive == "" || owner.AgentInstance == "" || seen[owner] {
			return invalid("empty or duplicate owner")
		}
		seen[owner] = true
	}
	return nil
}

func validate(db Database) error {
	if db.SchemaVersion != CurrentVersion {
		return invalid("schema_version must match current version")
	}
	paths := make(map[string]bool)
	for _, target := range db.Targets {
		if paths[target.Path] {
			return invalid("duplicate target path")
		}
		paths[target.Path] = true
		if err := validateTarget(target); err != nil {
			return err
		}
	}
	ids := make(map[string]bool)
	for _, plan := range db.History {
		if plan.ID == "" || ids[plan.ID] || plan.AppliedAt.IsZero() {
			return invalid("empty/duplicate plan ID or missing applied_at")
		}
		ids[plan.ID] = true
		if err := validateChanges(plan.Changes); err != nil {
			return err
		}
	}
	return validateStep(db.Pending)
}

func validateChanges(changes []Change) error {
	paths := make(map[string]bool)
	for _, change := range changes {
		if err := validPath(change.Path); err != nil {
			return err
		}
		if paths[change.Path] {
			return invalid("duplicate journal path")
		}
		paths[change.Path] = true
		if change.Previous == nil {
			continue
		}
		if err := validatePrevious(change); err != nil {
			return err
		}
	}
	return nil
}

func validatePrevious(change Change) error {
	prev := change.Previous
	if !validKind(prev.Kind) || !validHash(prev.WrittenHash) {
		return invalid("previous kind or hash")
	}
	if err := validateRestore(*prev); err != nil {
		return err
	}
	if prev.Managed != nil {
		if err := validateTarget(*prev.Managed); err != nil {
			return err
		}
		if prev.Managed.Path != change.Path || prev.Managed.Kind != prev.Kind || prev.Managed.WrittenHash != prev.WrittenHash {
			return invalid("previous ownership disagrees with journal entry")
		}
	}
	return nil
}

func validateRestore(prev Previous) error {
	if prev.Kind == Link {
		if prev.LinkTarget == "" || strings.ContainsRune(prev.LinkTarget, 0) || prev.BackupPath != "" {
			return invalid("previous link requires link target only")
		}
		return nil
	}
	if prev.LinkTarget != "" {
		return invalid("file cannot have link target")
	}
	return validPath(prev.BackupPath)
}
