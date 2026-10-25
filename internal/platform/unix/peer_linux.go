//go:build linux

package unix

import (
	"io/fs"
	"os"

	"golang.org/x/sys/unix"
)

func peerOwner(fd uintptr) error {
	cred, err := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	if err != nil {
		return err
	}
	if int64(cred.Uid) != int64(os.Geteuid()) {
		return fs.ErrPermission
	}
	return nil
}
