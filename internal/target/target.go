// Package target defines declarations shared by adapters and the plan engine.
// Adapters describe intent; core owns collisions, drift, and operation ordering.
package target

import (
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// Kind is a logical output, independent of link or copy execution.
type Kind string

// Supported logical outputs.
const (
	Materialized Kind = "materialized"
	Generated    Kind = "generated"
	Merged       Kind = "merged"
)

// BackupPolicy controls replacement of an unmanaged regular file.
type BackupPolicy string

// Supported backup policies. The zero value refuses replacement.
const (
	Refuse BackupPolicy = ""
	Once   BackupPolicy = "once"
)

// Merger is a deterministic native-format merge. It must preserve unowned keys,
// reject unsafe input, and return fresh bytes without modifying either argument.
// The planner supplies nil current bytes for a missing destination.
type Merger interface {
	Merge(current, fragment []byte) ([]byte, error)
}

// Target declares an absolute, clean destination and its owners. Materialized
// entries require SourcePath inside the repository; generated entries use Content.
// Merged entries use either a source fragment or Content and require Merger.
// Warning declares an unsupported entry, protecting any previous ownership.
// No materialization mode belongs here: KIT-16 selects link or copy at execution.
type Target struct {
	Merger     Merger
	Path       string
	SourcePath string
	Kind       Kind
	Backup     BackupPolicy
	Warning    string
	Content    []byte
	Owners     []state.Owner
}

// Adapter declares all its enabled targets from an already validated and
// host-resolved manifest. It must only read through fsys, never mutate inputs,
// and must not consult clocks, randomness, environment, or other host APIs.
// Callers supply the complete adapter set, including files/git declarations.
type Adapter interface {
	Declare(fsys manifest.FileSystem, repository string, intent *manifest.Manifest) ([]Target, error)
}
