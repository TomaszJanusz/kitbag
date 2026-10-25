package model

import (
	"time"

	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
)

// DefaultLogCapacity bounds the activity log when no capacity is given.
const DefaultLogCapacity = 256

// Severity ranks a log entry.
type Severity int

// Severities.
const (
	Info Severity = iota
	Warning
	Failure
)

// LogEntry is one line of activity. At is when the renderer received it.
type LogEntry struct {
	At       time.Time
	Text     text.Message
	Severity Severity
}

// Log is a bounded ring buffer of activity: lifecycle hints, this renderer's
// sync progress and setting writes. The oldest entry is dropped when full.
type Log struct {
	entries   []LogEntry
	start     int
	dropped   int
	connected bool
}

// NewLog returns an empty log; capacity below one uses DefaultLogCapacity.
func NewLog(capacity int) *Log {
	if capacity < 1 {
		capacity = DefaultLogCapacity
	}
	return &Log{entries: make([]LogEntry, 0, capacity)}
}

// Update records msg if it is activity. It never issues commands.
func (m *Log) Update(msg Msg) []Cmd {
	switch msg := msg.(type) {
	case EventReceived:
		m.event(msg)
	case SubscriptionEnded:
		// Report a lost connection once, not every failed reconnect attempt.
		if m.connected {
			m.connected = false
			m.add(msg.At, Warning, text.New(text.LogDisconnected))
		}
	case SyncProgressed:
		m.add(msg.At, Info, text.New(text.LogProgress, PhaseText(msg.Phase)))
	case SyncFinished:
		if problem := Classify(msg.Err); problem != nil {
			m.add(msg.At, Failure, text.New(text.LogOwnSyncFailed, ProblemText(problem)))
		} else {
			m.add(msg.At, severity(msg.Outcome.ExitCode), text.New(text.LogOwnSync, ResultText(msg.Outcome.ExitCode), number(msg.Outcome.Changed)))
		}
	case SettingWritten:
		if problem := Classify(msg.Err); problem != nil {
			m.add(msg.At, Failure, text.New(text.LogSettingFailed, text.Plain(msg.Key), ProblemText(problem)))
		} else {
			m.add(msg.At, Info, text.New(text.LogSettingSaved, text.Plain(msg.Key)))
		}
	}
	return nil
}

func (m *Log) event(msg EventReceived) {
	e := msg.Event
	switch e.Kind {
	case EventConnected:
		m.connected = true
		m.add(msg.At, Info, text.New(text.LogConnected))
	case EventSyncStarted:
		m.add(msg.At, Info, text.New(text.LogSyncStarted))
	case EventSyncFinished:
		m.add(msg.At, severity(e.ExitCode), text.New(text.LogSyncFinished, ResultText(e.ExitCode), number(e.Changed)))
	case EventDriftDetected:
		m.add(msg.At, Warning, text.New(text.LogDriftDetected))
	case EventUpdateAvailable:
		m.add(msg.At, Info, text.New(text.LogUpdate))
	default:
		m.add(msg.At, Info, text.New(text.LogEventUnknown, text.Plain(string(e.Kind))))
	}
}

func severity(code int) Severity {
	if code == 0 {
		return Info
	}
	return Warning
}

func (m *Log) add(at time.Time, level Severity, message text.Message) {
	entry := LogEntry{At: at, Severity: level, Text: message}
	if len(m.entries) < cap(m.entries) {
		m.entries = append(m.entries, entry)
		return
	}
	m.entries[m.start] = entry
	m.start = (m.start + 1) % len(m.entries)
	m.dropped++
}

// Entries returns a copy, oldest first.
func (m *Log) Entries() []LogEntry {
	result := make([]LogEntry, 0, len(m.entries))
	result = append(result, m.entries[m.start:]...)
	return append(result, m.entries[:m.start]...)
}

// Dropped counts entries discarded because the log was full.
func (m *Log) Dropped() int { return m.dropped }

// Capacity is the maximum number of retained entries.
func (m *Log) Capacity() int { return cap(m.entries) }

// LogView is everything a renderer shows for the activity log.
type LogView struct {
	Title   text.Message
	Entries []LogEntry
	Dropped int
}

// View projects the retained entries, oldest first.
func (m *Log) View() LogView {
	return LogView{Title: text.New(text.LogTitle), Entries: m.Entries(), Dropped: m.dropped}
}
