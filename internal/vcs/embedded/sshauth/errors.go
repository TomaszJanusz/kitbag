package sshauth

import "errors"

// Failure kinds let callers such as doctor choose a fix hint with errors.Is,
// without parsing messages. Messages never contain key material or passphrases.
var (
	ErrHostUnknown   = errors.New("unknown SSH host key")
	ErrHostChanged   = errors.New("SSH host key changed or revoked")
	ErrLocked        = errors.New("SSH identity is locked")
	ErrNoCredentials = errors.New("no usable SSH credentials")
)

type failure struct {
	kind error
	msg  string
}

// Error returns the fixed, display-safe message.
func (f *failure) Error() string { return f.msg }

// Unwrap exposes the failure kind.
func (f *failure) Unwrap() error { return f.kind }

func fail(kind error, msg string) error { return &failure{kind: kind, msg: msg} }
