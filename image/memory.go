// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"io"
	"os"
	"syscall"

	"gate.computer/gate/internal/file"
	"gate.computer/gate/internal/manifest"
)

const (
	memProgramName  = "gate-program"
	memInstanceName = "gate-instance"
)

// Memory implements Storage.  It doesn't support program or instance
// persistence.
var Memory mem

type mem struct{}

func (mem) newProgramFile() (*file.File, error) {
	return newMemoryFile(memProgramName, progMaxOffset)
}

func (mem) protectProgramFile(f *file.File) error {
	return protectFileMemory(f, syscall.PROT_READ|syscall.PROT_EXEC)
}

func (mem) storeProgram(*Program, string) error               { return nil }
func (mem) Programs() (_ []string, _ error)                   { return }
func (mem) LoadProgram(string) (_ *Program, _ error)          { return }
func (mem) loadProgram(Storage, string) (_ *Program, _ error) { return }

func (mem) newInstanceFile() (f *file.File, err error) {
	f, err = newMemoryFile(memInstanceName, instMaxOffset)
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			f.Close()
		}
	}()

	err = protectFileMemory(f, syscall.PROT_READ|syscall.PROT_WRITE)
	return
}

func (mem) instanceFileWriteSupported() bool       { return memoryFileWriteSupported }
func (mem) storeInstanceSupported() bool           { return false }
func (mem) storeInstance(*Instance, string) error  { return nil }
func (mem) Instances() (_ []string, _ error)       { return }
func (mem) LoadInstance(string) (*Instance, error) { return nil, os.ErrNotExist }

type persistMem struct {
	fs *Filesystem
}

// PersistentMemory supports instance persistence by copying data to and from a
// Filesystem.
func PersistentMemory(storage *Filesystem) InstanceStorage   { return persistMem{storage} }
func (pmem persistMem) newInstanceFile() (*file.File, error) { return Memory.newInstanceFile() }
func (pmem persistMem) instanceFileWriteSupported() bool     { return Memory.instanceFileWriteSupported() }
func (pmem persistMem) storeInstanceSupported() bool         { return true }

func (pmem persistMem) storeInstance(inst *Instance, name string) (err error) {
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

	// TODO: cache serialized form
	err = marshalManifest(f, inst.man, instManifestOffset, instanceFileTag)
	if err != nil {
		return
	}
	inst.manDirty = false

	err = fdatasync(f.FD())
	if err != nil {
		return
	}

	err = linkTempFile(f.Fd(), pmem.fs.instDir.Fd(), name)
	if err != nil {
		return
	}

	err = fdatasync(pmem.fs.instDir.FD())
	if err != nil {
		return
	}

	inst.dir = pmem.fs.instDir
	inst.name = name
	return
}

func (pmem persistMem) Instances() (names []string, err error) {
	return pmem.fs.Instances()
}

func (pmem persistMem) LoadInstance(name string) (inst *Instance, err error) {
	inst, err = pmem.fs.LoadInstance(name)
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

	err = copyInstance(f, inst.file, inst.man)
	if err != nil {
		return
	}

	fsFile := inst.file
	inst.file = f
	fsFile.Close()
	return
}

func copyInstance(dest, src *file.File, man *manifest.Instance) (err error) {
	o := int64(man.StackSize - man.StackUsage)
	l := int64(man.StackUsage)

	_, err = io.Copy(&offsetWriter{dest, o}, io.NewSectionReader(src, o, l))
	if err != nil {
		return
	}

	o = int64(man.StackSize) + alignPageOffset32(man.GlobalsSize) - int64(man.GlobalsSize)
	l = int64(man.GlobalsSize + man.MemorySize)

	_, err = io.Copy(&offsetWriter{dest, o}, io.NewSectionReader(src, o, l))
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
