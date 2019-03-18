// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"os"

	"github.com/tsavola/gate/internal/file"
	"github.com/tsavola/gate/internal/manifest"
)

const (
	memProgramName  = "gate-program"
	memInstanceName = "gate-instance"
)

// Memory implements LocalStorage.  It doesn't support program persistence.
var Memory mem

type mem struct{}

func (mem) programBackend() interface{}  { return Memory }
func (mem) instanceBackend() interface{} { return Memory }
func (mem) singleBackend() bool          { return true }

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

func (mem) storeProgram(*Program, string) (_ error)           { return }
func (mem) loadProgram(Storage, string) (_ *Program, _ error) { return }
func (mem) LoadProgram(string) (_ *Program, _ error)          { return }

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

func (mem) storeInstance(*Instance, string) (_ manifest.Instance, _ error) { return }
func (mem) LoadInstance(string, manifest.Instance) (*Instance, error)      { return nil, os.ErrNotExist }
