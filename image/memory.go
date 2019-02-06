// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"bytes"
	"context"
	"os"

	internal "github.com/tsavola/gate/internal/executable"
	"golang.org/x/sys/unix"
)

const (
	memExecutableName = "gate-executable"
	memArchiveName    = "gate-archive"
)

// Memory implements BackingStore and Storage.
var Memory memory

type memory struct{}

func (memory) getPageSize() int {
	return internal.PageSize
}

func (memory) newExecutableFile() (f *os.File, err error) {
	return newMemFile(memExecutableName)
}

func (memory) sealFile(f *os.File) (err error) {
	_, err = unix.FcntlInt(f.Fd(), unix.F_ADD_SEALS, unix.F_SEAL_SHRINK|unix.F_SEAL_GROW)
	return
}

func (memory) CreateModule(ctx context.Context, size int) (store ModuleStore, err error) {
	buf := bytes.NewBuffer(make([]byte, 0, size))

	store = ModuleStore{
		Writer: buf,

		Module: func(key string) (m Module, err error) {
			m = memModule{buf.Bytes()}
			return
		},

		Close: func() error { return nil },
	}

	return
}

func (memory) CreateArchive(ctx context.Context, manifest *ArchiveManifest) (store ExecutableStore, err error) {
	f, err := newMemFile(memArchiveName)
	if err != nil {
		return
	}

	store = ExecutableStore{
		Text:          f,
		GlobalsMemory: f,

		Archive: func(key string) (ar Archive, err error) {
			ar = &memArchive{
				file:     internal.NewFileRef(f, nil),
				manifest: *manifest,
			}

			f = nil // Archived.
			return
		},

		Close: func() (err error) {
			if f != nil {
				err = f.Close()
			}
			return
		},
	}

	return
}

func (memory) archive(key string, metadata Metadata, manifest *internal.Manifest, fileRef *internal.FileRef,
) (ar Archive, err error) {
	if manifest.StackSize != 0 {
		return
	}

	ar = &memArchive{
		file:     fileRef.Ref(),
		manifest: makeArchiveManifest(manifest, metadata),
	}
	return
}

type memModule struct {
	data []byte
}

func (m memModule) Open(context.Context) (load ModuleLoad, err error) {
	load = ModuleLoad{
		Length:       int64(len(m.data)),
		ReaderOption: ReaderOption{Stream: bytes.NewReader(m.data)},
		Close:        func() error { return nil },
	}
	return
}

func (memModule) Close() error {
	return nil
}

type memArchive struct {
	file     *internal.FileRef
	manifest ArchiveManifest
}

func (ar *memArchive) Manifest() *ArchiveManifest {
	return &ar.manifest
}

func (ar *memArchive) Open(context.Context) (load ExecutableLoad, err error) {
	load = ExecutableLoad{
		Text: ReaderOption{
			RandomAccess: ar.file.File,
			Offset:       0,
		},
		GlobalsMemory: ReaderOption{
			RandomAccess: ar.file.File,
			Offset:       int64(ar.manifest.TextSize),
		},
		Close: func() error { return nil },
	}
	return
}

func (ar *memArchive) Close() error {
	return ar.file.Close()
}

func newMemFile(name string) (f *os.File, err error) {
	fd, err := unix.MemfdCreate(name, unix.MFD_CLOEXEC|unix.MFD_ALLOW_SEALING)
	if err != nil {
		return
	}

	f = os.NewFile(uintptr(fd), name)
	return
}

func init() {
	var _ BackingStore = Memory
	var _ Storage = Memory
	var _ internalArchiveStorage = Memory
}
