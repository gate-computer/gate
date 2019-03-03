// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package file

import (
	"log"
	"runtime"
)

type File struct {
	fd int
}

func New(fd int) *File {
	f := &File{fd}
	runtime.SetFinalizer(f, finalizeFile)
	return f
}

func finalizeFile(f *File) {
	if f.fd >= 0 {
		log.Print("unreachable file")
		f.Close()
	}
}

func (f *File) Close() (err error) {
	err = closeFD(f.fd)
	f.fd = -1
	return
}

func (f *File) Fd() uintptr                                 { return uintptr(f.fd) }
func (f *File) Read(b []byte) (int, error)                  { return read(f.fd, b) }
func (f *File) ReadAt(b []byte, offset int64) (int, error)  { return pread(f.fd, b, offset) }
func (f *File) WriteAt(b []byte, offset int64) (int, error) { return pwrite(f.fd, b, offset) }
