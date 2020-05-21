// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"fmt"
	"os"
	"syscall"

	"gate.computer/gate/internal/file"
	"golang.org/x/sys/unix"
)

func openat(dirfd int, path string, flags int, mode uint32) (f *file.File, err error) {
	fd, err := syscall.Openat(dirfd, path, flags|unix.O_CLOEXEC, mode)
	if err != nil {
		if err == syscall.ENOENT {
			err = os.ErrNotExist
			return
		}

		err = fmt.Errorf("openat %d %q 0x%x: %v", dirfd, path, flags, err)
		return
	}

	f = file.New(fd)
	return
}
