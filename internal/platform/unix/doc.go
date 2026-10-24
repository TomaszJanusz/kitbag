//go:build darwin || linux

// Package unix implements platform.Host for macOS and Linux, the v1 hosts.
//
// Links are symbolic links. WriteAtomic and Link create a ".kitbag-*.tmp"
// sibling, sync it, rename it over the destination and then sync the parent
// directory; Rename and Remove also sync the affected directories. Locks are
// flock(2) locks, which the kernel releases when the process exits, so a crash
// never leaves a stale lock behind.
//
// Both systems use the XDG Base Directory layout (~/.config/kitbag,
// ~/.local/share/kitbag, ~/.local/state/kitbag and ~/.cache/kitbag, honoring
// absolute XDG_*_HOME values). macOS deliberately does not use
// ~/Library/Application Support; docs/platform.md records the rationale.
//
// Other Unix systems are not v1 targets and are excluded by the build
// constraint rather than compiled without verification.
package unix
