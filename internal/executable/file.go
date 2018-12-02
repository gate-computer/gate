// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package executable

import (
	"io"
	"os"
	"sync/atomic"
)

type BackingStore interface{}

type Manifest struct {
	TextSize      int
	StackSize     int
	StackUnused   int
	GlobalsSize   int
	MemorySize    int
	MaxMemorySize int
}

type Ref interface {
	io.Closer
	ref() // Can only be implemented in this package.
}

type FileRef struct {
	*os.File

	Back     BackingStore
	refCount int32 // Atomic
}

func NewFileRef(f *os.File, back BackingStore) *FileRef {
	return &FileRef{
		File:     f,
		Back:     back,
		refCount: 1,
	}
}

// Ref increments reference count.
func (ref *FileRef) Ref() *FileRef {
	atomic.AddInt32(&ref.refCount, 1)
	return ref
}

// Close decrements reference count.  File is closed when reference count drops
// to zero.
func (ref *FileRef) Close() (err error) {
	if atomic.AddInt32(&ref.refCount, -1) == 0 {
		err = ref.File.Close()
		ref.File = nil
	}
	return
}

func (*FileRef) ref() {}
