// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package build

import (
	"io"

	"github.com/tsavola/gate/entry"
	"github.com/tsavola/gate/image"
	"github.com/tsavola/gate/internal/error/notfound"
	"github.com/tsavola/gate/internal/error/resourcelimit"
	"github.com/tsavola/gate/internal/executable"
	"github.com/tsavola/gate/snapshot"
	"github.com/tsavola/gate/snapshot/wasm"
	"github.com/tsavola/wag/binding"
	"github.com/tsavola/wag/compile"
	"github.com/tsavola/wag/object"
	"github.com/tsavola/wag/section"
	"github.com/tsavola/wag/wa"
)

type Build struct {
	Image         *image.Build
	SectionMap    image.SectionMap
	Loaders       section.CustomLoaders
	Config        compile.Config
	Module        compile.Module
	StackSize     int
	MaxMemorySize int
	entryIndex    int64
	Buffers       snapshot.Buffers
}

func New(storage image.Storage, moduleSize, maxTextSize int, objectMap *object.CallMap, instance bool,
) (b *Build, err error) {
	b = new(Build)

	b.Image, err = image.NewBuild(storage, moduleSize, maxTextSize, objectMap, instance)
	if err != nil {
		return
	}

	b.Loaders = make(section.CustomLoaders)

	b.Config = compile.Config{
		SectionMapper:       b.SectionMap.Mapper(),
		CustomSectionLoader: b.Loaders.Load,
	}

	b.entryIndex = -1

	return
}

func (b *Build) InstallPrematureSnapshotSectionLoaders(newError func(string) error) {
	b.Loaders[wasm.FlagSection] = func(_ string, r section.Reader, length uint32) (err error) {
		b.installDuplicateFlagSectionLoader(newError)

		b.SectionMap.Flag = b.SectionMap.Sections[section.Custom] // This section.

		b.Buffers.Flags, err = wasm.ReadFlagSection(r, length, newError)
		return
	}

	b.Loaders[wasm.ServiceSection] = func(_ string, r section.Reader, length uint32) (err error) {
		return newError("service section appears too early in wasm module")
	}

	b.Loaders[wasm.IOSection] = func(_ string, r section.Reader, length uint32) (err error) {
		return newError("io section appears too early in wasm module")
	}

	b.Loaders[wasm.BufferSection] = func(_ string, r section.Reader, length uint32) (err error) {
		return newError("buffer section appears too early in wasm module")
	}

	b.Loaders[wasm.StackSection] = func(string, section.Reader, uint32) error {
		return newError("stack section appears too early in wasm module")
	}
}

func (b Build) ModuleConfig() *compile.ModuleConfig {
	return &compile.ModuleConfig{
		Config: b.Config,
	}
}

// ConfigureMaxMemorySize after initial module sections have been loaded.
func (b *Build) ConfigureMaxMemorySize(maxMemorySizeLimit int) (err error) {
	b.MaxMemorySize = b.Module.MemorySizeLimit()
	if b.MaxMemorySize > maxMemorySizeLimit {
		b.MaxMemorySize = alignMemorySize(maxMemorySizeLimit)
	}

	if b.Module.InitialMemorySize() > b.MaxMemorySize {
		err = resourcelimit.New("initial program memory size exceeds instance memory size limit")
		return
	}

	return
}

// BindFunctions (imports and optional entry function) after initial module
// sections have been loaded.
func (b *Build) BindFunctions(entryName string) (err error) {
	err = binding.BindImports(&b.Module, b.Image.ImportResolver())
	if err != nil {
		return
	}

	if entryName != "" {
		var index uint32

		index, err = entry.ModuleFuncIndex(b.Module, entryName)
		if err != nil {
			return
		}

		b.entryIndex = int64(index)
	}

	return
}

func (b Build) CodeConfig() *compile.CodeConfig {
	return &compile.CodeConfig{
		Text:   b.Image.TextBuffer(),
		Mapper: b.Image.ObjectMap(),
		Config: b.Config,
	}
}

func (b *Build) InstallSnapshotSectionLoaders(newError func(string) error) {
	var serviceBuf []byte

	b.Loaders[wasm.ServiceSection] = func(_ string, r section.Reader, length uint32) (err error) {
		b.installDuplicateServiceSectionLoader(newError)

		b.SectionMap.Service = b.SectionMap.Sections[section.Custom] // This section.

		b.Buffers.Services, serviceBuf, err = wasm.ReadServiceSection(r, length, newError)
		return
	}

	b.Loaders[wasm.IOSection] = func(_ string, r section.Reader, length uint32) (err error) {
		b.installDuplicateServiceSectionLoader(newError)
		b.installDuplicateIOSectionLoader(newError)

		b.SectionMap.IO = b.SectionMap.Sections[section.Custom] // This section.

		b.Buffers.Input, b.Buffers.Output, err = wasm.ReadIOSection(r, length, newError)
		return
	}

	b.Loaders[wasm.BufferSection] = func(_ string, r section.Reader, length uint32) (err error) {
		b.installDuplicateServiceSectionLoader(newError)
		b.installDuplicateIOSectionLoader(newError)
		b.installDuplicateBufferSectionLoader(newError)

		if uint64(length) != uint64(len(serviceBuf)+len(b.Buffers.Input)+len(b.Buffers.Output)) {
			err = newError("unexpected buffer section length in wasm module")
		}

		b.SectionMap.Buffer = b.SectionMap.Sections[section.Custom] // This section.

		_, err = io.ReadFull(r, serviceBuf)
		if err != nil {
			return
		}

		_, err = io.ReadFull(r, b.Buffers.Input)
		if err != nil {
			return
		}

		_, err = io.ReadFull(r, b.Buffers.Output)
		if err != nil {
			return
		}

		return
	}

	b.Loaders[wasm.StackSection] = func(_ string, r section.Reader, length uint32) (err error) {
		b.installDuplicateServiceSectionLoader(newError)
		b.installDuplicateIOSectionLoader(newError)
		b.installDuplicateBufferSectionLoader(newError)
		b.installDuplicateStackSectionLoader(newError)

		if b.entryIndex >= 0 {
			err = notfound.ErrSuspended
			return
		}

		if length > uint32(b.StackSize)-executable.StackLimitOffset {
			err = newError("stack section is too large")
			return
		}

		b.SectionMap.Stack = b.SectionMap.Sections[section.Custom] // This section.

		err = b.finishImageText(int(length))
		if err != nil {
			return
		}

		err = b.Image.ReadStack(r, b.Module.Types(), b.Module.FuncTypeIndexes())
		if err != nil {
			return
		}

		return
	}
}

func (b *Build) installDuplicateFlagSectionLoader(newError func(string) error) {
	b.Loaders[wasm.FlagSection] = func(string, section.Reader, uint32) error {
		return newError("multiple flag sections in wasm module")
	}
}

func (b *Build) installDuplicateServiceSectionLoader(newError func(string) error) {
	b.Loaders[wasm.ServiceSection] = func(string, section.Reader, uint32) error {
		return newError("service section must appear before stack section in wasm module")
	}
}

func (b *Build) installDuplicateIOSectionLoader(newError func(string) error) {
	b.Loaders[wasm.IOSection] = func(string, section.Reader, uint32) error {
		return newError("io section must appear before stack section in wasm module")
	}
}

func (b *Build) installDuplicateBufferSectionLoader(newError func(string) error) {
	b.Loaders[wasm.BufferSection] = func(string, section.Reader, uint32) error {
		return newError("buffer section must appear before stack section in wasm module")
	}
}

func (b *Build) installDuplicateStackSectionLoader(newError func(string) error) {
	b.Loaders[wasm.StackSection] = func(string, section.Reader, uint32) error {
		return newError("multiple stack sections in wasm module")
	}
}

func (b *Build) finishImageText(stackUsage int) error {
	return b.Image.FinishText(b.StackSize, stackUsage, b.Module.GlobalsSize(), b.Module.InitialMemorySize(), b.MaxMemorySize)
}

// FinishImageText after code and snapshot sections have been loaded.
func (b *Build) FinishImageText() (err error) {
	if b.SectionMap.Stack.Offset != 0 {
		return // Already done by stack section loader.
	}

	return b.finishImageText(0)
}

func (b *Build) InstallLateSnapshotSectionLoaders(newError func(string) error) {
	b.Loaders[wasm.ServiceSection] = func(string, section.Reader, uint32) error {
		return newError("service section appears too late in wasm module")
	}

	b.Loaders[wasm.IOSection] = func(string, section.Reader, uint32) error {
		return newError("io section appears too late in wasm module")
	}

	b.Loaders[wasm.BufferSection] = func(string, section.Reader, uint32) error {
		return newError("buffer section appears too late in wasm module")
	}

	b.Loaders[wasm.StackSection] = func(string, section.Reader, uint32) error {
		return newError("stack section appears too late in wasm module")
	}
}

// DataConfig is valid after FinishText.
func (b Build) DataConfig() *compile.DataConfig {
	return &compile.DataConfig{
		GlobalsMemory:   b.Image.GlobalsMemoryBuffer(),
		MemoryAlignment: b.Image.MemoryAlignment(),
		Config:          b.Config,
	}
}

// FinishProgramImage after module, stack, globals and memory have been
// populated.
func (b *Build) FinishProgramImage() (*image.Program, error) {
	indexes, addrs := entry.Maps(b.Module, b.Image.ObjectMap().FuncAddrs)
	return b.Image.FinishProgram(b.SectionMap, b.Module.GlobalTypes(), indexes, addrs)
}

// FinishInstanceImage after program image has been finished.
func (b *Build) FinishInstanceImage() (inst *image.Instance, err error) {
	var entryAddr uint32

	if b.entryIndex >= 0 {
		entryAddr = b.Image.ObjectMap().FuncAddrs[b.entryIndex]
	}

	inst, err = b.Image.FinishInstance(uint32(b.entryIndex), entryAddr)
	if err != nil {
		return
	}

	return
}

func (b *Build) Close() error {
	return b.Image.Close()
}

func alignMemorySize(size int) int {
	mask := wa.PageSize - 1
	return (size + mask) &^ mask
}
