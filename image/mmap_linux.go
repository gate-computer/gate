// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"fmt"
	"syscall"
)

func mmap(fd uintptr, offset int64, length, prot, flags int) (data []byte, err error) {
	data, err = syscall.Mmap(int(fd), offset, length, prot, flags)
	if err != nil {
		err = fmt.Errorf("mmap: %v", err)
		return
	}

	return
}

func mustMunmap(b []byte) {
	if err := syscall.Munmap(b); err != nil {
		panic(fmt.Errorf("munmap: %v", err))
	}
}
