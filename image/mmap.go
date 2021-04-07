// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"fmt"
	"syscall"
)

func mmap(fd int, offset int64, length, prot, flags int) ([]byte, error) {
	b, err := syscall.Mmap(fd, offset, length, prot, flags)
	if err != nil {
		return nil, fmt.Errorf("mmap: %w", err)
	}

	return b, err
}

func mustMunmap(b []byte) {
	if err := syscall.Munmap(b); err != nil {
		panic(fmt.Errorf("munmap: %w", err))
	}
}
