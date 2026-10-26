// Package timefmt renders timestamps for people: wall-clock time in an
// explicit location and a coarse relative hint. Machine contracts (JSON,
// state files, the API) keep UTC RFC 3339 and never go through this package.
// The location and "now" are inputs, so output is deterministic in tests.
package timefmt

import (
	"strconv"
	"time"
)

// Layouts of the human-facing renderings.
const (
	// Stamp is a date and time with the zone abbreviation, e.g. 2026-10-26 15:47:51 CET.
	Stamp = "2006-01-02 15:04:05 MST"
	// TimeOfDay is the compact form used by the activity log.
	TimeOfDay = "15:04:05"
)

// day is a fixed 24 hours: hints are durations, so DST changes do not matter.
const day = 24 * time.Hour

// Unit is the granularity of a relative hint.
type Unit int

// Units, from the finest. JustNow also covers timestamps slightly in the future.
const (
	JustNow Unit = iota
	Minutes
	Hours
	Days
)

// Clock holds the zone to render in and the current time. The zero value
// renders in UTC and gives no relative hints.
type Clock struct {
	Location *time.Location
	Now      func() time.Time
}

// In converts t to the clock's location (UTC when none is set).
func (c Clock) In(t time.Time) time.Time {
	if c.Location == nil {
		return t.UTC()
	}
	return t.In(c.Location)
}

// Stamp renders t as "2006-01-02 15:04:05 MST" in the clock's location.
func (c Clock) Stamp(t time.Time) string { return c.In(t).Format(Stamp) }

// TimeOfDay renders t as "15:04:05" in the clock's location.
func (c Clock) TimeOfDay(t time.Time) string { return c.In(t).Format(TimeOfDay) }

// Since reports how long before now t happened, rounded down to one unit.
// ok is false when the clock has no Now.
func (c Clock) Since(t time.Time) (unit Unit, count int, ok bool) {
	if c.Now == nil {
		return JustNow, 0, false
	}
	elapsed := c.Now().Sub(t)
	switch {
	case elapsed < time.Minute:
		return JustNow, 0, true
	case elapsed < time.Hour:
		return Minutes, int(elapsed / time.Minute), true
	case elapsed < day:
		return Hours, int(elapsed / time.Hour), true
	}
	return Days, int(elapsed / day), true
}

// Ago is the English relative hint ("3 min ago"), or "" without a Now.
func (c Clock) Ago(t time.Time) string {
	unit, count, ok := c.Since(t)
	switch {
	case !ok:
		return ""
	case unit == Minutes:
		return strconv.Itoa(count) + " min ago"
	case unit == Hours:
		return strconv.Itoa(count) + " h ago"
	case unit == Days:
		return strconv.Itoa(count) + " d ago"
	}
	return "just now"
}

// Describe is Stamp followed by the relative hint in parentheses, if any.
func (c Clock) Describe(t time.Time) string {
	if ago := c.Ago(t); ago != "" {
		return c.Stamp(t) + " (" + ago + ")"
	}
	return c.Stamp(t)
}
