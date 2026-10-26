package model

import (
	"strconv"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/timefmt"
	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
)

// WhenText renders t in the clock's zone with a relative hint when the clock
// knows the time. Renderers show it verbatim; they never format times.
func WhenText(clock timefmt.Clock, t time.Time) text.Message {
	stamp := text.Plain(clock.Stamp(t))
	if clock.Now == nil {
		return stamp
	}
	return text.New(text.TimeWithAgo, stamp, RelativeText(clock, t))
}

// RelativeText renders a catalog-backed relative time, or a timestamp when no clock is supplied.
func RelativeText(clock timefmt.Clock, t time.Time) text.Message {
	unit, count, ok := clock.Since(t)
	if !ok {
		return text.Plain(clock.Stamp(t))
	}
	var ago text.Message
	switch unit {
	case timefmt.Minutes:
		ago = text.New(text.TimeMinutesAgo, text.Plain(strconv.Itoa(count)))
	case timefmt.Hours:
		ago = text.New(text.TimeHoursAgo, text.Plain(strconv.Itoa(count)))
	case timefmt.Days:
		ago = text.New(text.TimeDaysAgo, text.Plain(strconv.Itoa(count)))
	case timefmt.JustNow:
		ago = text.New(text.TimeJustNow)
	}
	return ago
}
