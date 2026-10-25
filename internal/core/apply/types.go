// Package apply executes fresh logical plans under the state lock using only
// platform.FS. It retains backups and persists each step before continuing.
package apply

import (
	"time"

	"github.com/TomaszJanusz/kitbag/internal/core/plan"
)

// Code is the command exit classification shared by all entry points.
type Code int

// Stable CLI exit codes. Warnings also includes operational failures.
const (
	OK Code = iota
	Warnings
	Usage
	Conflict
	ToolTooOld
	UnsupportedOS
)

// Options supplies the machine mode, explicit force authorization and clock.
// Empty Mode defaults to link. Manifest copy always wins over machine link.
type Options struct {
	Now   func() time.Time
	Mode  string
	Force bool
}

// Diagnostic describes a skipped operation; Changed contains successful steps.
type Diagnostic struct {
	Path    string
	Message string
}

// Result is silent when all three lists are empty. Conflicts outrank warnings.
type Result struct {
	Changed   []plan.Operation
	Warnings  []Diagnostic
	Conflicts []Diagnostic
}

// Code returns the report's exit classification; callers map errors to Warnings.
func (r Result) Code() Code {
	if len(r.Conflicts) > 0 {
		return Conflict
	}
	if len(r.Warnings) > 0 {
		return Warnings
	}
	return OK
}

// ResolveMode selects flag, environment, configuration, then platform default.
// The most specific manifest value is separate: copy on either side wins.
func ResolveMode(manifestMode, flag, environment, configuration, platformDefault string) (string, error) {
	machine := platformDefault
	for _, candidate := range []string{configuration, environment, flag} {
		if candidate != "" {
			machine = candidate
		}
	}
	if machine == "" {
		machine = "link"
	}
	if !validMode(machine) || (manifestMode != "" && !validMode(manifestMode)) {
		return "", errUsage()
	}
	if manifestMode == "copy" || machine == "copy" {
		return "copy", nil
	}
	return "link", nil
}

func validMode(mode string) bool { return mode == "link" || mode == "copy" }
