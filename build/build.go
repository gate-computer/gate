// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package build

import (
	"fmt"
	"io"
	"io/ioutil"

	"github.com/tsavola/gate/entry"
	"github.com/tsavola/gate/image"
	"github.com/tsavola/gate/image/manifest"
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

const maxPacketSize = 65536

type Build struct {
	Image         *image.Build
	SectionMap    image.SectionMap
	Loaders       section.CustomLoaders
	Config        compile.Config
	Module        compile.Module
	StackSize     int
	MaxMemorySize int
	entryIndex    int64
	snapshot      *manifest.Snapshot
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

func (b *Build) InstallEarlySnapshotLoaders(newError func(string) error) {
	b.Loaders[wasm.SectionSnapshot] = func(_ string, r section.Reader, length uint32) (err error) {
		b.installDuplicateSnapshotLoader(newError)

		if length == 0 {
			err = newError("gate.snapshot section is empty")
			return
		}

		version, err := r.ReadByte()
		if err != nil {
			return
		}
		length--
		if version != wasm.SnapshotVersion {
			err = newError(fmt.Sprintf("unsupported snapshot version: %d", version))
			return
		}

		monotime, n, err := readVaruint64(r, newError)
		length -= uint32(n)
		if err != nil {
			return
		}

		b.snapshot = &manifest.Snapshot{
			MonotonicTime: monotime,
		}

		_, err = io.CopyN(ioutil.Discard, r, int64(length))
		if err != nil {
			return
		}

		return
	}

	b.Loaders[wasm.SectionBuffer] = func(_ string, r section.Reader, length uint32) (err error) {
		return newError("gate.buffer section appears too early in wasm module")
	}

	b.Loaders[wasm.SectionStack] = func(string, section.Reader, uint32) error {
		return newError("gate.stack section appears too early in wasm module")
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

	if _, startSection := b.Module.StartFunc(); !startSection {
		if index, sig, found := b.Module.ExportFunc(entry.StartFuncName); found {
			if sig.Equal(entry.StartFuncType) {
				b.Module.SetStartFunc(index)
			}
		}
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

func (b Build) CodeConfig(mapper compile.ObjectMapper) *compile.CodeConfig {
	return &compile.CodeConfig{
		Text:   b.Image.TextBuffer(),
		Mapper: mapper,
		Config: b.Config,
	}
}

func (b *Build) InstallSnapshotDataLoaders(newError func(string) error) {
	b.Loaders[wasm.SectionBuffer] = func(_ string, r section.Reader, length uint32) (err error) {
		b.installLateSnapshotLoader(newError)
		b.installDuplicateBufferLoader(newError)

		if b.snapshot == nil {
			err = newError("gate.buffer section without gate.snapshot section")
			return
		}

		var n int
		var dataBuf []byte

		b.Buffers, n, dataBuf, err = wasm.ReadBufferSectionHeader(r, length, newError)
		if err != nil {
			return
		}

		_, err = io.ReadFull(r, dataBuf)
		if err != nil {
			return
		}

		_, err = io.CopyN(ioutil.Discard, r, int64(length)-int64(n)-int64(len(dataBuf)))
		if err != nil {
			return
		}

		b.SectionMap.Buffer = b.SectionMap.Sections[section.Custom]
		return
	}

	b.Loaders[wasm.SectionStack] = func(_ string, r section.Reader, length uint32) (err error) {
		b.installLateSnapshotLoader(newError)
		b.installLateBufferLoader(newError)
		b.installDuplicateStackLoader(newError)

		if b.snapshot == nil {
			err = newError("gate.stack section without gate.snapshot section")
			return
		}

		if b.entryIndex >= 0 {
			err = notfound.ErrSuspended
			return
		}

		if length > uint32(b.StackSize)-executable.StackUsageOffset {
			err = newError("gate.stack section is too large")
			return
		}

		err = b.finishImageText(int(length))
		if err != nil {
			return
		}

		err = b.Image.ReadStack(r, b.Module.Types(), b.Module.FuncTypeIndexes())
		if err != nil {
			return
		}

		b.SectionMap.Stack = b.SectionMap.Sections[section.Custom]
		return
	}
}

func (b *Build) installDuplicateSnapshotLoader(newError func(string) error) {
	b.Loaders[wasm.SectionSnapshot] = func(string, section.Reader, uint32) error {
		return newError("multiple gate.snapshot sections in wasm module")
	}
}

func (b *Build) installDuplicateBufferLoader(newError func(string) error) {
	b.Loaders[wasm.SectionBuffer] = func(string, section.Reader, uint32) error {
		return newError("multiple gate.buffer sections in wasm module")
	}
}

func (b *Build) installDuplicateStackLoader(newError func(string) error) {
	b.Loaders[wasm.SectionStack] = func(string, section.Reader, uint32) error {
		return newError("multiple gate.stack sections in wasm module")
	}
}

func (b *Build) finishImageText(stackUsage int) error {
	return b.Image.FinishText(b.StackSize, stackUsage, b.Module.GlobalsSize(), b.Module.InitialMemorySize(), b.MaxMemorySize, b.snapshot)
}

// FinishImageText after code and snapshot sections have been loaded.
func (b *Build) FinishImageText() (err error) {
	if b.SectionMap.Stack.Offset != 0 {
		return // Already done by stack section loader.
	}

	return b.finishImageText(0)
}

func (b *Build) InstallLateSnapshotLoaders(newError func(string) error) {
	b.installLateSnapshotLoader(newError)
	b.installLateBufferLoader(newError)
	b.installLateStackLoader(newError)
}

func (b *Build) installLateSnapshotLoader(newError func(string) error) {
	b.Loaders[wasm.SectionSnapshot] = func(string, section.Reader, uint32) error {
		return newError("gate.snapshot section appears too late in wasm module")
	}
}

func (b *Build) installLateBufferLoader(newError func(string) error) {
	b.Loaders[wasm.SectionBuffer] = func(string, section.Reader, uint32) error {
		return newError("gate.buffer section appears too late in wasm module")
	}
}

func (b *Build) installLateStackLoader(newError func(string) error) {
	b.Loaders[wasm.SectionStack] = func(string, section.Reader, uint32) error {
		return newError("gate.stack section appears too late in wasm module")
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

func readVaruint64(r section.Reader, newError func(string) error) (x uint64, n int, err error) {
	var shift uint
	for n = 1; ; n++ {
		var b byte
		b, err = r.ReadByte()
		if err != nil {
			return
		}
		if b < 0x80 {
			if n > 9 || n == 9 && b > 1 {
				err = newError("varuint64 is too large")
				return
			}
			x |= uint64(b) << shift
			return
		}
		x |= (uint64(b) & 0x7f) << shift
		shift += 7
	}
}
