// Package daemonhost implements daemon clocks, release metadata and bounded logs.
// Scheduling policy belongs in core/daemon; host paths arrive through platform.
package daemonhost

import (
	"time"

	"github.com/TomaszJanusz/kitbag/internal/core/daemon"
)

// Clock supplies standard library timers.
type (
	Clock struct{}
	timer struct{ value *time.Timer }
)

// NewTimer constructs a stoppable timer.
func (Clock) NewTimer(d time.Duration) daemon.Timer { return timer{time.NewTimer(d)} }

// C returns the timer channel.
func (t timer) C() <-chan time.Time { return t.value.C }

// Reset rearms the timer.
func (t timer) Reset(d time.Duration) { t.value.Reset(d) }

// Stop disarms the timer.
func (t timer) Stop() { t.value.Stop() }
