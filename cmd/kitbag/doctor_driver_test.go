//go:build darwin || linux

package main

import (
	"os"
	"runtime"

	"github.com/TomaszJanusz/kitbag/internal/core/state"
)

// doctorLockedDriver is test-only: hold the real state lock while the ordinary
// CLI runs. The e2e HOME guard runs before this entry and no background process
// or test-specific diagnostic behavior is needed.
func doctorLockedDriver(args []string) int {
	host := newHost()
	store, err := state.New(host, host, state.Options{})
	if err != nil {
		return driverError(os.Stderr, err)
	}
	code := 0
	err = store.WithLock(func(*state.Session) error {
		code = run(runtime.GOOS, host, args, os.Stdout, os.Stderr)
		return nil
	})
	if err != nil {
		return driverError(os.Stderr, err)
	}
	return code
}
