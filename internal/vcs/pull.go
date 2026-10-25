package vcs

import (
	"context"
	"fmt"
)

// PullOutcome says what [Pull] did.
type PullOutcome int

// Pull outcomes.
const (
	// PullUpdated means the branch was fast-forwarded to its upstream.
	PullUpdated PullOutcome = iota + 1
	// PullUpToDate means the upstream had nothing new. The local branch may
	// still be ahead (Status.Ahead).
	PullUpToDate
	// PullSkippedDirty means the working copy has local changes, so nothing
	// was fetched or changed.
	PullSkippedDirty
	// PullDiverged means local and upstream commits both exist; the branch
	// was left alone because only a merge or rebase could combine them.
	PullDiverged
)

// String returns a short description of the outcome.
func (o PullOutcome) String() string {
	switch o {
	case PullUpdated:
		return "updated"
	case PullUpToDate:
		return "up to date"
	case PullSkippedDirty:
		return "skipped: the working copy has local changes"
	case PullDiverged:
		return "skipped: the branch has diverged from its upstream"
	}
	return "unknown"
}

// PullResult reports a [Pull].
type PullResult struct {
	// Status is the working copy after the pull (before it when skipped).
	Status  Status
	Outcome PullOutcome
}

// Pull fetches and fast-forwards the current branch of dir, the way sync
// updates its working copy. A working copy with local changes, untracked
// files included, is skipped before fetching; a branch that has diverged is
// skipped after fetching. Pull never merges, rebases, forces or discards
// anything. A detached HEAD or a branch without upstream is an error.
func Pull(ctx context.Context, b Backend, dir string) (PullResult, error) {
	before, err := b.Status(ctx, dir)
	if err != nil {
		return PullResult{}, err
	}
	if err = checkTracking(before); err != nil {
		return PullResult{}, err
	}
	if !before.Clean() {
		return PullResult{Outcome: PullSkippedDirty, Status: before}, nil
	}
	if err = b.Fetch(ctx, dir); err != nil {
		return PullResult{}, err
	}
	fetched, err := b.Status(ctx, dir)
	if err != nil {
		return PullResult{}, err
	}
	if outcome, skip := skipAfterFetch(fetched); skip {
		return PullResult{Outcome: outcome, Status: fetched}, nil
	}
	if err = b.FastForward(ctx, dir); err != nil {
		return PullResult{}, err
	}
	after, err := b.Status(ctx, dir)
	if err != nil {
		return PullResult{}, err
	}
	return PullResult{Outcome: PullUpdated, Status: after}, nil
}

// skipAfterFetch decides whether a fetched working copy must be left alone.
// It may have been changed while fetching.
func skipAfterFetch(s Status) (PullOutcome, bool) {
	switch {
	case !s.Clean():
		return PullSkippedDirty, true
	case s.Behind == 0:
		return PullUpToDate, true
	case s.Ahead > 0:
		return PullDiverged, true
	}
	return 0, false
}

func checkTracking(s Status) error {
	switch {
	case s.Detached():
		return fmt.Errorf("pull: %w", ErrDetachedHead)
	case s.Upstream == "":
		return fmt.Errorf("pull %s: %w", s.Branch, ErrNoUpstream)
	}
	return nil
}
