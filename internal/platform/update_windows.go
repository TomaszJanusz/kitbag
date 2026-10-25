//go:build windows

package platform

import "context"

// Executable is unavailable before Windows runtime support.
func Executable() (string, error) { return "", ErrUnsupported }

// ProbeExecutable is unavailable before Windows runtime support.
func ProbeExecutable(context.Context, string) (string, error) { return "", ErrUnsupported }

// Reexec is unavailable before Windows runtime support.
func Reexec(context.Context, string, []string) error { return ErrUnsupported }
