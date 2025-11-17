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

	"gate.computer/gate/runtime/abi"
	"gate.computer/internal/file"
	pb "gate.computer/internal/pb/image"
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

func NewFilesystem(root string) (*Filesystem, error) {
	return NewFilesystemWithOwnership(root, -1, -1)
}

func NewFilesystemWithOwnership(root string, uid, gid int) (*Filesystem, error) {
	progPath := path.Join(root, "program")
	instPath := path.Join(root, "instance")

	// Don't use MkdirAll to get an error if root doesn't exist.
	for _, p := range []string{progPath, instPath} {
		if err := os.Mkdir(p, 0o700); err != nil && !os.IsExist(err) {
			return nil, err
		}
	}

	var ok bool

	progDir, err := openat(unix.AT_FDCWD, progPath, syscall.O_DIRECTORY, 0)
	if err != nil {
		return nil, err
	}
	defer func() {
		if !ok {
			progDir.Close()
		}
	}()

	instDir, err := openat(unix.AT_FDCWD, instPath, syscall.O_DIRECTORY, 0)
	if err != nil {
		return nil, err
	}
	defer func() {
		if !ok {
			instDir.Close()
		}
	}()

	if uid >= 0 || gid >= 0 {
		if err := unix.Fchownat(int(progDir.Fd()), ".", uid, gid, 0); err != nil {
			return nil, err
		}
		if err := unix.Fchownat(int(instDir.Fd()), ".", uid, gid, 0); err != nil {
			return nil, err
		}
	}

	ok = true
	return &Filesystem{progDir, instDir}, nil
}

func (fs *Filesystem) Close() error {
	fs.instDir.Close()
	fs.progDir.Close()
	return nil
}

func (fs *Filesystem) newProgramFile() (*file.File, error) {
	var ok bool

	f, err := openat(int(fs.progDir.Fd()), ".", unix.O_TMPFILE|syscall.O_RDWR, 0o400)
	if err != nil {
		return nil, err
	}
	defer func() {
		if !ok {
			f.Close()
		}
	}()

	if err := ftruncate(f.FD(), progMaxOffset); err != nil {
		return nil, err
	}

	ok = true
	return f, nil
}

func (fs *Filesystem) protectProgramFile(*file.File) error { return nil }

func (fs *Filesystem) storeProgram(prog *Program, name string) error {
	if err := marshalManifest(prog.file, prog.man, progManifestOffset, programFileTag); err != nil {
		return err
	}
	if err := fdatasync(prog.file.FD()); err != nil {
		return err
	}
	if err := linkTempFile(prog.file.Fd(), fs.progDir.Fd(), name); err != nil && !os.IsExist(err) {
		return err
	}
	return fdatasync(fs.progDir.FD())
}

func (fs *Filesystem) Programs() ([]string, error) {
	return fs.listNames(fs.progDir.Fd())
}

func (fs *Filesystem) LoadProgram(name string) (*Program, error) {
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
		man:     new(pb.ProgramManifest),
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

func (fs *Filesystem) newInstanceFile() (*file.File, error) {
	var ok bool

	f, err := openat(int(fs.instDir.Fd()), ".", unix.O_TMPFILE|syscall.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	defer func() {
		if !ok {
			f.Close()
		}
	}()

	if err := ftruncate(f.FD(), instMaxOffset); err != nil {
		return nil, err
	}

	ok = true
	return f, nil
}

func (fs *Filesystem) instanceFileWriteSupported() bool { return true }
func (fs *Filesystem) storeInstanceSupported() bool     { return true }

func (fs *Filesystem) storeInstance(inst *Instance, name string) error {
	if inst.manDirty {
		if err := marshalManifest(inst.file, inst.man, instManifestOffset, instanceFileTag); err != nil {
			return err
		}
		inst.manDirty = false
	}

	if err := fdatasync(inst.file.FD()); err != nil {
		return err
	}
	if err := linkTempFile(inst.file.Fd(), fs.instDir.Fd(), name); err != nil {
		return err
	}
	if err := fdatasync(fs.instDir.FD()); err != nil {
		return err
	}

	inst.dir = fs.instDir
	inst.name = name
	return nil
}

func (fs *Filesystem) Instances() ([]string, error) {
	return fs.listNames(fs.instDir.Fd())
}

func (fs *Filesystem) LoadInstance(name string) (*Instance, error) {
	var ok bool

	f, err := openat(int(fs.instDir.Fd()), name, syscall.O_RDWR, 0)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() {
		if !ok {
			f.Close()
		}
	}()

	inst := &Instance{
		man:      new(pb.InstanceManifest),
		coherent: true,
		file:     f,
		dir:      fs.instDir,
		name:     name,
	}

	if err := unmarshalManifest(f, inst.man, instManifestOffset, instanceFileTag); err != nil {
		return nil, err
	}

	ok = true
	return inst, nil
}

func (fs *Filesystem) listNames(dirFD uintptr) ([]string, error) {
	dir, err := os.Open(fmt.Sprintf("/proc/self/fd/%d", dirFD))
	if err != nil {
		return nil, err
	}
	defer dir.Close()

	infos, err := dir.Readdir(-1)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(infos))
	for _, info := range infos {
		if info.Mode().IsRegular() {
			names = append(names, info.Name())
		}
	}
	return names, nil
}

func marshalManifest(f *file.File, man proto.Message, offset int64, tag uint32) error {
	marsh, err := proto.Marshal(man)
	if err != nil {
		return err
	}

	size := manifestHeaderSize + len(marsh)
	b := make([]byte, size)
	binary.LittleEndian.PutUint32(b, tag)
	binary.LittleEndian.PutUint32(b[4:], uint32(size))
	copy(b[manifestHeaderSize:], marsh)

	if _, err := f.WriteAt(b, offset); err != nil {
		return err
	}
	return nil
}

func unmarshalManifest(f *file.File, man proto.Message, offset int64, tag uint32) error {
	b := make([]byte, maxManifestSize)
	if _, err := io.ReadFull(io.NewSectionReader(f, offset, maxManifestSize), b); err != nil {
		return err
	}

	if x := binary.LittleEndian.Uint32(b); x != tag {
		return fmt.Errorf("incorrect file tag: %#x", x)
	}

	size := binary.LittleEndian.Uint32(b[4:])
	if size < manifestHeaderSize || size > maxManifestSize {
		return fmt.Errorf("manifest size out of bounds: %d", size)
	}

	return proto.Unmarshal(b[manifestHeaderSize:size], man)
}
