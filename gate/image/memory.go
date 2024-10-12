// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"io"
	"os"
	"syscall"

	"gate.computer/internal/file"
	pb "gate.computer/internal/pb/image"
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

func (mem) newInstanceFile() (*file.File, error) {
	var ok bool

	f, err := newMemoryFile(memInstanceName, instMaxOffset)
	if err != nil {
		return nil, err
	}
	defer func() {
		if !ok {
			f.Close()
		}
	}()

	if err := protectFileMemory(f, syscall.PROT_READ|syscall.PROT_WRITE); err != nil {
		return nil, err
	}

	ok = true
	return f, nil
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

func (pmem persistMem) storeInstance(inst *Instance, name string) error {
	f, err := pmem.fs.newInstanceFile()
	if err != nil {
		return err
	}
	defer f.Close()

	o := int64(inst.man.StackSize - inst.man.StackUsage)
	l := int64(inst.man.StackUsage)

	if _, err := io.Copy(&offsetWriter{f, o}, io.NewSectionReader(inst.file, o, l)); err != nil {
		return err
	}

	o = int64(inst.man.StackSize) + alignPageOffset32(inst.man.GlobalsSize) - int64(inst.man.GlobalsSize)
	l = int64(inst.man.GlobalsSize + inst.man.MemorySize)

	if _, err := io.Copy(&offsetWriter{f, o}, io.NewSectionReader(inst.file, o, l)); err != nil {
		return err
	}

	// TODO: cache serialized form
	if err := marshalManifest(f, inst.man, instManifestOffset, instanceFileTag); err != nil {
		return err
	}
	inst.manDirty = false

	if err := fdatasync(f.FD()); err != nil {
		return err
	}

	if err := linkTempFile(f.Fd(), pmem.fs.instDir.Fd(), name); err != nil {
		return err
	}

	if err := fdatasync(pmem.fs.instDir.FD()); err != nil {
		return err
	}

	inst.dir = pmem.fs.instDir
	inst.name = name
	return nil
}

func (pmem persistMem) Instances() ([]string, error) {
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

func copyInstance(dest, src *file.File, man *pb.InstanceManifest) error {
	o := int64(man.StackSize - man.StackUsage)
	l := int64(man.StackUsage)

	if _, err := io.Copy(&offsetWriter{dest, o}, io.NewSectionReader(src, o, l)); err != nil {
		return err
	}

	o = int64(man.StackSize) + alignPageOffset32(man.GlobalsSize) - int64(man.GlobalsSize)
	l = int64(man.GlobalsSize + man.MemorySize)

	if _, err := io.Copy(&offsetWriter{dest, o}, io.NewSectionReader(src, o, l)); err != nil {
		return err
	}

	return nil
}

type offsetWriter struct {
	writerAt *file.File
	offset   int64
}

func (ow *offsetWriter) Write(b []byte) (int, error) {
	n, err := ow.writerAt.WriteAt(b, ow.offset)
	ow.offset += int64(n)
	return n, err
}
