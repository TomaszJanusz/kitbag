//go:build windows

// Package windows is the Windows platform until the M6 compatibility layer:
// every operation fails with platform.ErrUnsupported naming the operation, so
// the binary compiles from M1 and reports unsupported work explicitly.
// Compilation alone is not a claim of Windows support (ADR 3); cmd/kitbag
// refuses to run on Windows through platform.CheckOS.
package windows
