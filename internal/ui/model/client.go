package model

import (
	"context"
	"errors"
	"time"
)

// Client is everything the models need from the local API. Implementations
// live outside this package (see internal/ui/apiclient) so models stay pure.
type Client interface {
	Status(ctx context.Context) (StatusSnapshot, error)
	Drift(ctx context.Context) (DriftSnapshot, error)
	// Sync calls progress synchronously with coarse phase names.
	Sync(ctx context.Context, progress func(phase string)) (SyncOutcome, error)
	Settings(ctx context.Context) ([]SettingValue, error)
	SetSetting(ctx context.Context, key, value string) error
	UnsetSetting(ctx context.Context, key string) error
	// Subscribe delivers EventConnected once registered, then lifecycle hints,
	// until ctx ends or the connection fails.
	Subscribe(ctx context.Context, deliver func(Event)) error
}

// Classification mirrors the plan and status drift vocabulary.
type Classification string

// Drift classifications.
const (
	InSync            Classification = "in sync"
	UpdateNeeded      Classification = "update needed"
	LocalDrift        Classification = "local drift"
	Conflict          Classification = "conflict"
	UnmanagedObstacle Classification = "unmanaged obstacle"
)

// DriftCounts counts observed targets per classification.
type DriftCounts struct {
	InSync, UpdateNeeded, LocalDrift, Conflict, UnmanagedObstacle int
}

// LastSync is the most recent recorded sync of this working copy.
type LastSync struct {
	At       time.Time
	ExitCode int
	Changed  int
}

// ToolTooOld reports a repository requiring a newer kitbag.
type ToolTooOld struct {
	Required, Current string
}

// GitState is Git's local knowledge of the working copy and its upstream.
type GitState struct {
	Branch, Commit, Upstream string
	Ahead, Behind            int
	Dirty, UpstreamGone      bool
}

// StatusSnapshot is one status.get result; Daemon reports who answered.
type StatusSnapshot struct {
	LastSync   *LastSync
	ToolTooOld *ToolTooOld
	Repository string
	Warnings   []string
	Git        GitState
	Drift      DriftCounts
	Daemon     bool
}

// ActionKind mirrors the planner's logical operation kinds.
type ActionKind string

// Planned operation kinds.
const (
	ActionRemove      ActionKind = "remove"
	ActionDiscard     ActionKind = "discard"
	ActionBackup      ActionKind = "backup"
	ActionMaterialize ActionKind = "materialize"
	ActionWrite       ActionKind = "write"
	ActionMerge       ActionKind = "merge"
	ActionUnmerge     ActionKind = "unmerge"
	ActionRestore     ActionKind = "restore"
	ActionRecord      ActionKind = "record"
	ActionRelease     ActionKind = "release"
	ActionSkip        ActionKind = "skip"
)

// Action is one planned operation for a drifted target.
type Action struct {
	Kind                            ActionKind
	Warning, BackupPath, SourcePath string
}

// DriftItem is a target other than in sync, with its planned actions.
type DriftItem struct {
	Path           string
	Classification Classification
	Actions        []Action
}

// DriftSnapshot is the drift projection of one plan.get result.
type DriftSnapshot struct {
	Items    []DriftItem
	Warnings []string
	Daemon   bool
}

// SyncOutcome is the shared CLI exit classification and changed count.
type SyncOutcome struct {
	ExitCode, Changed int
}

// SourceKind says which precedence level supplied a setting.
type SourceKind string

// Setting sources, in precedence order.
const (
	SourceFlag        SourceKind = "flag"
	SourceEnvironment SourceKind = "environment"
	SourceFile        SourceKind = "file"
	SourceDefault     SourceKind = "default"
)

// SettingValue is one effective setting; Name is the flag or variable name.
type SettingValue struct {
	Key, Value, Name string
	Source           SourceKind
}

// EventKind identifies a lifecycle hint.
type EventKind string

// Lifecycle hints. EventConnected is synthesized when a subscription starts.
const (
	EventConnected       EventKind = "connected"
	EventSyncStarted     EventKind = "sync.started"
	EventSyncFinished    EventKind = "sync.finished"
	EventDriftDetected   EventKind = "drift.detected"
	EventUpdateAvailable EventKind = "update.available"
)

// Event is a content-free hint; snapshots remain authoritative.
type Event struct {
	Kind              EventKind
	ExitCode, Changed int
}

// ErrorKind classifies failures for presentation.
type ErrorKind string

// Failure classes.
const (
	ErrOperation     ErrorKind = "operation"
	ErrUnavailable   ErrorKind = "unavailable"
	ErrBusy          ErrorKind = "busy"
	ErrInvalid       ErrorKind = "invalid"
	ErrConflict      ErrorKind = "conflict"
	ErrToolTooOld    ErrorKind = "tool_too_old"
	ErrUnauthorized  ErrorKind = "unauthorized"
	ErrIncompatible  ErrorKind = "incompatible"
	ErrUnsupportedOS ErrorKind = "unsupported_os"
	ErrCanceled      ErrorKind = "canceled"
)

// Error is a classified client failure. Detail is untranslated diagnostic
// text from core, shown only as secondary information.
type Error struct {
	Err      error
	Kind     ErrorKind
	Detail   string
	ExitCode int
}

// Error returns the diagnostic detail.
func (e *Error) Error() string { return e.Detail }

// Unwrap exposes the cause.
func (e *Error) Unwrap() error { return e.Err }

// Classify returns err's *Error, treating cancellation and unclassified errors
// explicitly. It returns nil for nil.
func Classify(err error) *Error {
	if err == nil {
		return nil
	}
	var classified *Error
	if errors.As(err, &classified) {
		return classified
	}
	if errors.Is(err, context.Canceled) {
		return &Error{Kind: ErrCanceled, Detail: err.Error(), Err: err}
	}
	return &Error{Kind: ErrOperation, Detail: err.Error(), Err: err, ExitCode: 1}
}
