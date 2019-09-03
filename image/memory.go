// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"io"
	"os"

	"github.com/tsavola/gate/internal/file"
	"github.com/tsavola/gate/internal/manifest"
)

const (
	memProgramName  = "gate-program"
	memInstanceName = "gate-instance"
)

// Memory implements Storage.  It doesn't support program or instance
// persistence.
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
	defer func() {
		if err != nil {
			f.Close()
		}
	}()

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
	defer func() {
		if err != nil {
			f.Close()
		}
	}()

	err = ftruncate(f.Fd(), instMaxOffset)
	if err != nil {
		return
	}

	return
}

func (mem) storeInstanceSupported() bool                                   { return false }
func (mem) storeInstance(*Instance, string) (_ manifest.Instance, _ error) { return }
func (mem) LoadInstance(string, manifest.Instance) (*Instance, error)      { return nil, os.ErrNotExist }

type persistMem struct {
	fs *Filesystem
}

// PersistentMemory supports instance persistence by copying data to and from a
// Filesystem.
func PersistentMemory(storage *Filesystem) InstanceStorage { return persistMem{storage} }
func (pmem persistMem) instanceBackend() interface{}       { return pmem }
func (persistMem) newInstanceFile() (*file.File, error)    { return Memory.newInstanceFile() }
func (persistMem) storeInstanceSupported() bool            { return true }

func (pmem persistMem) storeInstance(inst *Instance, name string) (man manifest.Instance, err error) {
	if inst.name != "" {
		// Instance not mutated after it was loaded; link is still there.
		return
	}

	f, err := pmem.fs.newInstanceFile()
	if err != nil {
		return
	}
	defer f.Close()

	o := int64(inst.man.StackSize - inst.man.StackUsage)
	l := int64(inst.man.StackUsage)

	_, err = io.Copy(&offsetWriter{f, o}, io.NewSectionReader(inst.file, o, l))
	if err != nil {
		return
	}

	o = int64(inst.man.StackSize) + alignPageOffset32(inst.man.GlobalsSize) - int64(inst.man.GlobalsSize)
	l = int64(inst.man.GlobalsSize + inst.man.MemorySize)

	_, err = io.Copy(&offsetWriter{f, o}, io.NewSectionReader(inst.file, o, l))
	if err != nil {
		return
	}

	// TODO: move this to filesystem.go

	err = fdatasync(f.Fd())
	if err != nil {
		return
	}

	err = linkTempFile(f.Fd(), pmem.fs.instDir.Fd(), name)
	if err != nil {
		return
	}

	err = fdatasync(pmem.fs.instDir.Fd())
	if err != nil {
		return
	}

	inst.dir = pmem.fs.instDir
	inst.name = name
	man = inst.man
	return
}

func (pmem persistMem) LoadInstance(name string, man manifest.Instance) (inst *Instance, err error) {
	inst, err = pmem.fs.LoadInstance(name, man)
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			inst.Close()
		}
	}()

	f, err := Memory.newInstanceFile()
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			f.Close()
		}
	}()

	err = pmem.copyInstance(f, inst.file, man)
	if err != nil {
		return
	}

	fsFile := inst.file
	inst.file = f
	fsFile.Close()
	return
}

func (persistMem) copyInstance(dest, src *file.File, man manifest.Instance) (err error) {
	o := int64(man.StackSize - man.StackUsage)
	l := int64(man.StackUsage)

	_, err = io.Copy(&offsetWriter{dest, o}, io.NewSectionReader(src, o, l))
	if err != nil {
		return
	}

	o = int64(man.StackSize) + alignPageOffset32(man.GlobalsSize) - int64(man.GlobalsSize)
	l = int64(man.GlobalsSize + man.MemorySize)

	_, err = io.Copy(&offsetWriter{dest, o}, io.NewSectionReader(src, o, l))
	if err != nil {
		return
	}

	return
}

type offsetWriter struct {
	writerAt *file.File
	offset   int64
}

func (ow *offsetWriter) Write(b []byte) (n int, err error) {
	n, err = ow.writerAt.WriteAt(b, ow.offset)
	ow.offset += int64(n)
	return
}
