//go:build darwin

package unix

import (
	"io/fs"
	"os"

	"golang.org/x/sys/unix"
)

func peerOwner(fd uintptr) error {
	cred, err := unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
	if err != nil {
		return err
	}
	if int64(cred.Uid) != int64(os.Geteuid()) {
		return fs.ErrPermission
	}
	return nil
}
