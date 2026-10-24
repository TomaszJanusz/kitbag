package vcs

import (
	"context"
	"errors"
)

// DefaultRemote is the remote a clone creates and that [Backend.Fetch] and
// [Backend.Push] use.
const DefaultRemote = "origin"

// Classification sentinels returned (wrapped) by every backend.
var (
	// ErrNotFastForward reports that the local branch and its upstream have
	// diverged, so updating would need a merge or rebase, which kitbag never
	// does on its own.
	ErrNotFastForward = errors.New("not a fast-forward")
	// ErrPushRejected reports that the remote refused a push, for example
	// because the remote branch has commits the local branch lacks.
	ErrPushRejected = errors.New("push rejected by the remote")
	// ErrNoUpstream reports a branch without a configured upstream branch.
	ErrNoUpstream = errors.New("no upstream branch")
	// ErrDetachedHead reports an operation that needs a current branch while
	// HEAD is detached.
	ErrDetachedHead = errors.New("HEAD is detached")
	// ErrNothingToCommit reports a commit without any change to record.
	ErrNothingToCommit = errors.New("nothing to commit")
	// ErrNoRemote reports a remote that is not configured.
	ErrNoRemote = errors.New("no such remote")
	// ErrInvalidArgument reports a path, revision or name a backend refuses
	// before running Git, such as a relative directory or a revision that
	// looks like an option.
	ErrInvalidArgument = errors.New("invalid argument")
)

// Backend is a Git implementation. Every dir argument is the absolute path of
// a working copy's top-level directory; backends never search parent
// directories for a repository. Implementations are safe for concurrent use
// on different working copies.
type Backend interface {
	// Kind reports which implementation this is.
	Kind() Kind
	// Version describes the implementation, such as "git 2.43.0".
	Version() string

	// Clone clones url into dir, which must not exist or be empty, and
	// checks out opts.Branch or the remote's default branch.
	Clone(ctx context.Context, url, dir string, opts CloneOptions) error
	// Fetch updates the remote-tracking branches of [DefaultRemote]. It never
	// changes the working tree or local branches.
	Fetch(ctx context.Context, dir string) error
	// FastForward moves the current branch to its upstream when that is a
	// fast-forward, and fails with [ErrNotFastForward] otherwise. It never
	// merges, rebases or discards local changes. Callers that must skip a
	// dirty working copy use [Pull].
	FastForward(ctx context.Context, dir string) error
	// Status reports the current branch, its relation to the upstream and
	// every local change, including untracked files but not ignored ones.
	Status(ctx context.Context, dir string) (Status, error)
	// Diff lists the files that differ between the trees of two revisions,
	// without rename detection.
	Diff(ctx context.Context, dir, from, to string) ([]Change, error)
	// Commit records every change in the working tree, including new and
	// deleted files, and returns the new commit. A clean working tree fails
	// with [ErrNothingToCommit].
	Commit(ctx context.Context, dir string, opts CommitOptions) (string, error)
	// Push sends a local branch to the branch of the same name on
	// [DefaultRemote]. It never forces; a rejection fails with
	// [ErrPushRejected].
	Push(ctx context.Context, dir string, opts PushOptions) error
	// CreateBranch creates branch name at start (HEAD when empty) without
	// checking it out. An existing branch is an error.
	CreateBranch(ctx context.Context, dir, name, start string) error
	// DeleteBranch deletes a local branch that is fully merged. The current
	// branch and unmerged branches are errors.
	DeleteBranch(ctx context.Context, dir, name string) error
	// RemoteURL returns the configured URL of remote, verbatim; an unknown
	// remote fails with [ErrNoRemote].
	RemoteURL(ctx context.Context, dir, remote string) (string, error)
	// Head reports the current branch and commit.
	Head(ctx context.Context, dir string) (Head, error)
}

// CloneOptions configure [Backend.Clone].
type CloneOptions struct {
	// Branch is the branch to check out; empty selects the remote's default.
	Branch string
}

// CommitOptions configure [Backend.Commit].
type CommitOptions struct {
	// Message is the commit message; it must not be empty.
	Message string
}

// PushOptions configure [Backend.Push].
type PushOptions struct {
	// Branch is the local branch to push; empty selects the current branch.
	Branch string
	// SetUpstream records the pushed branch as the local branch's upstream.
	SetUpstream bool
}

// Head is the state of HEAD.
type Head struct {
	// Branch is the current branch, or "" when HEAD is detached.
	Branch string
	// Commit is the full object name of the current commit, or "" when the
	// branch is unborn (a new or empty repository).
	Commit string
}

// Detached reports whether HEAD points at a commit rather than a branch.
func (h Head) Detached() bool {
	return h.Branch == ""
}

// Unborn reports whether the current branch has no commit yet.
func (h Head) Unborn() bool {
	return h.Commit == ""
}

// Status is the state of a working copy.
type Status struct {
	Head
	// Upstream is the upstream branch, such as "origin/main", or "" when none
	// is configured.
	Upstream string
	// Changes lists local changes in the index and the working tree.
	Changes []Change
	// Ahead and Behind count the commits only on the local branch and only on
	// the upstream branch.
	Ahead, Behind int
	// UpstreamGone reports a configured upstream whose remote-tracking branch
	// does not exist, for example because the remote branch was deleted or
	// the remote is still empty.
	UpstreamGone bool
}

// Clean reports whether the working copy has no local changes. Untracked
// files count as changes: a fast-forward could overwrite them.
func (s Status) Clean() bool {
	return len(s.Changes) == 0
}

// Change is one changed path.
type Change struct {
	// Path is slash-separated and relative to the working copy, verbatim
	// (no quoting), so it may contain spaces, newlines or any Unicode.
	Path string
	// OrigPath is the source path of a rename or copy, otherwise "".
	OrigPath string
	// Kind classifies the change.
	Kind ChangeKind
}

// ChangeKind classifies a [Change].
type ChangeKind int

// Change kinds. [Backend.Diff] reports only added, modified, deleted and
// type-changed paths.
const (
	// ChangeModified is a changed file.
	ChangeModified ChangeKind = iota + 1
	// ChangeAdded is a new file in the index or the second tree.
	ChangeAdded
	// ChangeDeleted is a removed file.
	ChangeDeleted
	// ChangeTypeChanged is a path that changed kind, such as file to link.
	ChangeTypeChanged
	// ChangeRenamed is a path renamed from OrigPath.
	ChangeRenamed
	// ChangeCopied is a path copied from OrigPath.
	ChangeCopied
	// ChangeUnmerged is a path with an unresolved conflict.
	ChangeUnmerged
	// ChangeUntracked is a file Git does not track and does not ignore.
	ChangeUntracked
)

// String returns the lower-case name of the kind.
func (k ChangeKind) String() string {
	switch k {
	case ChangeModified:
		return "modified"
	case ChangeAdded:
		return "added"
	case ChangeDeleted:
		return "deleted"
	case ChangeTypeChanged:
		return "type changed"
	case ChangeRenamed:
		return "renamed"
	case ChangeCopied:
		return "copied"
	case ChangeUnmerged:
		return "unmerged"
	case ChangeUntracked:
		return "untracked"
	}
	return "unknown"
}
