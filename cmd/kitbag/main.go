// Command kitbag manages coding-agent configuration across machines.
//
// Exit codes: 0 success, 1 failure, 2 usage error, 5 unsupported operating
// system (Windows before 1.x; see docs/platform.md).
package main

import (
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/TomaszJanusz/kitbag/internal/platform"
)

const (
	version     = "dev"
	exitFailure = 1
	exitUsage   = 2
	// exitUnsupportedOS follows the planned 0-4 codes of the apply engine.
	exitUnsupportedOS = 5
)

func main() {
	os.Exit(run(runtime.GOOS, os.Args[1:], os.Stdout, os.Stderr))
}

// run executes the CLI for the operating system goos, a runtime.GOOS value.
func run(goos string, args []string, stdout, stderr io.Writer) int {
	if err := platform.CheckOS(goos); err != nil {
		if _, writeErr := fmt.Fprintln(stderr, "kitbag:", err); writeErr != nil {
			return exitFailure
		}
		return exitUnsupportedOS
	}

	if len(args) != 1 || args[0] != "version" {
		if _, err := fmt.Fprintln(stderr, "usage: kitbag version"); err != nil {
			return exitFailure
		}
		return exitUsage
	}

	if _, err := fmt.Fprintln(stdout, "kitbag", version); err != nil {
		// A failed diagnostic cannot be reported recursively; signal it by exit code.
		if _, writeErr := fmt.Fprintln(stderr, "kitbag:", err); writeErr != nil {
			return exitFailure
		}
		return exitFailure
	}

	return 0
}
