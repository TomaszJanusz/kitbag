package unix

import (
	"fmt"
	"os"
	"syscall"
	"testing"

	sysunix "golang.org/x/sys/unix"
)

// openTerminal returns the slave side of a new pseudo-terminal: on macOS the
// master does not answer terminal attribute requests. The slave is named
// after the master's device minor, as ptsname(3) does.
func openTerminal(t *testing.T) *os.File {
	t.Helper()
	master := openFile(t, "/dev/ptmx")
	fd := int(master.Fd())
	for _, req := range []uint{sysunix.TIOCPTYGRANT, sysunix.TIOCPTYUNLK} {
		if err := sysunix.IoctlSetInt(fd, req, 0); err != nil {
			t.Fatalf("prepare pseudo-terminal: %v", err)
		}
	}
	fi, err := master.Stat()
	if err != nil {
		t.Fatal(err)
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatalf("stat of %s has no device", master.Name())
	}
	name := fmt.Sprintf("/dev/ttys%03d", sysunix.Minor(uint64(st.Rdev))) //nolint:gosec // G115: device numbers are non-negative.
	// Fail rather than skip: the master exists, so a missing slave means the
	// naming assumption is wrong and the positive check would go unverified.
	slave, err := os.OpenFile(name, os.O_RDWR|syscall.O_NOCTTY, 0) //nolint:gosec // G304: pseudo-terminal device path.
	if err != nil {
		t.Fatalf("open pseudo-terminal slave: %v", err)
	}
	t.Cleanup(func() {
		if err := slave.Close(); err != nil {
			t.Error(err)
		}
	})
	return slave
}
