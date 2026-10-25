//go:build windows

package platform

import "context"

// ShutdownContext is unused on unsupported Windows hosts.
func ShutdownContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithCancel(ctx)
}
