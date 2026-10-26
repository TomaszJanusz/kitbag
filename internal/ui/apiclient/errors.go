package apiclient

import (
	"context"
	"errors"

	"github.com/TomaszJanusz/kitbag/internal/api"
	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/ui/model"
)

// classify wraps err for presentation, preserving it for errors.Is/As.
func classify(err error) error {
	if err == nil {
		return nil
	}
	code := apply.ErrorCode(err)
	detail := err.Error()
	var remote *api.Error
	if errors.As(err, &remote) {
		code = remote.ExitCode()
		if remote.Data.Detail != "" {
			// The server's explanation of a refused change, without the
			// generic "operation failed" the kind already conveys.
			detail = remote.Data.Detail
		}
	}
	return &model.Error{Err: err, Detail: detail, ExitCode: int(code), Kind: kind(err, remote, code)}
}

func kind(err error, remote *api.Error, code apply.Code) model.ErrorKind {
	switch {
	case errors.Is(err, context.Canceled):
		return model.ErrCanceled
	case errors.Is(err, platform.ErrNoDaemon):
		return model.ErrUnavailable
	case errors.Is(err, platform.ErrLocked):
		return model.ErrBusy
	}
	if remote != nil {
		switch remote.Code {
		case api.Busy:
			return model.ErrBusy
		case api.Unauthorized:
			return model.ErrUnauthorized
		case api.UnsupportedVersion, api.MethodNotFound:
			return model.ErrIncompatible
		}
	}
	return codeKind(code)
}

func codeKind(code apply.Code) model.ErrorKind {
	switch code {
	case apply.Usage:
		return model.ErrInvalid
	case apply.Conflict:
		return model.ErrConflict
	case apply.ToolTooOld:
		return model.ErrToolTooOld
	case apply.UnsupportedOS:
		return model.ErrUnsupportedOS
	case apply.OK, apply.Warnings:
		return model.ErrOperation
	default:
		return model.ErrOperation
	}
}
