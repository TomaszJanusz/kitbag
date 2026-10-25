//go:build darwin || linux

package main

import (
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/platform/unix"
)

// newHost returns the real macOS and Linux host.
func newHost() platform.Host {
	return unix.New()
}
