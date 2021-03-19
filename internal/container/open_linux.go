// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package container

import (
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func openPath(path string, flags int) (*os.File, error) {
	fd, err := syscall.Open(path, flags|unix.O_PATH|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}

	return os.NewFile(uintptr(fd), path), nil
}
