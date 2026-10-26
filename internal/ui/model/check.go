package model

import (
	"time"

	"github.com/TomaszJanusz/kitbag/internal/timefmt"
	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
)

// CheckValue renders a remote check using the same safe catalog as failures.
func CheckValue(clock timefmt.Clock, check *LastCheck, running bool) text.Message {
	if check == nil {
		return text.New(text.StatusCheckUnknown)
	}
	outcome := text.New(text.StatusCheckOK)
	if check.Outcome == "failed" {
		outcome = text.New(text.StatusCheckFailed, ProblemText(&Error{Kind: ErrorKind(check.Category)}))
	}
	value := text.New(text.StatusCheckValue, WhenText(clock, check.At), outcome)
	if !running {
		value = text.New(text.StatusCheckStopped, value)
	}
	return value
}

// CheckStale evaluates liveness against the configured interval, not backoff.
func CheckStale(check *LastCheck, now time.Time) bool {
	const intervals = 3
	return check != nil && check.Interval > 0 && now.Sub(check.At) > intervals*check.Interval
}

func (m *Status) checkWarnings() []text.Message {
	check := m.snapshot.LastCheck
	var warnings []text.Message
	if check != nil && check.Outcome == "failed" {
		warnings = append(warnings, CheckValue(m.clock, check, m.snapshot.Daemon))
	}
	if m.snapshot.Daemon && m.clock.Now != nil && CheckStale(check, m.clock.Now()) {
		warnings = append(warnings, text.New(text.StatusCheckStuck))
	}
	return warnings
}
