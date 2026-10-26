package apply

import (
	"errors"
	"io/fs"

	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// Run is the internal orchestration entry for the future sync command: lock,
// recover, load, freshly plan, then execute. Version/manifest validation and host
// resolution remain the caller's responsibility. It produces no console output.
func Run(fsys platform.FS, store *state.Store, req plan.Request, options Options) (Result, error) {
	result := Result{}
	if options.Now == nil || options.Now().IsZero() {
		return result, errUsage()
	}
	mode, err := ResolveMode("", options.Mode, "", "", "")
	if err != nil {
		return result, err
	}
	err = store.WithLock(func(session *state.Session) error {
		var runErr error
		result, runErr = runLocked(fsys, session, req, options, mode)
		return runErr
	})
	return result, err
}

func runLocked(fsys platform.FS, session *state.Session, req plan.Request, options Options, mode string) (Result, error) {
	if err := Recover(fsys, session); err != nil {
		return Result{}, err
	}
	db, err := session.Load()
	if err != nil {
		return Result{}, err
	}
	req.State = db
	req.Force = options.Force
	req.Mode = mode
	p, err := plan.Build(fsys, req)
	if err != nil {
		return Result{}, err
	}
	return Execute(fsys, session, p, options)
}

// ErrorCode classifies failures; combine with Result.Code by taking the larger
// code. Operational errors and unavailable host operations use Warnings.
func ErrorCode(err error) Code {
	if err == nil {
		return OK
	}
	var versionError *manifest.VersionError
	var schemaError *manifest.SchemaError
	var osError *platform.UnsupportedOSError
	switch {
	case errors.As(err, &osError):
		return UnsupportedOS
	case errors.As(err, &versionError), errors.As(err, &schemaError):
		return ToolTooOld
	case errors.Is(err, errChanged()):
		return Conflict
	case errors.Is(err, fs.ErrInvalid):
		return Usage
	default:
		return Warnings
	}
}
