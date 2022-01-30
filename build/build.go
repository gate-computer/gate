// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package build

import (
	"io"
	"io/ioutil"
	"math"

	"gate.computer/gate/image"
	internal "gate.computer/gate/internal/build"
	"gate.computer/gate/internal/error/badprogram"
	"gate.computer/gate/internal/error/notfound"
	"gate.computer/gate/internal/error/resourcelimit"
	"gate.computer/gate/internal/executable"
	"gate.computer/gate/snapshot"
	"gate.computer/gate/snapshot/wasm"
	"gate.computer/wag/binding"
	"gate.computer/wag/compile"
	"gate.computer/wag/object"
	"gate.computer/wag/section"
	"gate.computer/wag/wa"
)

type Build struct {
	Image                     *image.Build
	SectionMap                image.SectionMap
	Loaders                   map[string]section.CustomContentLoader
	Config                    compile.Config
	Module                    compile.Module
	StackSize                 int
	maxMemorySize             int // For instance.
	entryIndex                int
	Snapshot                  *snapshot.Snapshot
	breakpoints               map[uint32]compile.Breakpoint
	Buffers                   snapshot.Buffers
	bufferSectionHeaderLength int
}

func New(storage image.Storage, moduleSize, maxTextSize int, objectMap *object.CallMap, instance bool) (*Build, error) {
	img, err := image.NewBuild(storage, moduleSize, maxTextSize, objectMap, instance)
	if err != nil {
		return nil, err
	}

	b := &Build{
		Image:      img,
		Loaders:    make(map[string]section.CustomContentLoader),
		entryIndex: -1,
	}
	b.Config.ModuleMapper = &b.SectionMap
	return b, nil
}

func (b *Build) InstallEarlySnapshotLoaders() {
	b.Loaders[wasm.SectionSnapshot] = func(_ string, r section.Reader, length uint32) (err error) {
		b.installDuplicateSnapshotLoader()

		snap, n, err := wasm.ReadSnapshotSection(r)
		if err != nil {
			return
		}

		_, err = io.CopyN(ioutil.Discard, r, int64(length)-int64(n))
		if err != nil {
			return
		}

		b.SectionMap.Snapshot = b.SectionMap.Sections[section.Custom]
		b.Snapshot = &snap
		return
	}

	b.Loaders[wasm.SectionExport] = func(_ string, r section.Reader, length uint32) (err error) {
		if b.Snapshot == nil {
			err = badprogram.Error("gate.export section without gate.snapshot section")
			return
		}

		if length < 2 { // Minimum standard section frame size.
			err = badprogram.Error("gate.export section is too short")
			return
		}

		id, err := r.ReadByte()
		if err != nil {
			return
		}
		if id != byte(section.Export) {
			err = badprogram.Error("gate.export section does not contain a standard export section")
			return
		}
		err = r.UnreadByte()
		if err != nil {
			return
		}

		// Don't read payload; wag will load it as a standard section.
		// Duplicate sections are checked automatically.

		b.SectionMap.ExportWrap = b.SectionMap.Sections[section.Custom]
		return section.Unwrapped
	}

	b.Loaders[wasm.SectionBuffer] = func(_ string, r section.Reader, length uint32) (err error) {
		return badprogram.Error("gate.buffer section appears too early in wasm module")
	}

	b.Loaders[wasm.SectionStack] = func(string, section.Reader, uint32) error {
		return badprogram.Error("gate.stack section appears too early in wasm module")
	}

	// Loaders keys should not change after this.
	b.Config.CustomSectionLoader = section.CustomLoader(b.Loaders)
}

func (b *Build) ModuleConfig() *compile.ModuleConfig {
	return &compile.ModuleConfig{
		Config: b.Config,
	}
}

// SetMaxMemorySize after initial module sections have been loaded.
func (b *Build) SetMaxMemorySize(maxMemorySize int) error {
	if limit := b.Module.MemorySizeLimit(); limit >= 0 && maxMemorySize > limit {
		maxMemorySize = limit
	}
	b.maxMemorySize = alignMemorySize(maxMemorySize)

	if b.Module.InitialMemorySize() > b.maxMemorySize {
		return resourcelimit.Error("initial program memory size exceeds instance memory size limit")
	}

	return nil
}

// BindFunctions (imports and entry function) after initial module sections
// have been loaded.
func (b *Build) BindFunctions(entryName string) (err error) {
	err = binding.BindImports(&b.Module, b.Image.ImportResolver())
	if err != nil {
		return
	}

	if b.SectionMap.ExportWrap.Size != 0 {
		// Exports are hidden.
		if entryName != "" {
			err = notfound.ErrSuspended
			return
		}
	} else {
		b.entryIndex, err = internal.ResolveEntryFunc(b.Module, entryName, b.Snapshot != nil)
		if err != nil {
			return
		}
	}

	return
}

func (b *Build) CodeConfig(mapper compile.ObjectMapper) *compile.CodeConfig {
	if b.Snapshot != nil {
		b.breakpoints = make(map[uint32]compile.Breakpoint)
		for _, offset := range b.Snapshot.Breakpoints {
			if offset <= math.MaxUint32 {
				b.breakpoints[uint32(offset)] = compile.Breakpoint{}
			}
		}
	}

	return &compile.CodeConfig{
		Text:        b.Image.TextBuffer(),
		Mapper:      mapper,
		Breakpoints: b.breakpoints,
		Config:      b.Config,
	}
}

func (b *Build) VerifyBreakpoints() (err error) {
	if b.Snapshot == nil {
		return
	}

	for _, offset := range b.Snapshot.Breakpoints {
		if offset > math.MaxUint32 {
			err = badprogram.Errorf("breakpoint could not be set at offset 0x%x", offset)
			return
		}
	}

	for offset, bp := range b.breakpoints {
		if !bp.Set {
			err = badprogram.Errorf("breakpoint could not be set at offset 0x%x", offset)
			return
		}
	}

	return
}

func (b *Build) InstallSnapshotDataLoaders() {
	b.Loaders[wasm.SectionBuffer] = func(_ string, r section.Reader, length uint32) (err error) {
		b.installLateSnapshotLoader()
		b.installDuplicateBufferLoader()

		if b.Snapshot == nil {
			err = badprogram.Error("gate.buffer section without gate.snapshot section")
			return
		}

		var dataBuf []byte

		b.Buffers, b.bufferSectionHeaderLength, dataBuf, err = wasm.ReadBufferSectionHeader(r, length)
		if err != nil {
			return
		}

		_, err = io.ReadFull(r, dataBuf)
		if err != nil {
			return
		}

		_, err = io.CopyN(ioutil.Discard, r, int64(length)-int64(b.bufferSectionHeaderLength)-int64(len(dataBuf)))
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

		if b.Snapshot == nil {
			err = badprogram.Error("gate.stack section without gate.snapshot section")
			return
		}

		if b.entryIndex >= 0 {
			err = notfound.ErrSuspended
			return
		}

		if length > uint32(b.StackSize)-executable.StackUsageOffset {
			err = badprogram.Error("gate.stack section is too large")
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
		return badprogram.Error("multiple gate.snapshot sections in wasm module")
	}
}

func (b *Build) installDuplicateBufferLoader() {
	b.Loaders[wasm.SectionBuffer] = func(string, section.Reader, uint32) error {
		return badprogram.Error("multiple gate.buffer sections in wasm module")
	}
}

func (b *Build) installDuplicateStackLoader() {
	b.Loaders[wasm.SectionStack] = func(string, section.Reader, uint32) error {
		return badprogram.Error("multiple gate.stack sections in wasm module")
	}
}

func (b *Build) finishImageText(stackUsage int) error {
	return b.Image.FinishText(b.StackSize, stackUsage, b.Module.GlobalsSize(), b.Module.InitialMemorySize())
}

// FinishImageText after code and snapshot sections have been loaded.
func (b *Build) FinishImageText() (err error) {
	if b.SectionMap.Stack.Start != 0 {
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
		return badprogram.Error("gate.snapshot section appears too late in wasm module")
	}
}

func (b *Build) installLateBufferLoader() {
	b.Loaders[wasm.SectionBuffer] = func(string, section.Reader, uint32) error {
		return badprogram.Error("gate.buffer section appears too late in wasm module")
	}
}

func (b *Build) installLateStackLoader() {
	b.Loaders[wasm.SectionStack] = func(string, section.Reader, uint32) error {
		return badprogram.Error("gate.stack section appears too late in wasm module")
	}
}

// DataConfig is valid after FinishText.
func (b *Build) DataConfig() *compile.DataConfig {
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
	if i, ok := b.Module.StartFunc(); ok {
		startIndex = int(i)
	}

	return b.Image.FinishProgram(b.SectionMap, b.Module, startIndex, true, b.Snapshot, b.bufferSectionHeaderLength)
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
