// Package state persists machine-local ownership and revert journals. It never
// resolves sources, stores credentials or file contents, or applies a plan.
package state

import "time"

// CurrentVersion is the on-disk schema understood by this release.
const CurrentVersion = 1

// DefaultHistoryLimit bounds retained applied plans when no limit is configured.
const DefaultHistoryLimit = 10

// Kind describes how a destination was materialized.
type Kind string

// Supported materialization kinds.
const (
	Link      Kind = "link"
	Copy      Kind = "copy"
	Generated Kind = "generated"
	Merged    Kind = "merged"
)

// Owner identifies a primitive on one agent instance. Shared targets have
// multiple owners; these are local identities, not source provenance.
type Owner struct {
	Primitive     string `json:"primitive"`
	AgentInstance string `json:"agent_instance"`
}

// Target records the last successful materialization. SourcePath and SourceHash
// describe local input bytes, never a resolved remote pin. Generated or merged
// targets may omit both when no single source exists.
type Target struct {
	Path        string  `json:"path"`
	Kind        Kind    `json:"kind"`
	SourcePath  string  `json:"source_path,omitempty"`
	SourceHash  string  `json:"source_hash,omitempty"`
	WrittenHash string  `json:"written_hash"`
	Owners      []Owner `json:"owners"`
}

// Previous describes the entry before a change. File bytes live in BackupPath,
// outside this database; LinkTarget preserves a link's exact, possibly relative,
// text. Managed is nil for an unmanaged entry, otherwise its prior ownership.
type Previous struct {
	Managed     *Target `json:"managed,omitempty"`
	Kind        Kind    `json:"kind"`
	WrittenHash string  `json:"written_hash"`
	LinkTarget  string  `json:"link_target,omitempty"`
	BackupPath  string  `json:"backup_path,omitempty"`
}

// Change journals a destination once per plan. A nil Previous means it did not
// exist; a non-nil Previous contains enough metadata to restore its prior entry.
type Change struct {
	Previous *Previous `json:"previous"`
	Path     string    `json:"path"`
}

// AppliedPlan is a completed plan's revert journal. Changes are independent
// destination records, not ordered apply instructions. History is oldest first.
type AppliedPlan struct {
	ID        string    `json:"id"`
	AppliedAt time.Time `json:"applied_at"`
	Changes   []Change  `json:"changes"`
}

// Database is one atomic snapshot. Empty collections serialize as arrays.
type Database struct {
	Pending       *Step         `json:"pending,omitempty"`
	Targets       []Target      `json:"targets"`
	History       []AppliedPlan `json:"history"`
	SchemaVersion int           `json:"schema_version"`
}

// Migration upgrades explicitly versioned N-1 JSON to CurrentVersion in memory.
// Load never overwrites the original; only an explicit Save persists the result.
// The returned document is decoded and validated like any current snapshot.
type Migration interface {
	Migrate(previous []byte) ([]byte, error)
}

// Options configures a Store. A zero HistoryLimit selects DefaultHistoryLimit;
// negative limits are invalid. Migration is optional; no legacy format is implied.
type Options struct {
	Migration    Migration
	HistoryLimit int
}

// Entry is a lexical filesystem precondition, never following a final link.
// Kind is empty for absence, or file, directory, or link.
type Entry struct {
	Kind string `json:"kind"`
	Hash string `json:"hash,omitempty"`
	Link string `json:"link,omitempty"`
}

// Step is write-ahead recovery metadata for one authorized filesystem step.
// Stage is a reserved sibling owned by this transaction. Backup retains the old
// entry. Recovery either recognizes the complete output or restores the old one.
type Step struct {
	At       time.Time `json:"at"`
	After    *Target   `json:"after,omitempty"`
	Previous *Target   `json:"previous,omitempty"`
	Before   Entry     `json:"before"`
	Path     string    `json:"path"`
	Stage    string    `json:"stage,omitempty"`
	Backup   string    `json:"backup,omitempty"`
	Action   string    `json:"action"`
	ID       string    `json:"id"`
}
