// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

type ioctlSetInt uintptr
type ioctlPair uintptr

const (
	ashmemSetSize     = ioctlSetInt(0x40087703)
	ashmemSetProtMask = ioctlSetInt(0x40087705)
	ashmemPin         = ioctlPair(0x40087707)
)

var ioctlStrings = map[uintptr]string{
	uintptr(ashmemSetSize):     "ASHMEM_SET_SIZE",
	uintptr(ashmemSetProtMask): "ASHMEM_SET_PROT_MASK",
	uintptr(ashmemPin):         "ASHMEM_PIN",
}

func (cmd ioctlSetInt) ioctl(fd uintptr, arg uintptr) error {
	return ioctl(fd, uintptr(cmd), arg)
}

func (cmd ioctlPair) ioctl(fd uintptr, offset, len uint32) (err error) {
	arg := [2]uint32{
		offset,
		len,
	}
	err = ioctl(fd, uintptr(cmd), uintptr(unsafe.Pointer(&arg[0])))
	runtime.KeepAlive(&arg)
	return
}

func ioctl(fd, cmd, arg uintptr) (err error) {
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, cmd, arg); errno != 0 {
		err = fmt.Errorf("ioctl %s: %v", ioctlStrings[cmd], errno)
		return
	}

	return
}
