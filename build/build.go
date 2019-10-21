// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package build

import (
	"fmt"
	"io"
	"io/ioutil"

	"github.com/tsavola/gate/build/resolve"
	"github.com/tsavola/gate/image"
	"github.com/tsavola/gate/internal/error/badprogram"
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
const minSnapshotVersion = 0

type Build struct {
	Image         *image.Build
	SectionMap    image.SectionMap
	Loaders       map[string]section.CustomContentLoader
	Config        compile.Config
	Module        compile.Module
	StackSize     int
	maxMemorySize int // For instance.
	entryIndex    int
	snapshot      bool
	monotonicTime uint64
	Buffers       snapshot.Buffers
}

func New(storage image.Storage, moduleSize, maxTextSize int, objectMap *object.CallMap, instance bool,
) (b *Build, err error) {
	b = new(Build)

	b.Image, err = image.NewBuild(storage, moduleSize, maxTextSize, objectMap, instance)
	if err != nil {
		return
	}

	b.Loaders = make(map[string]section.CustomContentLoader)

	b.Config = compile.Config{
		SectionMapper: b.SectionMap.Mapper(),
	}

	b.entryIndex = -1
	return
}

func (b *Build) InstallEarlySnapshotLoaders() {
	b.Loaders[wasm.SectionSnapshot] = func(_ string, r section.Reader, length uint32) (err error) {
		b.installDuplicateSnapshotLoader()

		if length == 0 {
			err = badprogram.Err("gate.snapshot section is empty")
			return
		}

		version, n, err := readVaruint64(r)
		length -= uint32(n)
		if err != nil {
			return
		}
		if version < minSnapshotVersion {
			err = badprogram.Err(fmt.Sprintf("unsupported snapshot version: %d", version))
			return
		}

		_, n, err = readVaruint64(r) // Flags
		length -= uint32(n)
		if err != nil {
			return
		}

		b.monotonicTime, n, err = readVaruint64(r)
		length -= uint32(n)
		if err != nil {
			return
		}

		_, err = io.CopyN(ioutil.Discard, r, int64(length))
		if err != nil {
			return
		}

		b.SectionMap.Snapshot = b.SectionMap.Sections[section.Custom]
		b.snapshot = true
		return
	}

	b.Loaders[wasm.SectionBuffer] = func(_ string, r section.Reader, length uint32) (err error) {
		return badprogram.Err("gate.buffer section appears too early in wasm module")
	}

	b.Loaders[wasm.SectionStack] = func(string, section.Reader, uint32) error {
		return badprogram.Err("gate.stack section appears too early in wasm module")
	}

	// Loaders keys should not change after this.
	b.Config.CustomSectionLoader = section.CustomLoader(b.Loaders)
}

func (b Build) ModuleConfig() *compile.ModuleConfig {
	return &compile.ModuleConfig{
		Config: b.Config,
	}
}

// SetMaxMemorySize after initial module sections have been loaded.
func (b *Build) SetMaxMemorySize(maxMemorySize int) (err error) {
	if limit := b.Module.MemorySizeLimit(); limit >= 0 && maxMemorySize > limit {
		maxMemorySize = limit
	}
	b.maxMemorySize = alignMemorySize(maxMemorySize)

	if b.Module.InitialMemorySize() > b.maxMemorySize {
		err = resourcelimit.New("initial program memory size exceeds instance memory size limit")
		return
	}

	return
}

// BindFunctions (imports and optional start and entry functions) after initial
// module sections have been loaded.
func (b *Build) BindFunctions(entryName string) (err error) {
	err = binding.BindImports(&b.Module, b.Image.ImportResolver())
	if err != nil {
		return
	}

	b.entryIndex, err = resolve.EntryFunc(b.Module, entryName)
	if err != nil {
		return
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

func (b *Build) InstallSnapshotDataLoaders() {
	b.Loaders[wasm.SectionBuffer] = func(_ string, r section.Reader, length uint32) (err error) {
		b.installLateSnapshotLoader()
		b.installDuplicateBufferLoader()

		if !b.snapshot {
			err = badprogram.Err("gate.buffer section without gate.snapshot section")
			return
		}

		var n int
		var dataBuf []byte

		b.Buffers, n, dataBuf, err = wasm.ReadBufferSectionHeader(r, length)
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
		b.installLateSnapshotLoader()
		b.installLateBufferLoader()
		b.installDuplicateStackLoader()

		if !b.snapshot {
			err = badprogram.Err("gate.stack section without gate.snapshot section")
			return
		}

		if b.entryIndex >= 0 {
			err = notfound.ErrSuspended
			return
		}

		if length > uint32(b.StackSize)-executable.StackUsageOffset {
			err = badprogram.Err("gate.stack section is too large")
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

func (b *Build) installDuplicateSnapshotLoader() {
	b.Loaders[wasm.SectionSnapshot] = func(string, section.Reader, uint32) error {
		return badprogram.Err("multiple gate.snapshot sections in wasm module")
	}
}

func (b *Build) installDuplicateBufferLoader() {
	b.Loaders[wasm.SectionBuffer] = func(string, section.Reader, uint32) error {
		return badprogram.Err("multiple gate.buffer sections in wasm module")
	}
}

func (b *Build) installDuplicateStackLoader() {
	b.Loaders[wasm.SectionStack] = func(string, section.Reader, uint32) error {
		return badprogram.Err("multiple gate.stack sections in wasm module")
	}
}

func (b *Build) finishImageText(stackUsage int) error {
	return b.Image.FinishText(b.StackSize, stackUsage, b.Module.GlobalsSize(), b.Module.InitialMemorySize())
}

// FinishImageText after code and snapshot sections have been loaded.
func (b *Build) FinishImageText() (err error) {
	if b.SectionMap.Stack.Offset != 0 {
		return // Already done by stack section loader.
	}

	return b.finishImageText(0)
}

func (b *Build) InstallLateSnapshotLoaders() {
	b.installLateSnapshotLoader()
	b.installLateBufferLoader()
	b.installLateStackLoader()
}

func (b *Build) installLateSnapshotLoader() {
	b.Loaders[wasm.SectionSnapshot] = func(string, section.Reader, uint32) error {
		return badprogram.Err("gate.snapshot section appears too late in wasm module")
	}
}

func (b *Build) installLateBufferLoader() {
	b.Loaders[wasm.SectionBuffer] = func(string, section.Reader, uint32) error {
		return badprogram.Err("gate.buffer section appears too late in wasm module")
	}
}

func (b *Build) installLateStackLoader() {
	b.Loaders[wasm.SectionStack] = func(string, section.Reader, uint32) error {
		return badprogram.Err("gate.stack section appears too late in wasm module")
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
	startIndex := -1
	if index, found := b.Module.StartFunc(); found {
		startIndex = int(index)
	} else if index, sig, found := b.Module.ExportFunc("_start"); found {
		if binding.IsStartFuncType(sig) {
			startIndex = int(index)
		}
	}

	return b.Image.FinishProgram(b.SectionMap, b.Module, startIndex, true, b.monotonicTime)
}

// FinishInstanceImage after program image has been finished.
func (b *Build) FinishInstanceImage(prog *image.Program) (*image.Instance, error) {
	return b.Image.FinishInstance(prog, b.maxMemorySize, b.entryIndex)
}

func (b *Build) Close() error {
	return b.Image.Close()
}

func alignMemorySize(size int) int {
	mask := wa.PageSize - 1
	return (size + mask) &^ mask
}

func readVaruint64(r section.Reader) (x uint64, n int, err error) {
	var shift uint
	for n = 1; ; n++ {
		var b byte
		b, err = r.ReadByte()
		if err != nil {
			return
		}
		if b < 0x80 {
			if n > 9 || n == 9 && b > 1 {
				err = badprogram.Err("varuint64 is too large")
				return
			}
			x |= uint64(b) << shift
			return
		}
		x |= (uint64(b) & 0x7f) << shift
		shift += 7
	}
}
