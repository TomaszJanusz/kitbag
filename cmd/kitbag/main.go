// Command kitbag manages coding-agent configuration across machines.
package main

import (
	"fmt"
	"io"
	"os"
)

const version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 || args[0] != "version" {
		fmt.Fprintln(stderr, "usage: kitbag version")
		return 2
	}

	if _, err := fmt.Fprintln(stdout, "kitbag", version); err != nil {
		fmt.Fprintln(stderr, "kitbag:", err)
		return 1
	}

	return 0
}
