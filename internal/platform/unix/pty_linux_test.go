package unix

import (
	"os"
	"testing"
)

// openTerminal returns a pseudo-terminal master, which answers terminal
// attribute requests on Linux.
func openTerminal(t *testing.T) *os.File {
	t.Helper()
	return openFile(t, "/dev/ptmx")
}
