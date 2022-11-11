// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"bytes"
	"io"

	"gate.computer/gate/snapshot"
	"gate.computer/gate/snapshot/wasm"
	"gate.computer/internal/error/notfound"
	internal "gate.computer/internal/executable"
	"gate.computer/internal/file"
	"gate.computer/internal/manifest"
	"gate.computer/wag/object"
)

type Program struct {
	Map object.CallMap

	storage Storage
	man     *manifest.Program
	file    *file.File
}

func (prog *Program) PageSize() int     { return internal.PageSize }
func (prog *Program) TextSize() int     { return alignPageSize32(prog.man.TextSize) }
func (prog *Program) ModuleSize() int64 { return prog.man.ModuleSize }
func (prog *Program) Random() bool      { return prog.man.Random }

// Breakpoints are in ascending order and unique.
func (prog *Program) Breakpoints() []uint64 { return prog.man.Snapshot.Breakpoints }

// ResolveEntryFunc index or the implicit _start function index.  The started
// argument is disregarded if the program is a snapshot.
func (prog *Program) ResolveEntryFunc(exportName string, started bool) (int, error) {
	// internal/build.ResolveEntryFunc must be kept in sync with this.

	var (
		startIndex uint32
		startFound bool
	)
	if prog.man.SnapshotSection.Size == 0 && !started {
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
	return prog.file, nil
}

func (prog *Program) NewModuleReader() io.Reader {
	return io.NewSectionReader(prog.file, progModuleOffset, prog.man.ModuleSize)
}

func (prog *Program) LoadBuffers() (bs snapshot.Buffers, err error) {
	if prog.man.BufferSection.Size == 0 {
		return
	}

	header := make([]byte, prog.man.BufferSectionHeaderSize)

	off := progModuleOffset + prog.man.BufferSection.Start
	_, err = prog.file.ReadAt(header, off)
	if err != nil {
		return
	}

	bs, n, dataBuf, err := wasm.ReadBufferSectionHeader(bytes.NewReader(header), prog.man.BufferSection.Size)
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

func (prog *Program) Close() error {
	err := prog.file.Close()
	prog.file = nil
	return err
}

type ProgramStorage interface {
	Programs() (names []string, err error)

	newProgramFile() (*file.File, error)
	protectProgramFile(*file.File) error
	storeProgram(prog *Program, name string) error
	loadProgram(combined Storage, name string) (*Program, error)
}
