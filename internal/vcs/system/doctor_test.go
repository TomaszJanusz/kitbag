package system

import (
	"slices"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

func TestDoctorGitCommands(t *testing.T) {
	t.Parallel()
	r := &fakeRunner{gitDir: "/bin", reply: doctorGitReply}
	env := fake.New(fake.WithEnv(hostileEnv("/bin")))
	b, err := New(t.Context(), Options{Env: env, Runner: r})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := b.InspectCommit(t.Context(), "/work/repo")
	if err != nil || cfg != (vcs.CommitConfig{Identity: true, Signing: true, SigningKey: true}) {
		t.Fatal(cfg, err)
	}
	if err = b.ProbeRemote(t.Context(), "/work/repo"); err != nil {
		t.Fatal(err)
	}
	cmd := r.cmds[len(r.cmds)-1]
	if !slices.Equal(cmd.Args, []string{"ls-remote", "--quiet", "--", "origin"}) || cmd.Dir != "/work/repo" {
		t.Fatal("unexpected remote probe", cmd)
	}
}

func TestAgentProbeEnvironment(t *testing.T) {
	t.Parallel()
	r := &fakeRunner{gitDir: "/bin", reply: versionReply("agent 1.2.3\n")}
	env := fake.New(fake.WithEnv(hostileEnv("/bin")))
	got, err := ProbeVersion(t.Context(), env, r, "claude")
	if err != nil || got != "agent 1.2.3" {
		t.Fatal(got, err)
	}
	cmd := r.cmds[0]
	if cmd.Path != "/bin/claude" || !slices.Equal(cmd.Args, []string{"--version"}) || !slices.Equal(cmd.Env, []string{"PATH=/bin", "LC_ALL=C"}) {
		t.Fatal("agent inherited private environment", cmd)
	}
}

func doctorGitReply(cmd Command) Result {
	if slices.Equal(cmd.Args, []string{"--version"}) {
		return Result{Stdout: []byte("git version 2.50.0")}
	}
	switch cmd.Args[len(cmd.Args)-1] {
	case "user.name":
		return Result{Stdout: []byte("Private Name\n")}
	case "user.email":
		return Result{Stdout: []byte("private@example.invalid\n")}
	case "commit.gpgsign":
		return Result{Stdout: []byte("true\n")}
	case "user.signingkey":
		return Result{Stdout: []byte("private-key-id\n")}
	default:
		return Result{}
	}
}
