// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package executable

import (
	"io"
	"log"
	"os"
	"runtime"
	"sync/atomic"
)

type Ref interface {
	io.Closer
	ref() // Can only be implemented in this package.
}

type FileRef struct {
	*os.File

	refCount int32 // Atomic
}

func NewFileRef(f *os.File) *FileRef {
	ref := &FileRef{
		File:     f,
		refCount: 1,
	}
	runtime.SetFinalizer(ref, finalizeFileRef)
	return ref
}

func finalizeFileRef(ref *FileRef) {
	if ref.refCount != 0 {
		log.Printf("unreachable executable file with reference count %d", ref.refCount)
		if ref.refCount > 0 {
			ref.cleanup()
		}
	}
}

func (ref *FileRef) cleanup() (err error) {
	err = ref.File.Close()
	ref.File = nil
	return
}

// Ref increments reference count.
func (ref *FileRef) Ref() *FileRef {
	atomic.AddInt32(&ref.refCount, 1)
	return ref
}

// Close decrements reference count.  File is closed when reference count drops
// to zero.
func (ref *FileRef) Close() (err error) {
	switch n := atomic.AddInt32(&ref.refCount, -1); {
	case n == 0:
		err = ref.cleanup()

	case n < 0:
		panic("executable file reference count is negative")
	}

	return
}

func (*FileRef) ref() {}
