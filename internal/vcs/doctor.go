package vcs

import "context"

// RemoteProber is an optional read-only network capability. It must not fetch
// objects or update refs. Context cancellation bounds the probe.
type RemoteProber interface {
	ProbeRemote(ctx context.Context, dir string) error
}

// CommitInspector reads effective commit configuration without making a commit.
// It returns presence, never identity/key values. Embedded Git reports identity
// presence only; doctor requires the system backend for signing inspection.
type CommitInspector interface {
	InspectCommit(ctx context.Context, dir string) (CommitConfig, error)
}

// CommitConfig describes configuration presence, not proof that signing succeeds.
type CommitConfig struct {
	Identity   bool
	Signing    bool
	SigningKey bool
}
