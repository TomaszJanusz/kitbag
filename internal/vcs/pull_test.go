package vcs_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

// scriptedBackend returns queued statuses and records calls.
type scriptedBackend struct {
	vcs.Backend
	fetchErr error
	statuses []vcs.Status
	calls    []string
}

func (b *scriptedBackend) Status(context.Context, string) (vcs.Status, error) {
	b.calls = append(b.calls, "status")
	if len(b.statuses) == 0 {
		return vcs.Status{}, errors.New("unexpected status call")
	}
	s := b.statuses[0]
	if len(b.statuses) > 1 {
		b.statuses = b.statuses[1:]
	}
	return s, nil
}

func (b *scriptedBackend) Fetch(context.Context, string) error {
	b.calls = append(b.calls, "fetch")
	return b.fetchErr
}

func (b *scriptedBackend) FastForward(context.Context, string) error {
	b.calls = append(b.calls, "fast-forward")
	return nil
}

func TestPull(t *testing.T) {
	t.Parallel()

	tracking := vcs.Status{Head: vcs.Head{Branch: "main", Commit: "c1"}, Upstream: "origin/main"}
	dirty := tracking
	dirty.Changes = []vcs.Change{{Path: "x", Kind: vcs.ChangeUntracked}}
	behind, ahead, diverged := tracking, tracking, tracking
	behind.Behind = 2
	ahead.Ahead = 1
	diverged.Ahead, diverged.Behind = 1, 1
	dirtyBehind := behind
	dirtyBehind.Changes = dirty.Changes

	fetched := []string{"status", "fetch", "status"}
	for _, tt := range []struct {
		name      string
		statuses  []vcs.Status
		wantCalls []string
		want      vcs.PullOutcome
	}{
		{"dirty skips before fetching", []vcs.Status{dirty}, []string{"status"}, vcs.PullSkippedDirty},
		{"up to date", []vcs.Status{tracking, tracking}, fetched, vcs.PullUpToDate},
		{"only ahead", []vcs.Status{tracking, ahead}, fetched, vcs.PullUpToDate},
		{"diverged", []vcs.Status{tracking, diverged}, fetched, vcs.PullDiverged},
		{"dirtied while fetching", []vcs.Status{tracking, dirtyBehind}, fetched, vcs.PullSkippedDirty},
		{
			"behind",
			[]vcs.Status{tracking, behind, tracking},
			[]string{"status", "fetch", "status", "fast-forward", "status"},
			vcs.PullUpdated,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := &scriptedBackend{statuses: tt.statuses}
			res, err := vcs.Pull(t.Context(), b, "/repo")
			if err != nil || res.Outcome != tt.want {
				t.Fatalf("Pull = %v, %v; want %v", res.Outcome, err, tt.want)
			}
			if !slices.Equal(b.calls, tt.wantCalls) {
				t.Errorf("calls = %q, want %q", b.calls, tt.wantCalls)
			}
		})
	}
}

func TestPullErrors(t *testing.T) {
	t.Parallel()

	detached := vcs.Status{Head: vcs.Head{Commit: "c1"}}
	noUpstream := vcs.Status{Head: vcs.Head{Branch: "main", Commit: "c1"}}
	for _, tt := range []struct {
		want   error
		status vcs.Status
	}{
		{vcs.ErrDetachedHead, detached},
		{vcs.ErrNoUpstream, noUpstream},
	} {
		b := &scriptedBackend{statuses: []vcs.Status{tt.status}}
		if _, err := vcs.Pull(t.Context(), b, "/repo"); !errors.Is(err, tt.want) {
			t.Errorf("Pull(%+v) error = %v, want %v", tt.status, err, tt.want)
		}
		if slices.Contains(b.calls, "fetch") {
			t.Errorf("Pull(%+v) fetched", tt.status)
		}
	}

	failing := &scriptedBackend{
		statuses: []vcs.Status{{Head: vcs.Head{Branch: "main"}, Upstream: "origin/main"}},
		fetchErr: errors.New("offline"),
	}
	if _, err := vcs.Pull(t.Context(), failing, "/repo"); !errors.Is(err, failing.fetchErr) {
		t.Errorf("fetch error = %v, want it returned", err)
	}
}

func TestOutcomeAndKindStrings(t *testing.T) {
	t.Parallel()

	for _, o := range []vcs.PullOutcome{vcs.PullUpdated, vcs.PullUpToDate, vcs.PullSkippedDirty, vcs.PullDiverged} {
		if o.String() == "unknown" {
			t.Errorf("outcome %d has no description", o)
		}
	}
	for k := vcs.ChangeModified; k <= vcs.ChangeUntracked; k++ {
		if k.String() == "unknown" {
			t.Errorf("change kind %d has no name", k)
		}
	}
}
