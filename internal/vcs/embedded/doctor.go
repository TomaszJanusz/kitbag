package embedded

import (
	"context"
	"errors"
	"io"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp"
	"github.com/go-git/go-git/v5/plumbing/transport"

	"github.com/TomaszJanusz/kitbag/internal/vcs"
	"github.com/TomaszJanusz/kitbag/internal/vcs/embedded/sshauth"
)

// DiagnoseRemote lists origin refs without fetching; prompts remain disabled.
func (b *Backend) DiagnoseRemote(ctx context.Context, dir string) (diagnosis vcs.RemoteDiagnosis, err error) {
	diagnosis.Fix = "check origin, SSH agent/IdentityFile and known_hosts; unlock encrypted identities in an SSH agent"
	r, err := open(ctx, dir)
	if err != nil {
		return diagnosis, err
	}
	url, err := b.RemoteURL(ctx, dir, vcs.DefaultRemote)
	if err != nil {
		return diagnosis, err
	}
	// Doctor is always non-interactive, even on a backend used by a UI.
	quiet := *b
	quiet.opts.SSH.Interactive = false
	auth, closeAuth, err := quiet.auth(ctx, r, url)
	if errors.Is(err, vcs.ErrUnsupported) {
		diagnosis.Fix = "install git or resolve in the working copy"
	}
	if err != nil {
		return diagnosis, err
	}
	defer func() { err = errors.Join(err, closeAuth()) }()
	if credentials, ok := auth.(*sshauth.Auth); ok {
		err = probeSSH(credentials, url)
		if err == nil {
			diagnosis.Source = credentials.Source()
		}
		diagnosis.Fix = sshFix(err, diagnosis.Fix)
	} else {
		err = listRemote(ctx, r, auth)
		diagnosis.Source = "none required"
	}
	if err != nil {
		return diagnosis, err
	}

	diagnosis.Fix = ""
	return diagnosis, nil
}

func listRemote(ctx context.Context, r *git.Repository, auth transport.AuthMethod) error {
	remote, err := r.Remote(vcs.DefaultRemote)
	if err != nil {
		return err
	}
	_, err = remote.ListContext(ctx, &git.ListOptions{Auth: auth})
	if errors.Is(err, transport.ErrEmptyRemoteRepository) {
		return nil
	}
	return err
}

func probeSSH(auth *sshauth.Auth, url string) (err error) {
	endpoint, err := transport.NewEndpoint(url)
	if err != nil {
		return err
	}
	client, err := auth.Connect()
	if err != nil {
		return err
	}
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := session.Close(); closeErr != nil && !errors.Is(closeErr, io.EOF) {
			err = errors.Join(err, closeErr)
		}
	}()
	output, err := session.StdoutPipe()
	if err != nil {
		return err
	}
	session.Stderr = io.Discard
	// Quote exactly one remote path, using the same POSIX quoting as go-git.
	path := strings.NewReplacer("'", `'\''`, "!", `'\!'`).Replace(endpoint.Path)
	if err = session.Start("git-upload-pack '" + path + "'"); err != nil {
		return err
	}
	return packp.NewAdvRefs().Decode(output)
}

// sshFix maps typed credential failures to a specific, display-safe hint.
func sshFix(err error, fallback string) string {
	switch {
	case err == nil:
		return fallback
	case errors.Is(err, sshauth.ErrHostUnknown):
		return "verify the SSH host key fingerprint, then add it to known_hosts (for example by connecting once with ssh)"
	case errors.Is(err, sshauth.ErrHostChanged):
		return "verify the server's host key independently, then replace its known_hosts entry"
	case errors.Is(err, sshauth.ErrLocked):
		return "add the encrypted identity to your SSH agent (ssh-add) and retry"
	case errors.Is(err, sshauth.ErrNoCredentials):
		return "start an SSH agent with your key loaded, or set IdentityFile in ~/.ssh/config"
	default:
		return fallback
	}
}
