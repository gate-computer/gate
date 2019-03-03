// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"fmt"
	"syscall"

	"github.com/tsavola/gate/internal/file"
	"golang.org/x/sys/unix"
)

func open(path string, mode int) (f *file.File, err error) {
	fd, err := syscall.Open(path, mode|unix.O_CLOEXEC, 0)
	if err != nil {
		err = fmt.Errorf("open %q: %v", path, err)
		return
	}

	f = file.New(fd)
	return
}

func openTempFile(path string, mode int, perm uint32) (f *file.File, err error) {
	fd, err := syscall.Open(path, mode|unix.O_TMPFILE|unix.O_CLOEXEC, perm)
	if err != nil {
		err = fmt.Errorf("open anonymous temporary file at %q: %v", path, err)
		return
	}

	f = file.New(fd)
	return
}
