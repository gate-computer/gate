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

func linkat(olddirfd int, oldpath string, newdirfd int, newpath string, flags int) (err error) {
	err = unix.Linkat(olddirfd, oldpath, newdirfd, newpath, flags)
	if err != nil {
		if err == syscall.EEXIST {
			err = os.ErrExist
			return
		}
		if err == syscall.ENOENT {
			err = os.ErrNotExist
			return
		}

		err = fmt.Errorf("linkat %d %q %d %q %#x: %w", olddirfd, oldpath, newdirfd, newpath, flags, err)
		return
	}

	return
}

func linkTempFile(fd, newdirfd uintptr, newpath string) error {
	oldpath := fmt.Sprintf("/proc/self/fd/%d", fd)
	return linkat(unix.AT_FDCWD, oldpath, int(newdirfd), newpath, unix.AT_SYMLINK_FOLLOW)
}
