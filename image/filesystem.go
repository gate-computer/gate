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

	"gate.computer/gate/internal/file"
	"gate.computer/gate/internal/manifest"
	"gate.computer/gate/runtime/abi"
	"gate.computer/wag/object"
	"golang.org/x/sys/unix"
	"google.golang.org/protobuf/proto"
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
		if e := os.Mkdir(p, 0o700); e != nil && !os.IsExist(e) {
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
	f, err = openat(int(fs.progDir.Fd()), ".", unix.O_TMPFILE|syscall.O_RDWR, 0o400)
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			f.Close()
		}
	}()

	err = ftruncate(f.FD(), progMaxOffset)
	return
}

func (fs *Filesystem) protectProgramFile(*file.File) error { return nil }

func (fs *Filesystem) storeProgram(prog *Program, name string) (err error) {
	err = marshalManifest(prog.file, prog.man, progManifestOffset, programFileTag)
	if err != nil {
		return
	}

	err = fdatasync(prog.file.FD())
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

	err = fdatasync(fs.progDir.FD())
	return
}

func (fs *Filesystem) Programs() (names []string, err error) {
	return fs.listNames(fs.progDir.Fd())
}

func (fs *Filesystem) LoadProgram(name string) (prog *Program, err error) {
	return fs.loadProgram(fs, name)
}

func (fs *Filesystem) loadProgram(storage Storage, name string) (*Program, error) {
	f, err := openat(int(fs.progDir.Fd()), name, syscall.O_RDONLY, 0)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() {
		if f != nil {
			f.Close()
		}
	}()

	prog := &Program{
		storage: storage,
		man:     new(manifest.Program),
	}

	if err := unmarshalManifest(f, prog.man, progManifestOffset, programFileTag); err != nil {
		return nil, err
	}

	if prog.man.LibraryChecksum != abi.LibraryChecksum() {
		return nil, nil
	}

	var (
		progCallSitesOffset = progModuleOffset + align8(int64(prog.man.ModuleSize))
		progFuncAddrsOffset = progCallSitesOffset + int64(prog.man.CallSitesSize)
	)

	prog.Map.CallSites = make([]object.CallSite, prog.man.CallSitesSize/callSiteSize)
	prog.Map.FuncAddrs = make([]uint32, prog.man.FuncAddrsSize/4)

	// TODO: preadv

	if _, err := io.ReadFull(io.NewSectionReader(f, progCallSitesOffset, int64(prog.man.CallSitesSize)), callSitesBytes(&prog.Map)); err != nil {
		return nil, err
	}

	if _, err := io.ReadFull(io.NewSectionReader(f, progFuncAddrsOffset, int64(prog.man.FuncAddrsSize)), funcAddrsBytes(&prog.Map)); err != nil {
		return nil, err
	}

	prog.file = f
	f = nil
	return prog, nil
}

func (fs *Filesystem) newInstanceFile() (f *file.File, err error) {
	f, err = openat(int(fs.instDir.Fd()), ".", unix.O_TMPFILE|syscall.O_RDWR, 0o600)
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			f.Close()
		}
	}()

	err = ftruncate(f.FD(), instMaxOffset)
	return
}

func (fs *Filesystem) instanceFileWriteSupported() bool { return true }
func (fs *Filesystem) storeInstanceSupported() bool     { return true }

func (fs *Filesystem) storeInstance(inst *Instance, name string) (err error) {
	if inst.manDirty {
		err = marshalManifest(inst.file, inst.man, instManifestOffset, instanceFileTag)
		if err != nil {
			return
		}
		inst.manDirty = false
	}

	err = fdatasync(inst.file.FD())
	if err != nil {
		return
	}

	err = linkTempFile(inst.file.Fd(), fs.instDir.Fd(), name)
	if err != nil {
		return
	}

	err = fdatasync(fs.instDir.FD())
	if err != nil {
		return
	}

	inst.dir = fs.instDir
	inst.name = name
	return
}

func (fs *Filesystem) Instances() (names []string, err error) {
	return fs.listNames(fs.instDir.Fd())
}

func (fs *Filesystem) LoadInstance(name string) (inst *Instance, err error) {
	f, err := openat(int(fs.instDir.Fd()), name, syscall.O_RDWR, 0)
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

	inst = &Instance{
		man:      new(manifest.Instance),
		coherent: true,
		file:     f,
		dir:      fs.instDir,
		name:     name,
	}

	err = unmarshalManifest(f, inst.man, instManifestOffset, instanceFileTag)
	return
}

func (fs *Filesystem) listNames(dirFD uintptr) (names []string, err error) {
	dir, err := os.Open(fmt.Sprintf("/proc/self/fd/%d", dirFD))
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

func marshalManifest(f *file.File, man proto.Message, offset int64, tag uint32) (err error) {
	marsh, err := proto.Marshal(man)
	if err != nil {
		return
	}

	size := manifestHeaderSize + len(marsh)
	b := make([]byte, size)
	binary.LittleEndian.PutUint32(b, tag)
	binary.LittleEndian.PutUint32(b[4:], uint32(size))
	copy(b[manifestHeaderSize:], marsh)

	_, err = f.WriteAt(b, offset)
	return
}

func unmarshalManifest(f *file.File, man proto.Message, offset int64, tag uint32) (err error) {
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

	return proto.Unmarshal(b[manifestHeaderSize:size], man)
}
