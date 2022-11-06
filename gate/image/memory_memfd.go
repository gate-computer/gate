// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !android
// +build !android

package image

import (
	"fmt"
	"syscall"

	"gate.computer/internal/file"
	"golang.org/x/sys/unix"
)

const memoryFileWriteSupported = true

// newMemoryFile with the given size.  name can used if a name is needed.  The
// file must initially be readable, writable, and executable.
func newMemoryFile(name string, size int64) (*file.File, error) {
	fd, err := unix.MemfdCreate(name, unix.MFD_CLOEXEC)
	if err != nil {
		return nil, fmt.Errorf("memfd_create: %w", err)
	}
	defer func() {
		if fd >= 0 {
			syscall.Close(fd)
		}
	}()

	if err := ftruncate(fd, size); err != nil {
		return nil, err
	}

	f := file.New(fd)
	fd = -1
	return f, nil
}

// protectFileMemory on a best-effort basis.  mask is a combination of
// syscall.PROT_* flags.
func protectFileMemory(f *file.File, mask uintptr) (err error) {
	return
}
