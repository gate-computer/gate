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

func openat(dirfd int, path string, flags int, mode uint32) (*file.File, error) {
	fd, err := syscall.Openat(dirfd, path, flags|unix.O_CLOEXEC, mode)
	if err != nil {
		if err == syscall.ENOENT {
			return nil, os.ErrNotExist
		}

		return nil, fmt.Errorf("openat %d %q 0x%x: %w", dirfd, path, flags, err)
	}

	return file.New(fd), nil
}
