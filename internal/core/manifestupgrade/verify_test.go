package manifestupgrade

import (
	"errors"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/manifestedit"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// TestVerifyRefusesADifferentResolution feeds verify a schema-2 result that
// drops the host-only instance of runner02: the upgrade must stop before
// anything is written, whatever the edits produced.
func TestVerifyRefusesADifferentResolution(t *testing.T) {
	t.Parallel()
	before := "schema = 1\n[targets.claude]\nagent = \"claude\"\n[targets.claude.instances.main]\ndir = \"~/.claude\"\n" +
		"[hosts.runner02.targets.claude.instances.main]\ndir = \"~/.claude\"\n[hosts.runner02.targets.claude.instances.work]\ndir = \"~/.claude-work\"\n"
	m, err := manifest.Parse("kitbag.toml", []byte(before))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, after, want string }{
		{"lossy", "schema = 2\n[targets.claude]\nagent = \"claude\"\n[targets.claude.instances.main]\ndir = \"~/.claude\"\n", "resolves differently on host runner02"},
		{"lossless", "schema = 2\n[targets.claude]\nagent = \"claude\"\n[targets.claude.instances.main]\ndir = \"~/.claude\"\n[targets.claude.instances.work]\ndir = \"~/.claude-work\"\nhosts = [\"runner02\"]\n", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			plan := &Plan{After: []byte(tc.after)}
			err := (&converter{in: Input{Filename: "kitbag.toml"}, m: m, plan: plan}).verify()
			if tc.want == "" {
				if err != nil || strings.Join(plan.Hosts, ",") != "runner02,kitbag-undeclared-host" {
					t.Fatalf("verify %v, checked %q", err, plan.Hosts)
				}
				return
			}
			if !errors.Is(err, manifestedit.ErrVerify) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("verify %v, want %q", err, tc.want)
			}
		})
	}
}
