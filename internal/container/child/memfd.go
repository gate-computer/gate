// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package child

import (
	"fmt"
	"syscall"

	"golang.org/x/sys/unix"
)

func memfdCreateDup(name string, data []byte, asFD, dupFlags int) error {
	fd, err := unix.MemfdCreate(name, unix.MFD_ALLOW_SEALING|unix.MFD_CLOEXEC)
	if err != nil {
		return fmt.Errorf("creating memfd for %s: %w", name, err)
	}
	defer syscall.Close(fd)

	if _, err := syscall.Pwrite(fd, data, 0); err != nil {
		return fmt.Errorf("writing %s: %w", name, err)
	}

	if err := syscall.Dup3(fd, asFD, dupFlags); err != nil {
		return fmt.Errorf("duplicating memfd file descriptor of %s: %w", name, err)
	}

	return nil
}
