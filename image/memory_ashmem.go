// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build android

package image

// Android doesn't allow passing memfds via unix sockets; use ashmem instead.
// See memory_memfd.go for function contract documentation.

import (
	"syscall"

	"github.com/tsavola/gate/internal/file"
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
