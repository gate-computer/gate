// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path"
	"syscall"

	"github.com/tsavola/gate/internal/file"
	"github.com/tsavola/gate/internal/manifest"
	"github.com/tsavola/wag/object"
	"golang.org/x/sys/unix"
)

const (
	programFileTag     = 0x4a5274bd
	instanceFileTag    = 0xb405dd05
	manifestHeaderSize = 8
)

// Filesystem implements Storage.  It supports program and instance
// persistence.
type Filesystem struct {
	progDir *file.File
	instDir *file.File
}

func NewFilesystem(root string) (fs *Filesystem, err error) {
	progPath := path.Join(root, "program")
	instPath := path.Join(root, "instance")

	// Don't use MkdirAll to get an error if root doesn't exist.
	for _, p := range []string{progPath, instPath} {
		if e := os.Mkdir(p, 0700); e != nil && !os.IsExist(e) {
			err = e
			return
		}
	}

	progDir, err := openat(unix.AT_FDCWD, progPath, syscall.O_DIRECTORY, 0)
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			progDir.Close()
		}
	}()

	instDir, err := openat(unix.AT_FDCWD, instPath, syscall.O_DIRECTORY, 0)
	if err != nil {
		return
	}

	fs = &Filesystem{progDir, instDir}
	return
}

func (fs *Filesystem) Close() (err error) {
	fs.instDir.Close()
	fs.progDir.Close()
	return
}

func (fs *Filesystem) programBackend() interface{}  { return fs }
func (fs *Filesystem) instanceBackend() interface{} { return fs }

func (fs *Filesystem) newProgramFile() (f *file.File, err error) {
	f, err = openat(int(fs.progDir.Fd()), ".", unix.O_TMPFILE|syscall.O_RDWR, 0400)
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			f.Close()
		}
	}()

	err = ftruncate(f.Fd(), progMaxOffset)
	return
}

func (fs *Filesystem) protectProgramFile(*file.File) error { return nil }

func (fs *Filesystem) storeProgram(prog *Program, name string) (err error) {
	err = marshalManifest(prog.file, &prog.man, progManifestOffset, programFileTag)
	if err != nil {
		return
	}

	err = fdatasync(prog.file.Fd())
	if err != nil {
		return
	}

	err = linkTempFile(prog.file.Fd(), fs.progDir.Fd(), name)
	if err != nil {
		if !os.IsExist(err) {
			return
		}
		err = nil
	}

	err = fdatasync(fs.progDir.Fd())
	return
}

func (fs *Filesystem) Programs() (names []string, err error) {
	dir, err := openAsOSFile(fs.progDir.Fd())
	if err != nil {
		return
	}
	defer dir.Close()

	infos, err := dir.Readdir(-1)
	if err != nil {
		return
	}

	for _, info := range infos {
		if info.Mode().IsRegular() {
			names = append(names, info.Name())
		}
	}
	return
}

func (fs *Filesystem) LoadProgram(name string) (prog *Program, err error) {
	return fs.loadProgram(fs, name)
}

func (fs *Filesystem) loadProgram(storage Storage, name string) (prog *Program, err error) {
	f, err := openat(int(fs.progDir.Fd()), name, syscall.O_RDONLY, 0)
	if err != nil {
		if os.IsNotExist(err) {
			err = nil
		}
		return
	}
	defer func() {
		if err != nil {
			f.Close()
		}
	}()

	prog = &Program{
		storage: storage,
		file:    f,
	}

	err = unmarshalManifest(f, &prog.man, progManifestOffset, programFileTag)
	if err != nil {
		return
	}

	var (
		progCallSitesOffset = progModuleOffset + align8(int64(prog.man.ModuleSize))
		progFuncAddrsOffset = progCallSitesOffset + int64(prog.man.CallSitesSize)
	)

	prog.Map.CallSites = make([]object.CallSite, prog.man.CallSitesSize/callSiteSize)
	prog.Map.FuncAddrs = make([]uint32, prog.man.FuncAddrsSize/4)

	// TODO: preadv

	_, err = io.ReadFull(io.NewSectionReader(f, progCallSitesOffset, int64(prog.man.CallSitesSize)), callSitesBytes(&prog.Map))
	if err != nil {
		return
	}

	_, err = io.ReadFull(io.NewSectionReader(f, progFuncAddrsOffset, int64(prog.man.FuncAddrsSize)), funcAddrsBytes(&prog.Map))
	return
}

func (fs *Filesystem) newInstanceFile() (f *file.File, err error) {
	f, err = openat(int(fs.instDir.Fd()), ".", unix.O_TMPFILE|syscall.O_RDWR, 0600)
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			f.Close()
		}
	}()

	err = ftruncate(f.Fd(), instMaxOffset)
	return
}

func (fs *Filesystem) instanceFileWriteSupported() bool { return true }
func (fs *Filesystem) storeInstanceSupported() bool     { return true }

func (fs *Filesystem) storeInstance(inst *Instance, name string) (err error) {
	if inst.manDirty {
		err = marshalManifest(inst.file, &inst.man, instManifestOffset, instanceFileTag)
		if err != nil {
			return
		}
		inst.manDirty = false
	}

	err = fdatasync(inst.file.Fd())
	if err != nil {
		return
	}

	err = linkTempFile(inst.file.Fd(), fs.instDir.Fd(), name)
	if err != nil {
		return
	}

	err = fdatasync(fs.instDir.Fd())
	if err != nil {
		return
	}

	inst.dir = fs.instDir
	inst.name = name
	return
}

func (fs *Filesystem) LoadInstance(name string) (inst *Instance, err error) {
	f, err := openat(int(fs.instDir.Fd()), name, syscall.O_RDWR, 0)
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			f.Close()
		}
	}()

	inst = &Instance{
		coherent: true,
		file:     f,
		dir:      fs.instDir,
		name:     name,
	}

	err = unmarshalManifest(f, &inst.man, instManifestOffset, instanceFileTag)
	return
}

type marshaler interface {
	Size() int
	MarshalTo([]byte) (int, error)
}

func marshalManifest(f *file.File, man marshaler, offset int64, tag uint32) (err error) {
	size := manifestHeaderSize + man.Size()
	b := make([]byte, size)
	binary.LittleEndian.PutUint32(b, tag)
	binary.LittleEndian.PutUint32(b[4:], uint32(size))

	_, err = man.MarshalTo(b[manifestHeaderSize:])
	if err != nil {
		return
	}

	_, err = f.WriteAt(b, offset)
	return
}

type unmarshaler interface {
	Unmarshal([]byte) error
}

func unmarshalManifest(f *file.File, man unmarshaler, offset int64, tag uint32) (err error) {
	b := make([]byte, manifest.MaxSize)
	_, err = io.ReadFull(io.NewSectionReader(f, offset, manifest.MaxSize), b)
	if err != nil {
		return
	}

	if x := binary.LittleEndian.Uint32(b); x != tag {
		err = fmt.Errorf("incorrect file tag: %#x", x)
		return
	}

	size := binary.LittleEndian.Uint32(b[4:])
	if size < manifestHeaderSize || size > manifest.MaxSize {
		err = fmt.Errorf("manifest size out of bounds: %d", size)
		return
	}

	return man.Unmarshal(b[manifestHeaderSize:size])
}
