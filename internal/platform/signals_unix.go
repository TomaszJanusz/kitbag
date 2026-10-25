//go:build darwin || linux

package platform

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// ShutdownContext cancels on SIGINT or SIGTERM. Call stop to release registration.
func ShutdownContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
}
