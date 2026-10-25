package vcs_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

func TestWorkingCopyDir(t *testing.T) {
	t.Parallel()

	home := filepath.FromSlash("/home/me")
	for _, tt := range []struct {
		env  map[string]string
		want string
	}{
		{map[string]string{"HOME": home}, filepath.Join(home, ".local", "share", "kitbag", "repos", "agent-config")},
		{
			map[string]string{"HOME": home, "XDG_DATA_HOME": filepath.FromSlash("/data")},
			filepath.Join(filepath.FromSlash("/data"), "kitbag", "repos", "agent-config"),
		},
	} {
		got, err := vcs.WorkingCopyDir(fake.New(fake.WithEnv(tt.env)), "agent-config")
		if err != nil || got != tt.want {
			t.Errorf("WorkingCopyDir(%v) = %q, %v; want %q", tt.env, got, err, tt.want)
		}
	}

	if _, err := vcs.WorkingCopyDir(fake.New(), "agent-config"); err == nil {
		t.Error("WorkingCopyDir without HOME succeeded")
	}
	env := fake.New(fake.WithEnv(map[string]string{"HOME": home}))
	for _, name := range []string{"", ".", "..", "a/b", `a\b`, "../x", ".hidden"} {
		if _, err := vcs.WorkingCopyDir(env, name); !errors.Is(err, vcs.ErrInvalidArgument) {
			t.Errorf("WorkingCopyDir(%q) error = %v, want ErrInvalidArgument", name, err)
		}
	}
}

func TestRepoName(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		url, want string
	}{
		{"git@github.com:TomaszJanusz/agent-config.git", "agent-config"},
		{"https://github.com/TomaszJanusz/agent-config", "agent-config"},
		{"https://github.com/TomaszJanusz/agent-config.git/", "agent-config"},
		{"ssh://git@host:22/srv/configs.git", "configs"},
		{"host:repo", "repo"},
		{"/srv/git/dotfiles.git", "dotfiles"},
		{`C:\repos\dotfiles.git`, "dotfiles"},
	} {
		if got, err := vcs.RepoName(tt.url); err != nil || got != tt.want {
			t.Errorf("RepoName(%q) = %q, %v; want %q", tt.url, got, err, tt.want)
		}
	}
	for _, url := range []string{"", "/", "https://host/.git", "https://host/..", "host:"} {
		if got, err := vcs.RepoName(url); !errors.Is(err, vcs.ErrInvalidArgument) {
			t.Errorf("RepoName(%q) = %q, %v; want ErrInvalidArgument", url, got, err)
		}
	}
}
