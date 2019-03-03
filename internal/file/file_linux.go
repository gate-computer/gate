// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package file

import (
	"fmt"
	"io"
	"syscall"
)

func closeFD(fd int) (err error) {
	err = syscall.Close(fd)
	if err != nil {
		err = fmt.Errorf("close: %v", err)
		return
	}

	return
}

func read(fd int, b []byte) (n int, err error) {
	if len(b) == 0 {
		return
	}

	for {
		n, err = syscall.Read(fd, b)

		switch err {
		case nil:
			if n == 0 {
				err = io.EOF
			}
			return

		case syscall.EAGAIN, syscall.EINTR:
			continue

		default:
			err = fmt.Errorf("read: %v", err)
			return
		}
	}
}

func pread(fd int, b []byte, offset int64) (n int, err error) {
	if len(b) == 0 {
		return
	}

	for {
		n, err = syscall.Pread(fd, b, offset)

		switch err {
		case nil:
			if n == 0 {
				err = io.EOF
			}
			return

		case syscall.EAGAIN, syscall.EINTR:
			continue

		default:
			err = fmt.Errorf("pread: %v", err)
			return
		}
	}
}

func pwrite(fd int, b []byte, offset int64) (n int, err error) {
	for len(b) > 0 {
		count, err := syscall.Pwrite(fd, b, offset)

		switch err {
		case nil:
			if count == 0 {
				return n, io.EOF
			}
			b = b[count:]
			offset += int64(count)
			n += count

		case syscall.EAGAIN, syscall.EINTR:
			continue

		default:
			return n, fmt.Errorf("pwrite: %v", err)
		}
	}

	return
}
