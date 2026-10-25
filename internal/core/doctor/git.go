package doctor

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

func (s Service) gitChecks(ctx context.Context) []Check {
	probe, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	backend, resolution, err := s.Open(probe)
	checks := []Check{result("git-backend", fmt.Sprintf("requested %s; active %s", resolution.Kind, resolution.String()), vcs.SystemHint+" or select an available --git-backend", err, Fail)}
	if err != nil {
		return checks
	}
	if resolution.SystemErr != nil {
		checks[0].Status, checks[0].Fix = Warn, vcs.SystemHint+" to use system Git"
		checks[0].Message += "; system Git unavailable: " + resolution.SystemErr.Error()
	}
	checks = append(checks, s.remoteCheck(ctx, backend))
	return append(checks, s.commitChecks(ctx, backend)...)
}

const remoteFix = "check origin URL, connectivity and authentication; retry doctor"

func (s Service) remoteCheck(ctx context.Context, backend vcs.Backend) Check {
	if s.Offline {
		return Check{ID: "remote", Status: OK, Message: "skipped: --report performs no network operations"}
	}
	if probe, ok := backend.(vcs.RemoteDiagnoser); ok {
		return s.diagnoseRemote(ctx, probe)
	}
	probe, ok := backend.(vcs.RemoteProber)
	if !ok {
		return result("remote", "backend cannot probe remote", "use --git-backend=system to check remote reachability", errors.ErrUnsupported, Warn)
	}
	bounded, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	err := probe.ProbeRemote(bounded, s.Repository)
	// Never copy remote URLs or helper output into diagnostics.
	if err != nil {
		err = errors.New("origin is unreachable or the working copy is unavailable")
	}
	return result("remote", "remote reachability", remoteFix, err, Fail)
}

// diagnoseRemote reports credential provenance; errors may hold unsafe text and
// are never copied into the check.
func (s Service) diagnoseRemote(ctx context.Context, probe vcs.RemoteDiagnoser) Check {
	bounded, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	diagnosis, err := probe.DiagnoseRemote(bounded, s.Repository)
	if err != nil {
		if diagnosis.Fix == "" {
			diagnosis.Fix = remoteFix
		}
		return result("remote", "origin is unreachable or SSH authentication/host verification failed", diagnosis.Fix, errors.New("remote probe failed"), Fail)
	}
	home, err := s.Host.HomeDir()
	if err != nil {
		home = ""
	}
	return Check{ID: "remote", Status: OK, Message: "remote reachability; credentials: " + RedactText(diagnosis.Source, home)}
}

func (s Service) commitChecks(ctx context.Context, backend vcs.Backend) []Check {
	inspector, ok := backend.(vcs.CommitInspector)
	if backend.Kind() != vcs.System || !ok {
		return []Check{{ID: "commit-config", Status: OK, Message: "skipped: commit identity/signing inspection requires system Git"}}
	}
	bounded, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	cfg, err := inspector.InspectCommit(bounded, s.Repository)
	if err != nil {
		return []Check{result("commit-config", "cannot read Git commit configuration", "repair the working copy and Git config; retry doctor", fs.ErrInvalid, Warn)}
	}
	var identityErr, signingErr error
	if !cfg.Identity {
		identityErr = errors.New("user.name or user.email is missing")
	}
	if cfg.Signing && !cfg.SigningKey {
		signingErr = errors.New("commit.gpgsign is enabled without user.signingkey")
	}
	return []Check{
		result("commit-identity", "commit identity configured", "set git config user.name and user.email in the working copy", identityErr, Warn),
		result("commit-signing", "signing configuration checked (key usability not tested)", "set git config user.signingkey or explicitly disable commit.gpgsign", signingErr, Warn),
	}
}

func (s Service) sshChecks(context.Context) []Check {
	socket, _ := s.Host.LookupEnv("SSH_AUTH_SOCK")
	info, err := s.Host.Stat(socket)
	if socket == "" || err == nil && info.Mode()&fs.ModeSocket == 0 {
		err = errors.New("SSH_AUTH_SOCK does not name a socket")
	}
	return []Check{result("ssh-agent", "SSH agent socket", "start an SSH agent and export SSH_AUTH_SOCK (only needed for SSH remotes)", err, Warn)}
}

func (s Service) agentChecks(ctx context.Context) []Check {
	names := []string{"claude", "codex"}
	checks := make([]Check, 0, len(names))
	for _, name := range names {
		bounded, cancel := context.WithTimeout(ctx, Timeout)
		version, err := s.VersionProbe(bounded, name)
		cancel()
		if err != nil {
			err = errors.New("binary absent or --version probe failed/timed out")
		}
		checks = append(checks, result("agent-"+name, name+" "+version, "install "+name+" or check its PATH and --version command", err, Warn))
	}
	return checks
}
