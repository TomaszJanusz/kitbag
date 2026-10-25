// Command kitbag manages coding-agent configuration across machines.
//
// Exit codes: 0 success, 1 failure, 2 usage error, 5 unsupported operating
// system (Windows before 1.x; see docs/platform.md).
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/TomaszJanusz/kitbag/internal/platform"
)

const (
	version     = "dev"
	usage       = "usage: kitbag [--git-backend=auto|system|embedded] version"
	exitFailure = 1
	exitUsage   = 2
	// exitUnsupportedOS follows the planned 0-4 codes of the apply engine.
	exitUnsupportedOS = 5
)

func main() {
	os.Exit(run(runtime.GOOS, newHost(), os.Args[1:], os.Stdout, os.Stderr))
}

// run executes the CLI for the operating system goos, a runtime.GOOS value,
// in the environment env.
func run(goos string, env platform.Env, args []string, stdout, stderr io.Writer) int {
	if err := platform.CheckOS(goos); err != nil {
		return report(stderr, exitUnsupportedOS, err)
	}

	flags, args, err := parseGlobalFlags(args)
	if err != nil {
		if code := report(stderr, exitUsage, err); code != exitUsage {
			return code
		}
		return reportLine(stderr, exitUsage, usage)
	}
	if len(args) != 1 || args[0] != "version" {
		return reportLine(stderr, exitUsage, usage)
	}
	sel, err := selectGitBackend(flags, env)
	if err != nil {
		return report(stderr, exitUsage, err)
	}
	if err := checkGitBackend(context.Background(), sel, env); err != nil {
		return report(stderr, exitFailure, err)
	}

	if _, err := fmt.Fprintln(stdout, "kitbag", version); err != nil {
		// A failed diagnostic cannot be reported recursively; signal it by exit code.
		return report(stderr, exitFailure, err)
	}

	return 0
}

// report writes "kitbag: " and args to stderr and returns code, or
// exitFailure when even the diagnostic cannot be written.
func report(stderr io.Writer, code int, args ...any) int {
	if _, err := fmt.Fprintln(stderr, append([]any{"kitbag:"}, args...)...); err != nil {
		return exitFailure
	}
	return code
}

// reportLine writes line to stderr and returns code, or exitFailure when the
// line cannot be written.
func reportLine(stderr io.Writer, code int, line string) int {
	if _, err := fmt.Fprintln(stderr, line); err != nil {
		return exitFailure
	}
	return code
}
