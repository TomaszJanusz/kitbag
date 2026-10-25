package vcs

import (
	"context"
	"errors"
	"fmt"
)

// Kind names a backend choice.
type Kind string

// Backend choices.
const (
	// Auto selects system Git when a suitable version is installed and the
	// embedded backend otherwise.
	Auto Kind = "auto"
	// System is Git installed on the machine, version 2.30 or newer.
	System Kind = "system"
	// Embedded is the go-git backend built into kitbag.
	Embedded Kind = "embedded"
)

// EnvVar is the environment variable that selects a backend.
const EnvVar = "KITBAG_GIT_BACKEND"

// SystemHint tells the user how to make the system backend available.
const SystemHint = "install Git 2.30 or newer (https://git-scm.com/downloads)"

// Selection errors.
var (
	// ErrInvalidKind reports a backend name other than auto, system and
	// embedded.
	ErrInvalidKind = errors.New("invalid Git backend")
	// ErrSystemGitNotFound permits automatic fallback only when Git is absent.
	ErrSystemGitNotFound = errors.New("git was not found in PATH")
	// ErrUnavailable reports that the selected backend cannot be used. It is
	// always wrapped in an [*UnavailableError].
	ErrUnavailable = errors.New("git backend unavailable")
)

// ParseKind parses a backend name exactly as the user wrote it.
func ParseKind(s string) (Kind, error) {
	switch k := Kind(s); k {
	case Auto, System, Embedded:
		return k, nil
	}
	return "", fmt.Errorf("%w %q: want auto, system or embedded", ErrInvalidKind, s)
}

// Source says where a [Selection] came from.
type Source string

// Selection sources, highest precedence first. The values are shown to users.
const (
	SourceFlag    Source = "--git-backend"
	SourceEnv     Source = EnvVar
	SourceConfig  Source = "git.backend in config.toml"
	SourceDefault Source = "default"
)

// Selection is the requested backend and where the request came from.
type Selection struct {
	Kind   Kind
	Source Source
}

// Explicit reports whether the user asked for a specific backend rather than
// leaving the choice to kitbag.
func (s Selection) Explicit() bool {
	return s.Kind != Auto
}

// Select applies the precedence --git-backend flag > KITBAG_GIT_BACKEND >
// git.backend in config.toml > auto. An empty argument means the level is
// unset. Only the winning value is parsed; an invalid one is an error naming
// its source, never a reason to consult a lower level.
func Select(flag, env, config string) (Selection, error) {
	for _, level := range []struct {
		value  string
		source Source
	}{
		{flag, SourceFlag},
		{env, SourceEnv},
		{config, SourceConfig},
	} {
		if level.value == "" {
			continue
		}
		kind, err := ParseKind(level.value)
		if err != nil {
			return Selection{}, fmt.Errorf("%s: %w", level.source, err)
		}
		return Selection{Kind: kind, Source: level.source}, nil
	}
	return Selection{Kind: Auto, Source: SourceDefault}, nil
}

// Factories construct backends for [Open]. A factory returns an error
// explaining why its backend is unusable on this machine; a nil factory means
// the backend is not part of this build.
type Factories struct {
	System   func(ctx context.Context) (Backend, error)
	Embedded func(ctx context.Context) (Backend, error)
}

// Resolution is the outcome of [Open]. It is the doctor hook: it names the
// requested and the active backend, where the request came from and, in
// automatic mode, why system Git was not used.
type Resolution struct {
	// SystemErr explains why automatic mode did not use system Git.
	SystemErr error
	Selection
	// Active is the backend in use, or "" when none could be opened.
	Active Kind
	// Version describes the active backend, such as "git 2.43.0".
	Version string
}

// String summarizes the resolution in one line, for example
// "system (git 2.43.0), selected by default".
func (r Resolution) String() string {
	if r.Active == "" {
		return fmt.Sprintf("none (%s requested by %s)", r.Kind, r.Source)
	}
	return fmt.Sprintf("%s (%s), selected by %s", r.Active, r.Version, r.Source)
}

// UnavailableError reports that the selected backend cannot be used. It
// matches [ErrUnavailable] and its cause.
type UnavailableError struct {
	Selection
	// Err is the reason.
	Err error
	// Hint is a one-line fix for the user.
	Hint string
}

// Error returns the user-facing message, including the hint.
func (e *UnavailableError) Error() string {
	return fmt.Sprintf("Git backend %q (selected by %s) is unavailable: %v; %s",
		e.Kind, e.Source, e.Err, e.Hint)
}

// Unwrap returns [ErrUnavailable] and the cause.
func (e *UnavailableError) Unwrap() []error {
	return []error{ErrUnavailable, e.Err}
}

// Open returns the backend sel asks for. A forced backend that cannot be
// opened is an [*UnavailableError], never a fallback to the other one; only
// [Auto] falls back from system Git to the embedded backend. The returned
// [Resolution] is filled in even when Open fails.
func Open(ctx context.Context, sel Selection, f Factories) (Backend, Resolution, error) {
	res := Resolution{Selection: sel}
	var (
		b   Backend
		err error
	)
	switch sel.Kind {
	case System:
		b, err = openSystem(ctx, sel, f)
	case Embedded:
		b, err = openEmbedded(ctx, sel, f)
	case Auto:
		b, res.SystemErr, err = openAuto(ctx, sel, f)
	default:
		err = fmt.Errorf("%w %q", ErrInvalidKind, sel.Kind)
	}
	if err != nil {
		return nil, res, err
	}
	res.Active, res.Version = b.Kind(), b.Version()
	return b, res, nil
}

// openAuto prefers system Git and falls back to the embedded backend,
// returning why system Git was not used.
func openAuto(ctx context.Context, sel Selection, f Factories) (b Backend, systemErr, err error) {
	b, systemErr = build(ctx, f.System, errors.ErrUnsupported)
	if systemErr == nil {
		return b, nil, nil
	}
	if !errors.Is(systemErr, ErrSystemGitNotFound) {
		return nil, systemErr, &UnavailableError{Selection: sel, Err: systemErr, Hint: SystemHint}
	}
	b, embeddedErr := build(ctx, f.Embedded, errors.ErrUnsupported)
	if embeddedErr != nil {
		cause := fmt.Errorf("system Git: %w; embedded: %w", systemErr, embeddedErr)
		return nil, systemErr, &UnavailableError{Selection: sel, Err: cause, Hint: SystemHint}
	}
	return b, systemErr, nil
}

func openSystem(ctx context.Context, sel Selection, f Factories) (Backend, error) {
	b, err := build(ctx, f.System, errors.ErrUnsupported)
	if err != nil {
		return nil, &UnavailableError{Selection: sel, Err: err, Hint: SystemHint}
	}
	return b, nil
}

func openEmbedded(ctx context.Context, sel Selection, f Factories) (Backend, error) {
	b, err := build(ctx, f.Embedded, errors.ErrUnsupported)
	if err != nil {
		return nil, &UnavailableError{Selection: sel, Err: err, Hint: "use --git-backend=system and " + SystemHint}
	}
	return b, nil
}

// build runs factory, or fails with missing when the backend is not part of
// this build.
func build(ctx context.Context, factory func(context.Context) (Backend, error), missing error) (Backend, error) {
	if factory == nil {
		return nil, missing
	}
	return factory(ctx)
}
