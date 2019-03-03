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

	man  manifest.Archive
	file *file.File
	dir  string // Pending storage location.
}

func (prog *Program) Manifest() manifest.Archive { return prog.man }
func (prog *Program) PageSize() int              { return internal.PageSize }
func (prog *Program) TextSize() int              { return alignPageSize32(prog.man.TextSize) }
func (prog *Program) ModuleSize() int64          { return prog.man.ModuleSize }

// Text file handle is valid until the next Program method call.
func (prog *Program) Text() (file interface{ Fd() uintptr }, err error) {
	file = prog.file
	return
}

func (prog *Program) NewModuleReader() io.Reader {
	return io.NewSectionReader(prog.file, progModuleOffset, prog.man.ModuleSize)
}

// Store the program if the associated ProgramStorage supports it.  The name
// must not contain path separators.
func (prog *Program) Store(name string) (err error) {
	if prog.dir == "" {
		return
	}

	err = storeProgram(prog, name)
	if err != nil {
		return
	}

	prog.dir = ""
	return
}

func (prog *Program) Close() (err error) {
	err = prog.file.Close()
	prog.file = nil
	return
}

type ProgramStorage interface {
	// LoadProgram which has been stored previously.
	LoadProgram(name string) (*Program, error)

	newProgramFile() (*file.File, error)

	// The file must have been created with newProgramFile of this
	// ProgramStorage.  The Program takes ownership of the file.
	newProgram(m manifest.Archive, f *file.File, objectMap object.CallMap) *Program
}
