// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build android

package image

// Android doesn't allow passing memfds via unix sockets; use ashmem instead.
// See memory_memfd.go for function contract documentation.

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"gate.computer/internal/file"
	"golang.org/x/sys/unix"
)

const memoryFileWriteSupported = false // Ashmem doesn't implement write syscalls.

func newMemoryFile(name string, size int64) (f *file.File, err error) {
	f, err = openat(unix.AT_FDCWD, "/dev/ashmem", syscall.O_RDWR, 0)
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			f.Close()
		}
	}()

	err = ashmemSetSize.ioctl(f.Fd(), uintptr(size))
	if err != nil {
		return
	}

	return
}

func protectFileMemory(f *file.File, mask uintptr) error {
	return ashmemSetProtMask.ioctl(f.Fd(), mask)
}

type (
	ioctlSetInt uintptr
	ioctlPair   uintptr
)

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

func (cmd ioctlSetInt) ioctl(fd, arg uintptr) error {
	return ioctl(fd, uintptr(cmd), arg)
}

func (cmd ioctlPair) ioctl(fd uintptr, offset, len uint32) error {
	arg := [2]uint32{
		offset,
		len,
	}
	err := ioctl(fd, uintptr(cmd), uintptr(unsafe.Pointer(&arg[0])))
	runtime.KeepAlive(&arg)
	return err
}

func ioctl(fd, cmd, arg uintptr) error {
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, cmd, arg); errno != 0 {
		return fmt.Errorf("ioctl %s: %w", ioctlStrings[cmd], error(errno))
	}

	return nil
}
