// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"io"

	"github.com/tsavola/gate/image/manifest"
	internal "github.com/tsavola/gate/internal/executable"
	"github.com/tsavola/gate/internal/file"
	"github.com/tsavola/wag/object"
)

type Program struct {
	Map object.CallMap

	storage Storage
	man     manifest.Program
	file    *file.File
	mem     []byte
}

func (prog *Program) Manifest() manifest.Program { return prog.man }
func (prog *Program) PageSize() int              { return internal.PageSize }
func (prog *Program) TextSize() int              { return alignPageSize32(prog.man.TextSize) }
func (prog *Program) ModuleSize() int64          { return prog.man.ModuleSize }
func (prog *Program) RandomSeed() bool           { return prog.man.RandomSeed }

// Text file handle is valid until the next Program method call.
func (prog *Program) Text() (file interface{ Fd() uintptr }, err error) {
	file = prog.file
	return
}

func (prog *Program) NewModuleReader() io.Reader {
	return io.NewSectionReader(prog.file, progModuleOffset, prog.man.ModuleSize)
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
	newProgramFile() (*file.File, error)
	storeProgram(prog *Program, name string) error
	loadProgram(combined Storage, name string) (*Program, error)
	programBackend() interface{}
}
