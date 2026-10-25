package workflow

import (
	"context"
	"fmt"
	"io/fs"

	"github.com/TomaszJanusz/kitbag/internal/core/compatibility"
	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

func invalidInput() error { return fs.ErrInvalid }

// Inspection is a read-only local plan and the most recent sync for this clone.
type Inspection struct {
	LastSync *state.SyncRecord
	Warnings []string
	Plan     plan.Plan
}

// Inspect plans without fetching, recovering or saving. A pending apply requires sync.
func (s Service) Inspect() (Inspection, error) {
	result := Inspection{Warnings: []string{}}
	if err := s.compatible(); err != nil {
		return result, err
	}
	err := s.Store.WithLock(func(session *state.Session) error {
		m, mode, err := s.intent(&result.Warnings)
		if err != nil {
			return err
		}
		record, exists, err := session.LoadSync()
		if err != nil {
			return err
		}
		if exists && record.Repository == s.Repository {
			result.LastSync = &record
		}
		result.Plan, err = s.build(session, m, mode, false, &result.Warnings)
		return err
	})
	return result, err
}

// GitStatus is a stable projection of Git's local knowledge of its upstream.
type GitStatus struct {
	Branch       string `json:"branch"`
	Commit       string `json:"commit"`
	Upstream     string `json:"upstream"`
	Ahead        int    `json:"ahead"`
	Behind       int    `json:"behind"`
	Dirty        bool   `json:"dirty"`
	UpstreamGone bool   `json:"upstream_gone"`
}

// Status is versioned independently of plan and state JSON.
type Status struct {
	ToolTooOld   *compatibility.Event        `json:"tool_too_old,omitempty"`
	LastSync     *state.SyncRecord           `json:"last_sync"`
	Drift        map[plan.Classification]int `json:"drift"`
	Repository   string                      `json:"repository"`
	Warnings     []string                    `json:"warnings"`
	Git          GitStatus                   `json:"git"`
	StatusSchema int                         `json:"status_schema"`
}

// Status reports cached upstream counts, last sync, planned drift and warnings.
// It performs no network operations and never applies or recovers host changes.
func (s Service) Status(ctx context.Context) (Status, error) {
	inspection, err := s.Inspect()
	if err != nil {
		return Status{StatusSchema: 1, Repository: s.Repository, ToolTooOld: compatibility.FromError(err, s.Repository), Warnings: []string{}, Drift: map[plan.Classification]int{plan.InSync: 0, plan.UpdateNeeded: 0, plan.LocalDrift: 0, plan.Conflict: 0, plan.UnmanagedObstacle: 0}}, err
	}
	git, err := s.Backend.Status(ctx, s.Repository)
	if err != nil {
		return Status{}, err
	}
	result := Status{
		StatusSchema: 1, Repository: s.Repository, Git: gitStatus(git), LastSync: inspection.LastSync, Warnings: inspection.Warnings,
		Drift: map[plan.Classification]int{plan.InSync: 0, plan.UpdateNeeded: 0, plan.LocalDrift: 0, plan.Conflict: 0, plan.UnmanagedObstacle: 0},
	}
	for _, observation := range inspection.Plan.Observations {
		result.Drift[observation.Classification]++
	}
	for _, op := range inspection.Plan.Operations {
		if op.Warning != "" {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s: %s", op.Path, op.Warning))
		}
	}
	if git.Upstream == "" || git.UpstreamGone {
		result.Warnings = append(result.Warnings, "working copy has no available upstream")
	}
	return result, nil
}

func gitStatus(s vcs.Status) GitStatus {
	return GitStatus{Branch: s.Branch, Commit: s.Commit, Upstream: s.Upstream, Ahead: s.Ahead, Behind: s.Behind, Dirty: !s.Clean(), UpstreamGone: s.UpstreamGone}
}
