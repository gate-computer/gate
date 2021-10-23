// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package file

import (
	"fmt"
	"io"
	"log"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

type File struct {
	fd   int
	file string
	line int
}

func New(fd int) *File {
	_, file, line, ok := runtime.Caller(2)
	if !ok {
		log.Print("failed to discover caller of file.New")
	}
	f := &File{fd, file, line}
	runtime.SetFinalizer(f, (*File).finalize)
	return f
}

func (f *File) finalize() {
	if f.fd >= 0 {
		log.Printf("closing unreachable file descriptor %d created at %s:%d", f.fd, f.file, f.line)
		f.Close()
	}
}

func (f *File) Close() error {
	fd := f.fd
	f.fd = -1
	if err := syscall.Close(fd); err != nil {
		return fmt.Errorf("close: %w", err)
	}
	return nil
}

func (f *File) FD() int     { return f.fd }
func (f *File) Fd() uintptr { return uintptr(f.fd) }

func (f *File) Read(b []byte) (n int, err error) {
	if len(b) == 0 {
		return
	}

	for {
		n, err = syscall.Read(f.fd, b)

		switch err {
		case nil:
			if n == 0 {
				err = io.EOF
			}
			return

		case syscall.EAGAIN, syscall.EINTR:
			continue

		default:
			err = fmt.Errorf("read: %w", err)
			return
		}
	}
}

func (f *File) ReadAt(b []byte, offset int64) (n int, err error) {
	if len(b) == 0 {
		return
	}

	for {
		n, err = syscall.Pread(f.fd, b, offset)

		switch err {
		case nil:
			if n == 0 {
				err = io.EOF
			}
			return

		case syscall.EAGAIN, syscall.EINTR:
			continue

		default:
			err = fmt.Errorf("pread: %w", err)
			return
		}
	}
}

func (f *File) WriteAt(b []byte, offset int64) (n int, err error) {
	for len(b) > 0 {
		count, err := syscall.Pwrite(f.fd, b, offset)

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
			return n, fmt.Errorf("pwrite: %w", err)
		}
	}

	return
}

func (f *File) WriteVec(bufs [2][]byte) error {
	return f.WriteVecAt(bufs, -1)
}

func (f *File) WriteVecAt(bufs [2][]byte, offset int64) (err error) {
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

		n, _, errno := syscall.Syscall6(unix.SYS_PWRITEV2, uintptr(f.fd), uintptr(unsafe.Pointer(&iov[0])), n, uintptr(offset), 0, 0)

		switch errno {
		case 0:
			if n == 0 {
				err = io.EOF
				return
			}

		case syscall.EAGAIN, syscall.EINTR:
			continue

		default:
			err = fmt.Errorf("pwritev2: %w", error(errno))
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
