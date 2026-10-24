package unix

import sysunix "golang.org/x/sys/unix"

// ioctlReadTermios is the request that reads terminal attributes on Linux.
const ioctlReadTermios = sysunix.TCGETS
