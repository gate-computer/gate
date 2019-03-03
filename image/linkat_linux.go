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

		err = fmt.Errorf("linkat %q: %v", newpath, err)
		return
	}

	return
}

func linkTempFile(fd uintptr, newPath string) (err error) {
	oldPath := fmt.Sprintf("/proc/self/fd/%d", fd)

	err = linkat(unix.AT_FDCWD, oldPath, unix.AT_FDCWD, newPath, unix.AT_SYMLINK_FOLLOW)
	if err != nil {
		return
	}

	return
}
