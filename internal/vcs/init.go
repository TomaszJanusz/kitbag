package vcs

import "context"

// Initializer is an optional capability for creating an unborn main branch.
// Callers must supply a new or empty directory; implementations never reinitialize
// an existing repository. No commit, hooks or global configuration are written.
type Initializer interface {
	Init(ctx context.Context, dir string) error
}
