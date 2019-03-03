// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"github.com/tsavola/gate/internal/file"
	"github.com/tsavola/gate/internal/manifest"
	"github.com/tsavola/wag/object"
)

const (
	memProgramName  = "gate-program"
	memInstanceName = "gate-instance"
)

// Memory implements LocalStorage.  It doesn't support program persistence.
var Memory mem

type mem struct{}

func (mem) newProgramFile() (f *file.File, err error) {
	f, err = memfdCreate(memProgramName)
	if err != nil {
		return
	}

	err = ftruncate(f.Fd(), progMaxOffset)
	if err != nil {
		return
	}

	return
}

func (mem) newInstanceFile() (f *file.File, err error) {
	f, err = memfdCreate(memInstanceName)
	if err != nil {
		return
	}

	err = ftruncate(f.Fd(), instMaxOffset)
	if err != nil {
		return
	}

	return
}

func (mem) newProgram(man manifest.Archive, f *file.File, codeMap object.CallMap) *Program {
	return &Program{
		Map:  codeMap,
		man:  man,
		file: f,
	}
}

func (mem) LoadProgram(name string) (prog *Program, err error) {
	return
}
