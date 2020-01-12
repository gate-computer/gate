// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package facile

import (
	"bytes"

	"github.com/tsavola/gate/build"
	"github.com/tsavola/gate/image"
	"github.com/tsavola/gate/runtime/abi"
	"github.com/tsavola/gate/snapshot"
	"github.com/tsavola/wag/compile"
	"github.com/tsavola/wag/object"
	"github.com/tsavola/wag/wa"
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

func NewProgramImage(programStorage *Filesystem, wasm []byte) (prog *ProgramImage, err error) {
	storage := image.CombinedStorage(programStorage.fs, image.Memory)

	var objectMap object.CallMap

	b, err := build.New(storage, len(wasm), compile.MaxTextSize, &objectMap, false)
	if err != nil {
		return
	}
	defer b.Close()

	reader := bytes.NewReader(wasm)

	b.InstallEarlySnapshotLoaders()

	b.Module, err = compile.LoadInitialSections(b.ModuleConfig(), reader)
	if err != nil {
		return
	}

	b.StackSize = wa.PageSize
	b.SetMaxMemorySize(compile.MaxMemorySize)

	err = b.BindFunctions("")
	if err != nil {
		return
	}

	err = compile.LoadCodeSection(b.CodeConfig(&objectMap), reader, b.Module, abi.Library())
	if err != nil {
		return
	}

	b.InstallSnapshotDataLoaders()

	err = compile.LoadCustomSections(&b.Config, reader)
	if err != nil {
		return
	}

	err = b.FinishImageText()
	if err != nil {
		return
	}

	b.InstallLateSnapshotLoaders()

	err = compile.LoadDataSection(b.DataConfig(), reader, b.Module)
	if err != nil {
		return
	}

	err = compile.LoadCustomSections(&b.Config, reader)
	if err != nil {
		return
	}

	progImage, err := b.FinishProgramImage()
	if err != nil {
		return
	}

	prog = &ProgramImage{progImage, b.Module.MemorySizeLimit(), b.Buffers, b.Module.FuncTypes(), objectMap}
	return
}

func (prog *ProgramImage) Close() error {
	return prog.image.Close()
}

type InstanceImage struct {
	image   *image.Instance
	buffers snapshot.Buffers
}

func NewInstanceImage(prog *ProgramImage, entryFunction string) (inst *InstanceImage, err error) {
	stackSize := wa.PageSize

	entryFunc, err := prog.image.ResolveEntryFunc(entryFunction, false)
	if err != nil {
		return
	}

	instImage, err := image.NewInstance(prog.image, prog.memorySizeLimit, stackSize, entryFunc)
	if err != nil {
		return
	}

	inst = &InstanceImage{instImage, prog.buffers}
	return
}

func (inst *InstanceImage) Close() error {
	return inst.image.Close()
}
