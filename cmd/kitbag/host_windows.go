//go:build windows

package main

import (
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/platform/windows"
)

// newHost returns the Windows stub host; run refuses Windows before using it.
func newHost() platform.Host {
	return windows.New()
}
