// Command kitbag manages coding-agent configuration across machines.
package main

import (
	"fmt"
	"io"
	"os"
)

const (
	version     = "dev"
	exitFailure = 1
	exitUsage   = 2
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
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
