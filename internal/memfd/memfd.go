package memfd

import (
	"reflect"
	"syscall"
	"unsafe"
)

type Flags uint

const (
	CLOEXEC       = Flags(0x0001)
	ALLOW_SEALING = Flags(0x0002)
)

func Create(name string, flags Flags) (fd int, err error) {
	var nameBuf []byte
	if name == "" {
		nameBuf = []byte{0}
	} else {
		nameBuf = append([]byte(name), 0)
	}
	ret, _, err := syscall.Syscall(_SYS_memfd_create, (*reflect.StringHeader)(unsafe.Pointer(&nameBuf)).Data, uintptr(flags), 0)
	keepAlive(nameBuf)
	if errno, ok := err.(syscall.Errno); ok && errno == 0 {
		err = nil
	}
	if err == nil {
		fd = int(ret)
	} else {
		fd = -1
	}
	return
}
