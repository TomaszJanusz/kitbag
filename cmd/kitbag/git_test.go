package main

import (
	"bytes"
	"strings"
	"testing"
)

// The messages below are documented in docs/vcs.md; changing them is a
// user-visible change.
const (
	systemMissing = `kitbag: Git backend "system" (selected by %s) is unavailable: ` +
		`git was not found in PATH; install Git 2.30 or newer (https://git-scm.com/downloads)` + "\n"
	embeddedMissing = `kitbag: Git backend "embedded" (selected by %s) is unavailable: ` +
		`the embedded Git backend is not available in this version of kitbag; ` +
		`use --git-backend=system and install Git 2.30 or newer (https://git-scm.com/downloads)` + "\n"
	invalidBackend = `kitbag: %s: invalid Git backend "bogus": want auto, system or embedded` + "\n"
	missingValue   = "kitbag: flag needs an argument: --git-backend\n"
)

func TestRunGitBackend(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		err  string
		env  []string
		args []string
		code int
	}{
		{name: "auto needs no git", args: []string{"--git-backend=auto", "version"}},
		{name: "default needs no git", args: []string{"version"}},
		{
			name: "forced system", args: []string{"--git-backend=system", "version"}, code: 1,
			err: sprintf(systemMissing, "--git-backend"),
		},
		{
			name: "flag after command", args: []string{"version", "--git-backend", "system"}, code: 1,
			err: sprintf(systemMissing, "--git-backend"),
		},
		{
			name: "forced by environment", env: []string{"KITBAG_GIT_BACKEND", "system"}, args: []string{"version"},
			code: 1, err: sprintf(systemMissing, "KITBAG_GIT_BACKEND"),
		},
		{
			name: "flag beats environment", env: []string{"KITBAG_GIT_BACKEND", "system"},
			args: []string{"--git-backend=auto", "version"},
		},
		{name: "last flag wins", args: []string{"--git-backend=system", "--git-backend=auto", "version"}},
		{
			name: "forced embedded", args: []string{"--git-backend", "embedded", "version"}, code: 1,
			err: sprintf(embeddedMissing, "--git-backend"),
		},
		{
			name: "invalid flag", args: []string{"--git-backend=bogus", "version"}, code: 2,
			err: sprintf(invalidBackend, "--git-backend"),
		},
		{
			name: "invalid environment", env: []string{"KITBAG_GIT_BACKEND", "bogus"}, args: []string{"version"},
			code: 2, err: sprintf(invalidBackend, "KITBAG_GIT_BACKEND"),
		},
		{name: "missing value", args: []string{"version", "--git-backend"}, code: 2, err: missingValue},
		{name: "empty value is unset", args: []string{"--git-backend=", "version"}},
		{name: "similar flag is no flag", args: []string{"--git-backends=auto", "version"}, code: 2, err: "kitbag: unknown flag: --git-backends\n"},
		{name: "after --", args: []string{"--", "--git-backend=system", "version"}, code: 2, err: "kitbag: " + usage + "\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer
			code := run("linux", noGit(tt.env...), tt.args, &stdout, &stderr)
			if code != tt.code || stderr.String() != tt.err {
				t.Errorf("run(%q) = %d, stderr %q; want %d, %q", tt.args, code, stderr.String(), tt.code, tt.err)
			}
			if wantOut := "kitbag dev\n"; (code == 0) != (stdout.String() == wantOut) {
				t.Errorf("stdout = %q with exit code %d", stdout.String(), code)
			}
		})
	}
}

func sprintf(format, source string) string {
	return strings.ReplaceAll(format, "%s", source)
}
