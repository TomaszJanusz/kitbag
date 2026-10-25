package vcs

import "context"

// RemoteProber is an optional read-only network capability. It must not fetch
// objects or update refs. Context cancellation bounds the probe.
type RemoteProber interface {
	ProbeRemote(ctx context.Context, dir string) error
}

// RemoteDiagnoser probes origin and returns safe credential provenance. A source
// is evidence only when the operation succeeds. Implementations must never
// return credential bytes, remote URLs or arbitrary transport output here.
type RemoteDiagnoser interface {
	DiagnoseRemote(ctx context.Context, dir string) (RemoteDiagnosis, error)
}

// RemoteDiagnosis contains display-only authentication evidence and a fix hint.
type RemoteDiagnosis struct {
	Source string
	Fix    string
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
