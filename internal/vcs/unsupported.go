package vcs

import (
	"errors"
	"fmt"
)

// ErrUnsupported classifies a feature the embedded backend cannot honor.
var ErrUnsupported = errors.New("unsupported embedded Git feature")

// UnsupportedError explains how to finish an operation requiring system Git.
type UnsupportedError struct {
	// Err optionally retains another classification, such as ErrNotFastForward.
	Err     error
	Feature string
}

// Error names the feature and the recovery action.
func (e *UnsupportedError) Error() string {
	return fmt.Sprintf("%s: %v; install git or resolve in the working copy", e.Feature, ErrUnsupported)
}

// Unwrap preserves both unsupported and operation-specific classifications.
func (e *UnsupportedError) Unwrap() []error { return []error{ErrUnsupported, e.Err} }

// Identity is an unsigned commit author's name and email address.
type Identity struct {
	Name  string `toml:"name"`
	Email string `toml:"email"`
}
