// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"encoding/binary"
	"fmt"
	"os"
	"path"
	"syscall"

	"github.com/tsavola/gate/image/internal/manifest"
	"github.com/tsavola/gate/internal/file"
	"golang.org/x/sys/unix"
)

const (
	programFileTag     = 0x4a5274bd
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
func (fs *Filesystem) singleBackend() bool          { return true }

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
	if err != nil {
		return
	}

	return
}

func (fs *Filesystem) protectProgramFile(*file.File) error { return nil }

func (fs *Filesystem) storeProgram(prog *Program, name string) (err error) {
	err = func() (err error) {
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
		return
	}()
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
	if err != nil {
		return
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
	if err != nil {
		return
	}

	return
}

func (fs *Filesystem) instanceFileWriteSupported() bool { return true }
func (fs *Filesystem) storeInstanceSupported() bool     { return true }

func (fs *Filesystem) storeInstance(inst *Instance, name string) (man manifest.Instance, err error) {
	if inst.name != "" {
		// Instance not mutated after it was loaded; link is still there.
		return
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
	man = inst.man
	return
}

func (fs *Filesystem) LoadInstance(name string, man manifest.Instance) (inst *Instance, err error) {
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
		man:      man,
		file:     f,
		coherent: true,
		dir:      fs.instDir,
		name:     name,
	}
	return
}
