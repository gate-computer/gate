// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package file

import (
	"log"
	"runtime"
	"sync/atomic"
)

type countedFile struct {
	File
	count int32 // Atomic.
}

func (f *countedFile) finalize() {
	if f.count != 0 {
		log.Printf("unreachable file descriptor %d with reference count %d", f.fd, f.count)
	}
	f.File.finalize()
}

type Ref struct {
	file *countedFile
}

func Own(fd int) Ref {
	_, file, line, ok := runtime.Caller(2)
	if !ok {
		log.Print("failed to discover caller of file.Own")
	}
	f := &countedFile{
		File:  File{fd, file, line},
		count: 1,
	}
	runtime.SetFinalizer(f, (*countedFile).finalize)
	return Ref{f}
}

// MustRef increments reference count or panics.
func (ref *Ref) MustRef() Ref {
	if atomic.AddInt32(&ref.file.count, 1) <= 1 {
		panic("referencing unreferenced file")
	}
	return Ref{ref.file}
}

// Ref increments reference count if there is a referenced file.
func (ref *Ref) Ref() Ref {
	if ref.file == nil {
		return Ref{}
	}
	return ref.MustRef()
}

// Unref decrements reference count.  File is closed when reference count drops
// to zero.
func (ref *Ref) Unref() {
	f := ref.file
	if f == nil {
		return
	}
	ref.file = nil

	switch n := atomic.AddInt32(&f.count, -1); {
	case n == 0:
		f.Close()
	case n < 0:
		panic("file reference count is negative")
	}
}

// File gets a temporary pointer to the referenced file, or nil.
func (ref *Ref) File() *File {
	if ref.file == nil {
		return nil
	}
	return &ref.file.File
}
