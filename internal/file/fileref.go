// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package file

import (
	"log"
	"os"
	"runtime"
	"sync/atomic"
)

type OpaqueRef interface {
	Close() error
	ref() // Can only be implemented in this package.
}

type Ref struct {
	*os.File

	refCount int32 // Atomic
}

func NewRef(f *os.File) *Ref {
	ref := &Ref{
		File:     f,
		refCount: 1,
	}
	runtime.SetFinalizer(ref, finalizeRef)
	return ref
}

func finalizeRef(ref *Ref) {
	if ref.refCount != 0 {
		log.Printf("unreachable file with reference count %d", ref.refCount)
		if ref.refCount > 0 {
			ref.cleanup()
		}
	}
}

func (ref *Ref) cleanup() (err error) {
	err = ref.File.Close()
	ref.File = nil
	return
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
		err = ref.cleanup()

	case n < 0:
		panic("file reference count is negative")
	}

	return
}

func (*Ref) ref() {}
