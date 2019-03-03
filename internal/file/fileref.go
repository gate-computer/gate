// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package file

import (
	"log"
	"runtime"
	"sync/atomic"
)

type Ref struct {
	File
	refCount int32 // Atomic
}

func NewRef(fd int) *Ref {
	ref := &Ref{
		File:     File{fd},
		refCount: 1,
	}
	runtime.SetFinalizer(ref, finalizeRef)
	return ref
}

func finalizeRef(ref *Ref) {
	if ref.refCount != 0 {
		log.Printf("unreachable file with reference count %d", ref.refCount)
		if ref.refCount > 0 {
			ref.File.Close()
		}
	}
}

// Ref increments reference count.
func (ref *Ref) Ref() *Ref {
	atomic.AddInt32(&ref.refCount, 1)
	return ref
}

// Close decrements reference count.  File is closed when reference count drops
// to zero.
func (ref *Ref) Close() (err error) {
	switch n := atomic.AddInt32(&ref.refCount, -1); {
	case n == 0:
		err = ref.File.Close()

	case n < 0:
		panic("file reference count is negative")
	}

	return
}
