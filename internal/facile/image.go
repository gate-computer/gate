// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package facile

import (
	"bytes"

	"gate.computer/gate/build"
	"gate.computer/gate/image"
	"gate.computer/gate/runtime/abi"
	"gate.computer/gate/snapshot"
	"gate.computer/wag/compile"
	"gate.computer/wag/object"
	"gate.computer/wag/wa"
)

type Filesystem struct {
	fs *image.Filesystem
}

func NewFilesystem(root string) (filesystem *Filesystem, err error) {
	fs, err := image.NewFilesystem(root)
	if err != nil {
		return
	}

	filesystem = &Filesystem{fs}
	return
}

func (filesystem *Filesystem) Close() error {
	return filesystem.fs.Close()
}

type ProgramImage struct {
	image           *image.Program
	memorySizeLimit int
	buffers         snapshot.Buffers
	funcTypes       []wa.FuncType
	objectMap       object.CallMap
}

func NewProgramImage(programStorage *Filesystem, wasm []byte) (*ProgramImage, error) {
	storage := image.CombinedStorage(programStorage.fs, image.Memory)

	var objectMap object.CallMap

	b, err := build.New(storage, len(wasm), compile.MaxTextSize, &objectMap, false)
	if err != nil {
		return nil, err
	}
	defer b.Close()

	r := compile.NewLoader(bytes.NewReader(wasm))

	b.InstallEarlySnapshotLoaders()

	b.Module, err = compile.LoadInitialSections(b.ModuleConfig(), r)
	if err != nil {
		return nil, err
	}

	b.StackSize = wa.PageSize
	b.SetMaxMemorySize(compile.MaxMemorySize)

	if err = b.BindFunctions(""); err != nil {
		return nil, err
	}

	if err := compile.LoadCodeSection(b.CodeConfig(&objectMap), r, b.Module, abi.Library()); err != nil {
		return nil, err
	}

	if err := b.VerifyBreakpoints(); err != nil {
		return nil, err
	}

	b.InstallSnapshotDataLoaders()

	if err := compile.LoadCustomSections(&b.Config, r); err != nil {
		return nil, err
	}

	if err := b.FinishImageText(); err != nil {
		return nil, err
	}

	b.InstallLateSnapshotLoaders()

	if err := compile.LoadDataSection(b.DataConfig(), r, b.Module); err != nil {
		return nil, err
	}

	if err := compile.LoadCustomSections(&b.Config, r); err != nil {
		return nil, err
	}

	progImage, err := b.FinishProgramImage()
	if err != nil {
		return nil, err
	}

	prog := &ProgramImage{progImage, b.Module.MemorySizeLimit(), b.Buffers, b.Module.FuncTypes(), objectMap}
	return prog, nil
}

func (prog *ProgramImage) Close() error {
	return prog.image.Close()
}

type InstanceImage struct {
	image   *image.Instance
	buffers snapshot.Buffers
}

func NewInstanceImage(prog *ProgramImage, entryFunction string) (*InstanceImage, error) {
	stackSize := wa.PageSize

	entryFunc, err := prog.image.ResolveEntryFunc(entryFunction, false)
	if err != nil {
		return nil, err
	}

	instImage, err := image.NewInstance(prog.image, prog.memorySizeLimit, stackSize, entryFunc)
	if err != nil {
		return nil, err
	}

	return &InstanceImage{instImage, prog.buffers}, nil
}

func (inst *InstanceImage) Close() error {
	return inst.image.Close()
}
