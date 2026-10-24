//go:build darwin || linux

package unix

import (
	"math"

	sysunix "golang.org/x/sys/unix"
)

// IsTerminal reports whether fd refers to a terminal, by asking for its
// terminal attributes as isatty(3) does.
func (*Host) IsTerminal(fd uintptr) bool {
	if fd > math.MaxInt32 {
		return false
	}
	_, err := sysunix.IoctlGetTermios(int(fd), ioctlReadTermios)
	return err == nil
}
