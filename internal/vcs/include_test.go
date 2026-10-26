package vcs_test

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

type globalConfig struct {
	readErr, addErr      error
	files, values, added []string
}

func (c *globalConfig) GlobalConfigFiles() []string                      { return c.files }
func (c *globalConfig) GlobalIncludes(context.Context) ([]string, error) { return c.values, c.readErr }

func (c *globalConfig) AddGlobalInclude(_ context.Context, path string) error {
	if c.addErr != nil {
		return c.addErr
	}
	c.added = append(c.added, path)
	c.values = append(c.values, path)
	return nil
}

func includePath(parts ...string) string {
	return filepath.Join(append([]string{string(filepath.Separator)}, parts...)...)
}

func TestEnsureIncludesNormalizesAndRepeats(t *testing.T) {
	t.Parallel()
	home := includePath("home", "user")
	first, second := filepath.Join(home, "repo", "first"), filepath.Join(home, "repo", "second")
	cfg := &globalConfig{files: []string{filepath.Join(home, ".gitconfig")}, values: []string{"~/repo/./first", "~/unrelated"}}
	h := fake.New()
	for range 2 {
		result, err := vcs.EnsureIncludes(t.Context(), cfg, h, home, []string{first, second, second})
		if err != nil || len(result.Warnings) != 0 {
			t.Fatalf("result = %+v, err = %v", result, err)
		}
	}
	if !slices.Equal(cfg.added, []string{second}) {
		t.Fatalf("added = %v, want second once", cfg.added)
	}
	if !slices.Contains(cfg.values, "~/unrelated") {
		t.Fatal("unrelated include removed")
	}
}

func TestEnsureIncludesResolvesLinks(t *testing.T) {
	t.Parallel()
	home := includePath("home", "user")
	data := includePath("data", "agent-config")
	include := filepath.Join(home, "src", "agent-config", "git", "agent.gitconfig")
	h := fake.New()
	for _, dir := range []string{filepath.Join(data, "git"), filepath.Join(home, "src"), includePath("links")} {
		if err := h.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for name, target := range map[string]string{
		// ~/src/agent-config -> /data/agent-config through a relative link.
		filepath.Join(home, "src", "agent-config"): filepath.Join("..", "..", "..", "data", "agent-config"),
		// /links/clone -> /links/hop -> /data/agent-config: a chain.
		includePath("links", "clone"): "hop",
		includePath("links", "hop"):   data,
		// A loop never resolves and never equals anything else.
		includePath("links", "loop"): "loop",
	} {
		if err := h.Link(target, name); err != nil {
			t.Fatal(err)
		}
	}
	for name, tc := range map[string]struct {
		existing string
		added    bool
	}{
		"resolved spelling":      {existing: filepath.Join(data, "git", "agent.gitconfig")},
		"tilde spelling":         {existing: "~/src/agent-config/git/agent.gitconfig"},
		"link chain":             {existing: includePath("links", "clone", "git", "agent.gitconfig")},
		"link loop":              {existing: includePath("links", "loop", "git", "agent.gitconfig"), added: true},
		"other file":             {existing: filepath.Join(data, "git", "other.gitconfig"), added: true},
		"relative include value": {existing: "git/agent.gitconfig", added: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cfg := &globalConfig{files: []string{filepath.Join(home, ".gitconfig")}, values: []string{tc.existing}}
			result, err := vcs.EnsureIncludes(t.Context(), cfg, h, home, []string{include})
			if err != nil || len(result.Warnings) != 0 {
				t.Fatalf("result = %+v, err = %v", result, err)
			}
			if added := slices.Equal(cfg.added, []string{include}); added != tc.added {
				t.Fatalf("added = %v; want added %t", cfg.added, tc.added)
			}
			if cfg.values[0] != tc.existing {
				t.Fatalf("existing include changed: %v", cfg.values)
			}
		})
	}
}

func TestEnsureIncludesManagedConfig(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"symlink", "read-only", "directory"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			h := fake.New()
			home := includePath("home")
			file, include := filepath.Join(home, ".gitconfig"), filepath.Join(home, "repo", "gitconfig")
			makeConfig(t, h, file, kind)
			cfg := &globalConfig{files: []string{file}, values: []string{"~/repo/gitconfig"}}
			result, err := vcs.EnsureIncludes(t.Context(), cfg, h, home, []string{include})
			if err != nil || len(result.Warnings) != 0 || len(cfg.added) != 0 {
				t.Fatalf("present include: %+v, %v", result, err)
			}
			cfg.values = nil
			result, err = vcs.EnsureIncludes(t.Context(), cfg, h, home, []string{include})
			if err != nil || len(result.Warnings) != 1 || len(cfg.added) != 0 {
				t.Fatalf("missing include: %+v, %v", result, err)
			}
			if !strings.Contains(result.Warnings[0], file) || !strings.Contains(result.Warnings[0], include) {
				t.Fatalf("warning = %q", result.Warnings[0])
			}
		})
	}
}

func makeConfig(t *testing.T, h *fake.Host, file, kind string) {
	t.Helper()
	if err := h.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		t.Fatal(err)
	}
	var err error
	switch kind {
	case "symlink":
		err = h.Link(filepath.Join(filepath.Dir(file), "dotfiles"), file)
	case "read-only":
		err = h.WriteAtomic(file, []byte("[user]\n"), 0o400)
	case "directory":
		err = h.MkdirAll(file, 0o700)
	}
	if err != nil {
		t.Fatal(err)
	}
}

func TestEnsureIncludesSelectsExistingXDGConfig(t *testing.T) {
	t.Parallel()
	h := fake.New()
	home := includePath("home")
	xdg := filepath.Join(home, ".config", "git", "config")
	makeConfig(t, h, xdg, "read-only")
	cfg := &globalConfig{files: []string{filepath.Join(home, ".gitconfig"), xdg}}
	result, err := vcs.EnsureIncludes(t.Context(), cfg, h, home, []string{filepath.Join(home, "include")})
	if err != nil || len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], xdg) || len(cfg.added) != 0 {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
}

func TestEnsureIncludesErrors(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("config failure")
	home := includePath("home")
	for _, tt := range []struct {
		cfg   globalConfig
		want  error
		name  string
		home  string
		paths []string
	}{
		{name: "read", cfg: globalConfig{readErr: sentinel}, home: home, want: sentinel},
		{name: "add", cfg: globalConfig{files: []string{filepath.Join(home, ".gitconfig")}, addErr: sentinel}, home: home, paths: []string{filepath.Join(home, "include")}, want: sentinel},
		{name: "relative home", home: "home", want: vcs.ErrInvalidArgument},
		{name: "relative include", home: home, paths: []string{"include"}, want: vcs.ErrInvalidArgument},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := vcs.EnsureIncludes(t.Context(), &tt.cfg, fake.New(), tt.home, tt.paths)
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
	}
}
