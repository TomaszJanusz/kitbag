//go:build !darwin && !linux

package platform

import (
	"context"
	"io"
)

// RunEditor is unavailable on platforms without runtime support.
func RunEditor(_ context.Context, _, _ string, _ io.Reader, _, _ io.Writer) error {
	return ErrUnsupported
}
