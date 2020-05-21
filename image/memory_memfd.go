// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build !android

package image

import (
	"gate.computer/gate/internal/file"
)

const memoryFileWriteSupported = true

// newMemoryFile with the given size.  name can used if a name is needed.  The
// file must initially be readable, writable, and executable.
func newMemoryFile(name string, size int64) (f *file.File, err error) {
	f, err = memfdCreate(name)
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			f.Close()
		}
	}()

	err = ftruncate(f.Fd(), size)
	if err != nil {
		return
	}

	return
}

// protectFileMemory on a best-effort basis.  mask is a combination of
// syscall.PROT_* flags.
func protectFileMemory(f *file.File, mask uintptr) (err error) {
	return
}
