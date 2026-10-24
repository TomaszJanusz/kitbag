// Package vcs is the single boundary through which kitbag uses Git.
//
// [Backend] lists the operations synchronization needs; callers never run Git
// themselves, so the system backend (internal/vcs/system) and the embedded
// backend (go-git, not implemented yet) are interchangeable. The package also
// holds backend selection ([Select], [Open]), the "pull only when clean" policy
// ([Pull]) and the working copy location ([WorkingCopyDir]). It never starts
// processes; see docs/vcs.md for the selection rules and messages.
package vcs
