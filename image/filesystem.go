// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"encoding/binary"
	"fmt"
	"os"
	pathlib "path"
	"syscall"

	"github.com/tsavola/gate/internal/file"
	"github.com/tsavola/gate/internal/manifest"
)

const (
	fsRootDir     = "v0"
	fsProgramDir  = fsRootDir + "/program"
	fsInstanceDir = fsRootDir + "/instance"
)

const (
	programFileTag     = 0x4a5274bd
	manifestHeaderSize = 8
)

// Filesystem implements LocalStorage.  It supports program persistence.
type Filesystem struct {
	progDir string
	instDir string
}

func NewFilesystem(path string) (fs *Filesystem) {
	fs = &Filesystem{
		progDir: pathlib.Join(path, fsProgramDir),
		instDir: pathlib.Join(path, fsInstanceDir),
	}

	os.Mkdir(pathlib.Join(path, fsRootDir), 0700)
	os.Mkdir(fs.progDir, 0700)
	os.Mkdir(fs.instDir, 0700)
	return
}

func (fs *Filesystem) programBackend() interface{}  { return fs }
func (fs *Filesystem) instanceBackend() interface{} { return fs }
func (fs *Filesystem) singleBackend() bool          { return true }

func (fs *Filesystem) newProgramFile() (f *file.File, err error) {
	f, err = openTempFile(fs.progDir, syscall.O_RDWR, 0400)
	if err != nil {
		return
	}

	err = ftruncate(f.Fd(), progMaxOffset)
	if err != nil {
		return
	}

	return
}

func (fs *Filesystem) storeProgram(prog *Program, name string) (err error) {
	b, err := mmap(prog.file.Fd(), progManifestOffset, manifestHeaderSize+manifest.MaxSize, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		return
	}
	defer mustMunmap(b)

	n, err := prog.man.MarshalTo(b[manifestHeaderSize:])
	if err != nil {
		return
	}
	binary.LittleEndian.PutUint32(b[4:], uint32(n))
	binary.LittleEndian.PutUint32(b, programFileTag)

	err = linkTempFile(prog.file.Fd(), pathlib.Join(fs.progDir, name))
	if err != nil {
		if !os.IsExist(err) {
			return
		}
		err = nil
	}

	return
}

func (fs *Filesystem) LoadProgram(name string) (prog *Program, err error) {
	return fs.loadProgram(fs, name)
}

func (fs *Filesystem) loadProgram(storage Storage, name string) (prog *Program, err error) {
	f, err := open(pathlib.Join(fs.progDir, name), syscall.O_RDONLY)
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

	b, err := mmap(f.Fd(), progManifestOffset, manifestHeaderSize+manifest.MaxSize, syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		return
	}
	defer mustMunmap(b)

	if tag := binary.LittleEndian.Uint32(b); tag != programFileTag {
		err = fmt.Errorf("unknown program file tag: %#x", programFileTag)
		return
	}

	manSize := binary.LittleEndian.Uint32(b[4:])
	if manSize > manifest.MaxSize {
		err = fmt.Errorf("program manifest size out of bounds: %d", manSize)
		return
	}

	var man manifest.Program

	err = man.Unmarshal(b[manifestHeaderSize : manifestHeaderSize+int(manSize)])
	if err != nil {
		return
	}

	// TODO: load object map

	prog = &Program{
		storage: storage,
		man:     man,
		file:    f,
	}
	return
}

func (fs *Filesystem) newInstanceFile() (f *file.File, err error) {
	f, err = openTempFile(fs.instDir, syscall.O_RDWR, 0600)
	if err != nil {
		return
	}

	err = ftruncate(f.Fd(), instMaxOffset)
	if err != nil {
		return
	}

	return
}

func (fs *Filesystem) storeInstanceSupported() bool {
	return true
}

func (fs *Filesystem) storeInstance(inst *Instance, name string) (man manifest.Instance, err error) {
	if inst.path != "" {
		// Instance not mutated after it was loaded; link is still there.
		return
	}

	path := pathlib.Join(fs.instDir, name)

	err = linkTempFile(inst.file.Fd(), path)
	if err != nil {
		return
	}

	inst.path = path
	man = inst.man
	return
}

func (fs *Filesystem) LoadInstance(name string, man manifest.Instance) (inst *Instance, err error) {
	path := pathlib.Join(fs.instDir, name)

	f, err := open(path, syscall.O_RDWR)
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			f.Close()
		}
	}()

	inst = &Instance{
		man:      man,
		file:     f,
		coherent: true,
		path:     path,
	}
	return
}
