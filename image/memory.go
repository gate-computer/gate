// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"context"
	"os"

	internal "github.com/tsavola/gate/internal/executable"
	"github.com/tsavola/gate/internal/manifest"
	"github.com/tsavola/wag/object"
	"golang.org/x/sys/unix"
)

const (
	memExecutableName = "gate-executable"
	memArchiveName    = "gate-archive"
)

// Memory implements BackingStore and Storage.
var Memory memory

type memory struct{}

func (memory) newExecutableFile() (f *os.File, err error) {
	return newMemFile(memExecutableName)
}

func (memory) newArchiveFile() (f *os.File, err error) {
	return newMemFile(memArchiveName)
}

func (memory) reflinkable(interface{}) bool {
	return false
}

func (memory) give(key string, man manifest.Archive, file *internal.FileRef, objectMap object.CallMap,
) (arc LocalArchive, err error) {
	arc = &memArchive{
		f:         file.Ref(),
		man:       man,
		objectMap: objectMap,
	}
	return
}

func (memory) Store(ctx context.Context, man manifest.Archive) (storer ArchiveStorer, err error) {
	f, err := newMemFile(memArchiveName)
	if err != nil {
		return
	}

	storer = ArchiveStorer{
		Module: f,
		Text:   f,
		Stack:  f,
		Data:   f,

		Archive: func(key string) (arc Archive, err error) {
			arc = &memArchive{
				f:   internal.NewFileRef(f),
				man: man,
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

type memArchive struct {
	f         *internal.FileRef
	man       manifest.Archive
	objectMap object.CallMap
}

func (arc *memArchive) file() *internal.FileRef {
	return arc.f
}

func (arc *memArchive) Manifest() manifest.Archive {
	return arc.man
}

func (arc *memArchive) ObjectMap() (object.CallMap, error) {
	return arc.objectMap, nil
}

func (*memArchive) reflinkable(interface{}) bool {
	return false
}

func (arc *memArchive) Load(context.Context) (loader ArchiveLoader, err error) {
	var (
		moduleOffset    = int(manifest.MaxSize)
		callSitesOffset = moduleOffset + int(arc.man.ModuleSize)
		funcAddrsOffset = callSitesOffset + int(arc.man.CallSitesSize)
		textOffset      = int64(alignSize(funcAddrsOffset + int(arc.man.FuncAddrsSize)))
		stackOffset     = textOffset + int64(arc.man.Exe.TextSize)
		dataOffset      = stackOffset + int64(arc.man.Exe.StackSize)
	)

	loader = ArchiveLoader{
		Module: ReaderOption{RandomAccess: arc.f, Offset: int64(moduleOffset)},
		Text:   ReaderOption{RandomAccess: arc.f, Offset: textOffset},
		Stack:  ReaderOption{RandomAccess: arc.f, Offset: stackOffset},
		Data:   ReaderOption{RandomAccess: arc.f, Offset: dataOffset},
		Close:  func() error { return nil },
	}
	return
}

func (arc *memArchive) Close() error {
	return arc.f.Close()
}

func newMemFile(name string) (f *os.File, err error) {
	fd, err := unix.MemfdCreate(name, unix.MFD_CLOEXEC)
	if err != nil {
		return
	}

	f = os.NewFile(uintptr(fd), name)
	return
}

func init() {
	var _ BackingStore = Memory
	var _ LocalStorage = Memory
	var _ Storage = Memory
}
