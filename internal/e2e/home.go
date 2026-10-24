package e2e

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
)

const (
	// refusedExit is the exit status of a guarded command that refused to run.
	refusedExit = 125
	// privateDir and privateFile are the modes of directories and files the
	// harness creates for a script.
	privateDir  fs.FileMode = 0o700
	privateFile fs.FileMode = 0o600
)

var (
	// ErrRealHome reports that HOME is one of the invoking user's real home directories.
	ErrRealHome = errors.New("it is the real home directory")
	// ErrUnsafeHome reports that HOME is not a throwaway directory inside $WORK.
	ErrUnsafeHome = errors.New("it is not a throwaway directory inside $WORK")
)

// CheckHome returns an error unless home is an absolute path inside work that
// is none of realHomes. Symbolic links are resolved for paths that exist, so a
// link inside work cannot lead to a real home or outside work.
func CheckHome(home, work string, realHomes []string) error {
	if !filepath.IsAbs(home) || !filepath.IsAbs(work) {
		return fmt.Errorf("refusing to run with HOME=%q and WORK=%q: %w", home, work, ErrUnsafeHome)
	}
	for _, real := range realHomes {
		if real != "" && samePath(home, real) {
			return fmt.Errorf("refusing to run with HOME=%q: %w", home, ErrRealHome)
		}
	}
	if !within(work, home) {
		return fmt.Errorf("refusing to run with HOME=%q: %w", home, ErrUnsafeHome)
	}
	resolvedHome, homeErr := filepath.EvalSymlinks(home)
	resolvedWork, workErr := filepath.EvalSymlinks(work)
	if homeErr == nil && workErr == nil && !within(resolvedWork, resolvedHome) {
		return fmt.Errorf("refusing to run with HOME=%q (resolves to %q): %w", home, resolvedHome, ErrUnsafeHome)
	}
	return nil
}

// RealHomes returns the invoking user's home directories that are known: the
// account's home from the user database and the HOME of the current process.
// Call it from the test process, never from a command started by a script,
// whose HOME is the throwaway one.
func RealHomes() []string {
	homes := accountHome()
	if home, err := os.UserHomeDir(); err == nil {
		homes = append(homes, home)
	}
	return homes
}

// accountHome returns the home directory recorded for the current user in the
// user database, which does not depend on the HOME environment variable.
func accountHome() []string {
	account, err := user.Current()
	if err != nil || account.HomeDir == "" {
		return nil
	}
	return []string{account.HomeDir}
}

// Guarded wraps the main function of a command registered with
// testscript.Main. The wrapper refuses to run, with exit status 125, unless
// HOME is a throwaway directory inside the script's $WORK and is not the
// account's real home, even when a script overrides HOME after Setup.
func Guarded(mainFunc func()) func() {
	return func() {
		if err := CheckHome(os.Getenv("HOME"), os.Getenv("WORK"), accountHome()); err != nil {
			if _, writeErr := fmt.Fprintln(os.Stderr, "e2e harness:", err); writeErr != nil {
				// The refusal stands even when it cannot be reported.
				os.Exit(refusedExit)
			}
			os.Exit(refusedExit)
		}
		mainFunc()
	}
}

// samePath reports whether a and b name the same location, lexically or
// after resolving symbolic links.
func samePath(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	resolvedA, errA := filepath.EvalSymlinks(a)
	resolvedB, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && resolvedA == resolvedB
}

// within reports whether path is root or lexically below it.
func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && filepath.IsLocal(rel)
}
