package memfd

import (
	"syscall"
)

type FcntlCmd int

type SealFlags int

const (
	F_SEAL_SEAL   = SealFlags(0x0001)
	F_SEAL_SHRINK = SealFlags(0x0002)
	F_SEAL_GROW   = SealFlags(0x0004)
	F_SEAL_WRITE  = SealFlags(0x0008)
)

func Fcntl(fd int, cmd FcntlCmd, arg SealFlags) (flags SealFlags, err error) {
	ret, _, err := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), uintptr(cmd), uintptr(arg))
	if errno, ok := err.(syscall.Errno); ok && errno == 0 {
		err = nil
	}
	flags = SealFlags(ret)
	return
}
