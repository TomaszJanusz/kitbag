// Package fake implements platform.Host in memory for core tests, which must
// never touch the real filesystem or environment.
//
// The fake follows the unix contract, verified by the shared contract suite:
// symbolic links (absolute or relative, followed through intermediate
// components), atomic replacement, owner permission bits enforced as for an
// unprivileged user, sorted directory listings, exclusive locks and the XDG
// directory layout. Errors match the same sentinels as the unix host. Every
// method is safe for concurrent use.
//
// Deliberate simplifications: there is one user and no umask, names are
// case-sensitive (as on Linux, unlike default macOS volumes), modification
// times are zero, directory sizes are zero and FileInfo.Sys returns nil.
// [Host.Mount] simulates a second filesystem for cross-device renames, and
// [Host.Opened] records targets passed to Open instead of launching anything.
package fake
