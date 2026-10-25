// Package plan builds deterministic logical synchronization plans through
// platform.FS. It never mutates the host or its inputs and does not execute plans.
package plan

import (
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/target"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// SchemaVersion versions JSON plans independently of state and manifest schemas.
const SchemaVersion = 1

// Classification is the shared plan/status drift vocabulary.
type Classification string

// Three-way comparison outcomes.
const (
	InSync            Classification = "in sync"
	UpdateNeeded      Classification = "update needed"
	LocalDrift        Classification = "local drift"
	Conflict          Classification = "conflict"
	UnmanagedObstacle Classification = "unmanaged obstacle"
)

// Kind describes a logical operation. Materialize creates or replaces a source
// file/tree as a link or copy; Record updates only ownership/hash metadata.
type Kind string

// Supported operations, in execution phase order. Discard, Restore, Unmerge and
// Release only occur in unmanage/revert plans: Discard deletes unchanged owned
// output that the repository reproduces, Restore puts a journaled entry back,
// Unmerge writes settings without KitBag's hooks and Release forgets ownership.
const (
	Remove      Kind = "remove"
	Discard     Kind = "discard"
	Backup      Kind = "backup"
	Materialize Kind = "materialize"
	Write       Kind = "write"
	Merge       Kind = "merge"
	Unmerge     Kind = "unmerge"
	Restore     Kind = "restore"
	Record      Kind = "record"
	Release     Kind = "release"
	Skip        Kind = "skip"
)

// Restoration is the journaled entry a Restore operation puts back: a backup
// renamed into place or a link recreated verbatim. Owner is the ownership
// recorded afterwards; nil leaves the restored entry unmanaged.
type Restoration struct {
	Owner  *state.Target `json:"-"`
	Backup string        `json:"backup,omitempty"`
	Link   string        `json:"link,omitempty"`
	Entry  state.Entry   `json:"-"`
}

// Request supplies a complete desired world and a validated state snapshot.
// Manifest must be validated and host-resolved before Build. Adapters must obey
// target.Adapter's purity contract. Repository uses host-native absolute syntax.
type Request struct {
	Manifest    *manifest.Manifest
	Repository  string
	Mode        string
	Adapters    []target.Adapter
	LegacyRoots []string
	State       state.Database
	Force       bool
}

// Desired is resolved output metadata. Content is available to the apply
// engine for generated/merged writes, but never appears in either renderer.
// SourceHash describes local bytes, not remote provenance (ADR 9).
type Desired struct {
	InstanceDir string      `json:"-"`
	Mode        string      `json:"-"`
	Path        string      `json:"path"`
	Kind        target.Kind `json:"kind"`
	SourcePath  string      `json:"source_path,omitempty"`
	SourceHash  string      `json:"source_hash,omitempty"`
	WrittenHash string      `json:"written_hash"`
	// LegacyHash is the hash of agent-config-sync's equivalent rendering, if any.
	LegacyHash string `json:"-"`
	// LegacyHashes, with legacy roots configured, also hash that rendering for a
	// clone at each legacy root: such an unowned file is replaced in place.
	LegacyHashes []string      `json:"-"`
	Owners       []state.Owner `json:"owners"`
	Content      []byte        `json:"-"`
}

// Operation is one ordered logical action. A skip never authorizes mutation.
// For Discard, Restore and Unmerge a BackupPath prefix moves a forced, changed
// entry aside to a backup instead of discarding it. Replaces marks an output
// replacing agent-config-sync's reproducible output: apply discards it instead
// of keeping a backup, journaling it as that previously owned output.
// Apply must recheck preconditions under the state lock; a plan is not a lease.
type Operation struct {
	Expected       *state.Entry   `json:"-"`
	Desired        *Desired       `json:"desired,omitempty"`
	Replaces       *state.Target  `json:"-"`
	Restore        *Restoration   `json:"restore,omitempty"`
	Kind           Kind           `json:"kind"`
	Path           string         `json:"path"`
	Classification Classification `json:"classification"`
	BackupPath     string         `json:"backup_path,omitempty"`
	Warning        string         `json:"warning,omitempty"`
}

// Observation feeds status and UI lists, including targets needing no action.
type Observation struct {
	Path           string         `json:"path"`
	Classification Classification `json:"classification"`
}

// Plan contains ordered operations and path-sorted observations. An empty plan
// has zero Operations; it can still contain in-sync observations.
type Plan struct {
	Operations   []Operation   `json:"operations"`
	Observations []Observation `json:"observations"`
	PlanSchema   int           `json:"plan_schema"`
	Force        bool          `json:"-"`
}
