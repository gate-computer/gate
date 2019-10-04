// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package facile

import (
	"bytes"
	"errors"

	"github.com/tsavola/gate/build"
	"github.com/tsavola/gate/entry"
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
	image   *image.Program
	buffers snapshot.Buffers
}

func NewProgramImage(programStorage *Filesystem, wasm []byte) (prog *ProgramImage, err error) {
	storage := image.CombinedStorage(programStorage.fs, image.Memory)

	var codeMap object.CallMap

	b, err := build.New(storage, len(wasm), compile.DefaultMaxTextSize, &codeMap, false)
	if err != nil {
		return
	}
	defer b.Close()

	reader := bytes.NewReader(wasm)

	b.InstallEarlySnapshotLoaders(errors.New)

	b.Module, err = compile.LoadInitialSections(b.ModuleConfig(), reader)
	if err != nil {
		return
	}

	b.StackSize = wa.PageSize
	b.MaxMemorySize = b.Module.MemorySizeLimit()

	err = b.BindFunctions("")
	if err != nil {
		return
	}

	err = compile.LoadCodeSection(b.CodeConfig(&codeMap), reader, b.Module, abi.Library())
	if err != nil {
		return
	}

	b.InstallSnapshotDataLoaders(errors.New)

	err = compile.LoadCustomSections(&b.Config, reader)
	if err != nil {
		return
	}

	err = b.FinishImageText()
	if err != nil {
		return
	}

	b.InstallLateSnapshotLoaders(errors.New)

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

	prog = &ProgramImage{progImage, b.Buffers}
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
	var entryIndex uint32
	var entryAddr uint32

	if entryFunction != "" {
		entryIndex, err = entry.MapFuncIndex(prog.image.Manifest().EntryIndexes, entryFunction)
		if err != nil {
			return
		}

		entryAddr = entry.MapFuncAddr(prog.image.Manifest().EntryAddrs, entryIndex)
	}

	stackSize := wa.PageSize

	instImage, err := image.NewInstance(prog.image, stackSize, entryIndex, entryAddr)
	if err != nil {
		return
	}

	inst = &InstanceImage{instImage, prog.buffers}
	return
}

func (inst *InstanceImage) Close() error {
	return inst.image.Close()
}
