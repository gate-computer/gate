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

func writev(fd int, bufs [2][]byte) error {
	return pwritev(fd, bufs, -1)
}

// pwritev actually calls pwritev2 so offset may be -1.
func pwritev(fd int, bufs [2][]byte, offset int64) (err error) {
	bs := bufs[:]
	iov := make([]syscall.Iovec, 2)

	for {
		var n uintptr
		for _, b := range bs {
			if len(b) > 0 {
				iov[n].Base = &b[0]
				iov[n].SetLen(len(b))
				n++
			}
		}
		if n == 0 {
			return
		}

		n, _, errno := syscall.Syscall6(unix.SYS_PWRITEV2, uintptr(fd), uintptr(unsafe.Pointer(&iov[0])), n, uintptr(offset), 0, 0)

		switch errno {
		case 0:
			if n == 0 {
				err = io.EOF
				return
			}

		case syscall.EAGAIN, syscall.EINTR:
			continue

		default:
			err = fmt.Errorf("pwritev2: %v", errno)
			return
		}

		if offset >= 0 {
			offset += int64(n)
		}

		for {
			if n >= uintptr(len(bs[0])) {
				n -= uintptr(len(bs[0]))
				bs = bs[1:]
				if n == 0 && len(bs) == 0 {
					return
				}
			} else {
				bs[0] = bs[0][n:]
				break
			}
		}
	}
}
