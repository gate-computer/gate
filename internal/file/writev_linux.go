// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package file

import (
	"fmt"
	"io"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

func writev(fd int, iov []syscall.Iovec) error {
	return pwritev(fd, iov, -1)
}

// pwritev actually calls pwritev2 so offset may be -1.
func pwritev(fd int, iov []syscall.Iovec, offset int64) (err error) {
	for {
		var total uint64
		for _, span := range iov {
			total += span.Len
		}
		if total == 0 {
			return
		}

		n, _, errno := syscall.Syscall6(unix.SYS_PWRITEV2, uintptr(fd), uintptr(unsafe.Pointer(&iov[0])), uintptr(len(iov)), uintptr(offset), 0, 0)

		switch errno {
		case 0:
			switch uint64(n) {
			case total:
				return

			case 0:
				err = io.EOF
				return
			}

		case syscall.EAGAIN, syscall.EINTR:
			continue

		default:
			err = fmt.Errorf("pwritev2: %v", errno)
			return
		}

		for {
			if total >= iov[0].Len {
				total -= iov[0].Len
				iov = iov[1:]
			} else {
				span := iov[0]
				span.Len = iov[0].Len - total
				iov = append([]syscall.Iovec{span}, iov[1:]...)
				break
			}
		}
	}
}
