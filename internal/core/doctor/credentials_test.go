package doctor

import (
	"cmp"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

type credentialBackend struct {
	vcs.Backend
	err       error
	diagnosis vcs.RemoteDiagnosis
}

// DiagnoseRemote supplies only safe provenance, alongside potentially unsafe errors.
func (b credentialBackend) DiagnoseRemote(context.Context, string) (vcs.RemoteDiagnosis, error) {
	return b.diagnosis, b.err
}

func TestRemoteCredentials(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, source, fix, want string
		fail                    bool
	}{
		{name: "agent", source: "SSH agent", want: "credentials: SSH agent"},
		{name: "identity", source: "identity file id_ed25519", want: "credentials: identity file id_ed25519"},
		{name: "filename redacted", source: "identity file ghp_FAKE_diagnostic_secret", want: "credentials: identity file"},
		{name: "failure", fix: "repair known_hosts and unlock your SSH agent", fail: true},
		{name: "failure without hint", fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s, _ := subject(t)
			backend := credentialBackend{diagnosis: vcs.RemoteDiagnosis{Source: tc.source, Fix: tc.fix}}
			if tc.fail {
				backend.err = errors.New("PRIVATE KEY passphrase=hunter2 ssh://username:password@example.invalid")
			}
			check := s.remoteCheck(t.Context(), backend)
			wantStatus, wantFix := OK, ""
			if tc.fail {
				wantStatus, wantFix = Fail, cmp.Or(tc.fix, remoteFix)
			}
			if check.Status != wantStatus || check.Fix != wantFix || !strings.Contains(check.Message, tc.want) {
				t.Fatalf("check=%+v", check)
			}
			for _, secret := range []string{"hunter2", "PRIVATE KEY", "username", "password", "ghp_FAKE_diagnostic_secret"} {
				if strings.Contains(check.Message, secret) {
					t.Fatalf("secret leaked: %s", check.Message)
				}
			}
			s.Offline = true
			offline := s.remoteCheck(t.Context(), backend)
			if !strings.Contains(offline.Message, "skipped") {
				t.Fatal("offline probed", offline)
			}
		})
	}
}
