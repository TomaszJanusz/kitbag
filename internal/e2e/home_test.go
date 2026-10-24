package e2e

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

func TestCheckHome(t *testing.T) {
	t.Parallel()

	work := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(work, "escape")); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		want  error
		name  string
		home  string
		work  string
		homes []string
	}{
		{name: "throwaway home", home: filepath.Join(work, "home"), work: work},
		{name: "work itself", home: work, work: work},
		{name: "relative home", home: "home", work: work, want: ErrUnsafeHome},
		{name: "empty home", work: work, want: ErrUnsafeHome},
		{name: "outside work", home: outside, work: work, want: ErrUnsafeHome},
		{name: "parent of work", home: filepath.Join(work, ".."), work: work, want: ErrUnsafeHome},
		{name: "symlink out of work", home: filepath.Join(work, "escape"), work: work, want: ErrUnsafeHome},
		// The work directory contains the "real" home here, so only the
		// real-home comparison can refuse it.
		{name: "real home inside work", home: filepath.Join(work, "home"), work: work, homes: []string{filepath.Join(work, "home")}, want: ErrRealHome},
		{name: "real home via symlink", home: filepath.Join(work, "escape"), work: work, homes: []string{outside}, want: ErrRealHome},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if err := CheckHome(tt.home, tt.work, tt.homes); !errors.Is(err, tt.want) {
				t.Fatalf("CheckHome(%q, %q, %q) = %v, want %v", tt.home, tt.work, tt.homes, err, tt.want)
			}
		})
	}
}

func TestCheckHomeRefusesTheRealHome(t *testing.T) {
	t.Parallel()

	homes := RealHomes()
	if len(homes) == 0 {
		t.Fatal("no real home directory found to test against")
	}
	for _, home := range homes {
		// "/" contains every home, so the containment check cannot refuse it.
		if err := CheckHome(home, string(filepath.Separator), homes); !errors.Is(err, ErrRealHome) {
			t.Errorf("CheckHome(%q) = %v, want %v", home, err, ErrRealHome)
		}
	}
	if err := CheckHome(filepath.Join(t.TempDir(), "home"), os.TempDir(), homes); err != nil {
		t.Errorf("CheckHome(temporary home) = %v, want nil", err)
	}
}

func TestSetup(t *testing.T) {
	t.Parallel()

	work := t.TempDir()
	env := &testscript.Env{WorkDir: work, Values: map[any]any{}}
	if err := setup(env, RealHomes()); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(work, "home")
	if got := env.Getenv("HOME"); got != home {
		t.Errorf("HOME = %q, want %q", got, home)
	}
	if info, err := os.Stat(home); err != nil || !info.IsDir() {
		t.Errorf("HOME is not a directory: %v", err)
	}
	if got := env.Getenv("GIT_CONFIG_NOSYSTEM"); got != "1" {
		t.Errorf("GIT_CONFIG_NOSYSTEM = %q, want 1", got)
	}
}

func TestSetupRefusesARealHome(t *testing.T) {
	t.Parallel()

	work := t.TempDir()
	env := &testscript.Env{WorkDir: work, Values: map[any]any{}}
	home := filepath.Join(work, "home")
	if err := setup(env, []string{home}); !errors.Is(err, ErrRealHome) {
		t.Fatalf("setup = %v, want %v", err, ErrRealHome)
	}
	if _, err := os.Stat(home); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("setup created HOME before refusing: %v", err)
	}
	if got := env.Getenv("HOME"); got != "" {
		t.Errorf("HOME = %q after refusal, want unset", got)
	}
}
