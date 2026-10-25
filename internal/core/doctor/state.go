package doctor

import (
	"context"
	"errors"

	"github.com/TomaszJanusz/kitbag/internal/core/state"
)

func (s Service) stateChecks(context.Context) []Check {
	var healthErr error
	lockErr := s.Store.WithLock(func(session *state.Session) error {
		_, healthErr = session.Load()
		_, _, syncErr := session.LoadSync()
		healthErr = errors.Join(healthErr, syncErr)
		return nil
	})
	checks := []Check{result("state-lock", "state lock available (released immediately)", "wait for the running sync to exit; check state directory permissions", lockErr, Fail)}
	if lockErr == nil {
		checks = append(checks, result("state-health", "state files have supported schemas and valid data", "restore a valid state backup or use a compatible kitbag; do not delete state blindly", healthErr, Fail))
	}
	return append(checks, s.materialChecks()...)
}
