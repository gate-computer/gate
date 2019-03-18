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

func openat(dirfd int, path string, flags int, mode uint32) (f *file.File, err error) {
	fd, err := syscall.Openat(dirfd, path, flags|unix.O_CLOEXEC, mode)
	if err != nil {
		err = fmt.Errorf("openat %d %q: %v", dirfd, path, err)
		return
	}

	f = file.New(fd)
	return
}
