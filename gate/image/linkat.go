// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func linkat(olddirfd int, oldpath string, newdirfd int, newpath string, flags int) error {
	if err := unix.Linkat(olddirfd, oldpath, newdirfd, newpath, flags); err != nil {
		if err == syscall.EEXIST {
			return os.ErrExist
		}
		if err == syscall.ENOENT {
			return os.ErrNotExist
		}

		return fmt.Errorf("linkat %d %q %d %q %#x: %w", olddirfd, oldpath, newdirfd, newpath, flags, err)
	}

	return nil
}

func linkTempFile(fd, newdirfd uintptr, newpath string) error {
	oldpath := fmt.Sprintf("/proc/self/fd/%d", fd)
	return linkat(unix.AT_FDCWD, oldpath, int(newdirfd), newpath, unix.AT_SYMLINK_FOLLOW)
}
