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
	"github.com/tsavola/wag/object"
)

const (
	fsRootDir    = "v0"
	fsProgramDir = fsRootDir + "/program"
)

const (
	programFileTag     = 0x4a5274bd
	manifestHeaderSize = 8
)

// Filesystem implements LocalStorage.  It supports program persistence.
type Filesystem struct {
	rootDir string
	progDir string
}

func NewFilesystem(path string) (fs *Filesystem) {
	fs = &Filesystem{
		rootDir: pathlib.Join(path, fsRootDir),
		progDir: pathlib.Join(path, fsProgramDir),
	}

	os.Mkdir(fs.rootDir, 0700)
	os.Mkdir(fs.progDir, 0700)
	return
}

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

func (fs *Filesystem) newInstanceFile() (f *file.File, err error) {
	f, err = openTempFile(fs.rootDir, syscall.O_RDWR|syscall.O_EXCL, 0)
	if err != nil {
		return
	}

	err = ftruncate(f.Fd(), instMaxOffset)
	if err != nil {
		return
	}

	return
}

func (fs *Filesystem) newProgram(man manifest.Archive, f *file.File, codeMap object.CallMap) *Program {
	return &Program{
		Map:  codeMap,
		man:  man,
		file: f,
		dir:  fs.progDir,
	}
}

func (fs *Filesystem) LoadProgram(name string) (prog *Program, err error) {
	f, err := open(pathlib.Join(fs.progDir, name), syscall.O_RDONLY)
	if err != nil {
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

	var man manifest.Archive

	err = man.Unmarshal(b[manifestHeaderSize : manifestHeaderSize+int(manSize)])
	if err != nil {
		return
	}

	// TODO: load object map

	prog = &Program{
		man:  man,
		file: f,
	}
	return
}

func storeProgram(prog *Program, name string) (err error) {
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

	err = linkTempFile(prog.file.Fd(), pathlib.Join(prog.dir, name))
	if err != nil {
		return
	}

	return
}
