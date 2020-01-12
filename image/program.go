// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"bytes"
	"io"

	"github.com/tsavola/gate/image/internal/manifest"
	"github.com/tsavola/gate/internal/error/notfound"
	internal "github.com/tsavola/gate/internal/executable"
	"github.com/tsavola/gate/internal/file"
	"github.com/tsavola/gate/snapshot"
	"github.com/tsavola/gate/snapshot/wasm"
	"github.com/tsavola/wag/object"
)

type Program struct {
	Map object.CallMap

	storage Storage
	man     manifest.Program
	file    *file.File
	mem     []byte
}

func (prog *Program) PageSize() int     { return internal.PageSize }
func (prog *Program) TextSize() int     { return alignPageSize32(prog.man.TextSize) }
func (prog *Program) ModuleSize() int64 { return prog.man.ModuleSize }
func (prog *Program) Random() bool      { return prog.man.Random }

// ResolveEntryFunc or the implicit _start function.  The started argument is
// disregarded if the program is a snapshot.
func (prog *Program) ResolveEntryFunc(exportName string, started bool) (index int, err error) {
	// internal/build.ResolveEntryFunc must be kept in sync with this.

	var (
		startIndex uint32
		startFound bool
	)
	if prog.man.SnapshotSection.Length == 0 && !started {
		startIndex, startFound = prog.man.EntryIndexes["_start"]
	}

	if exportName == "" {
		if startFound {
			return int(startIndex), nil
		} else {
			return -1, nil
		}
	}

	if startFound {
		return -1, notfound.ErrStart
	}

	if exportName == "_start" {
		return -1, nil
	}

	i, found := prog.man.EntryIndexes[exportName]
	if !found {
		return -1, notfound.ErrFunction
	}

	return int(i), nil
}

func (prog *Program) Text() (file interface{ Fd() uintptr }, err error) {
	file = prog.file
	return
}

func (prog *Program) NewModuleReader() io.Reader {
	return io.NewSectionReader(prog.file, progModuleOffset, prog.man.ModuleSize)
}

func (prog *Program) LoadBuffers() (bs snapshot.Buffers, err error) {
	if prog.man.BufferSection.Length == 0 {
		return
	}

	header := make([]byte, prog.man.BufferSectionHeaderLength)

	off := progModuleOffset + prog.man.BufferSection.Offset
	_, err = prog.file.ReadAt(header, off)
	if err != nil {
		return
	}

	bs, n, dataBuf, err := wasm.ReadBufferSectionHeader(bytes.NewReader(header), uint32(prog.man.BufferSection.Length))
	if err != nil {
		return
	}
	off += int64(n)

	n, err = prog.file.ReadAt(dataBuf, off)
	if err != nil {
		return
	}
	off += int64(n)

	return
}

// Store the program.  The name must not contain path separators.
func (prog *Program) Store(name string) error {
	return prog.storage.storeProgram(prog, name)
}

func (prog *Program) Close() (err error) {
	err = prog.file.Close()
	prog.file = nil

	munmapp(&prog.mem)
	return
}

type ProgramStorage interface {
	Programs() (names []string, err error)

	newProgramFile() (*file.File, error)
	protectProgramFile(*file.File) error
	storeProgram(prog *Program, name string) error
	loadProgram(combined Storage, name string) (*Program, error)
	programBackend() interface{}
}
