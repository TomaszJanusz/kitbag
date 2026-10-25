package model

import (
	"strconv"

	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
)

// Phase is a model's loading state.
type Phase int

// Loading states. Failed means no usable snapshot exists yet.
const (
	Loading Phase = iota
	Ready
	Failed
)

// Row is a labelled value.
type Row struct {
	Label, Value text.Message
}

// Control is an action a renderer offers; dispatch Intent when chosen.
type Control struct {
	Intent  Msg
	Label   text.Message
	Enabled bool
}

// ProblemText is the user text for a classified failure.
func ProblemText(problem *Error) text.Message {
	keys := map[ErrorKind]text.Key{
		ErrOperation:     text.ErrorOperation,
		ErrUnavailable:   text.ErrorUnavailable,
		ErrBusy:          text.ErrorBusy,
		ErrInvalid:       text.ErrorInvalid,
		ErrConflict:      text.ErrorConflict,
		ErrToolTooOld:    text.ErrorToolTooOld,
		ErrUnauthorized:  text.ErrorUnauthorized,
		ErrIncompatible:  text.ErrorIncompatible,
		ErrUnsupportedOS: text.ErrorUnsupportedOS,
		ErrCanceled:      text.ErrorCanceled,
	}
	key, ok := keys[problem.Kind]
	if !ok {
		key = text.ErrorOperation
	}
	return text.New(key)
}

// PhaseText names a sync.progress phase.
func PhaseText(phase string) text.Message {
	keys := map[string]text.Key{
		"started": text.PhaseStarted, "pull": text.PhasePull, "plan": text.PhasePlan,
		"apply": text.PhaseApply, "includes": text.PhaseIncludes, "finished": text.PhaseFinished,
	}
	if key, ok := keys[phase]; ok {
		return text.New(key)
	}
	return text.New(text.PhaseOther)
}

// ResultText describes a shared CLI exit code.
func ResultText(code int) text.Message {
	keys := []text.Key{text.ResultOK, text.ResultWarnings, text.ResultUsage, text.ResultConflict, text.ResultToolTooOld, text.ResultUnsupportedOS}
	if code >= 0 && code < len(keys) {
		return text.New(keys[code])
	}
	return text.New(text.ResultUnknown, number(code))
}

// ClassificationText names a drift classification.
func ClassificationText(class Classification) text.Message {
	keys := map[Classification]text.Key{
		InSync: text.ClassInSync, UpdateNeeded: text.ClassUpdate, LocalDrift: text.ClassDrift,
		Conflict: text.ClassConflict, UnmanagedObstacle: text.ClassUnmanaged,
	}
	if key, ok := keys[class]; ok {
		return text.New(key)
	}
	return text.Plain(string(class))
}

func plain(values []string) []text.Message {
	result := make([]text.Message, 0, len(values))
	for _, value := range values {
		result = append(result, text.Plain(value))
	}
	return result
}

func number(n int) text.Message { return text.Plain(strconv.Itoa(n)) }
