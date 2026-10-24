// Package platform defines the host operations the synchronization core may
// use, so the core never learns which operating system it runs on.
//
// [Host] combines [FS] (materialization: links, atomic writes, permissions,
// locks) and [Env] (environment, kitbag and agent directories, host name,
// opening URLs, terminal detection). Implementations live in subpackages:
//
//   - unix: macOS and Linux, the supported v1 hosts.
//   - windows: stubs that fail with [ErrUnsupported] until M6.
//   - fake: an in-memory host for core tests; core tests never touch the real
//     filesystem.
//
// One contract suite (contract_test.go) runs against unix and fake to keep them
// equivalent. Only cmd/kitbag constructs a concrete host; every other package
// accepts these interfaces.
//
// # Paths
//
// Every path argument must be absolute and clean (filepath.Clean leaves it
// unchanged) in the host's own syntax; anything else fails with fs.ErrInvalid
// before the host is touched. Symbolic links in intermediate components are
// always followed. Whether the final component is followed is documented per
// method: operations that change an entry never follow it.
//
// # Materialization
//
// [FS.WriteAtomic] and [FS.Link] atomically replace an existing file or symbolic
// link at the destination, so readers see either the old or the new entry,
// never a partial one (macOS adds one caveat for links, see [FS.Link]). They
// never replace a directory and never write through a symbolic link. Deciding whether an existing entry may be replaced (for example
// an unmanaged file that needs a backup) is policy and belongs to the core. The
// unix implementation writes a temporary sibling named ".kitbag-*.tmp", syncs
// it, renames it over the destination and syncs the directory; a crash can leave
// such a temporary file behind, never a truncated destination.
//
// Names are compared byte for byte, but default macOS volumes are
// case-insensitive while Linux and the fake are case-sensitive: callers must
// not create names that differ only in case.
//
// # Errors
//
// Errors wrap fs.ErrNotExist, fs.ErrExist, fs.ErrPermission or fs.ErrInvalid
// where those apply, and otherwise one of the sentinels declared here, so callers
// classify them with errors.Is and never with OS error numbers.
//
// See docs/platform.md for the design rationale, including the directory layout
// on macOS.
package platform
