// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package build

import (
	"io"
	"math"

	"gate.computer/gate/image"
	"gate.computer/gate/snapshot"
	"gate.computer/gate/snapshot/wasm"
	"gate.computer/internal/build"
	"gate.computer/internal/error/badprogram"
	"gate.computer/internal/error/notfound"
	"gate.computer/internal/error/resourcelimit"
	"gate.computer/internal/executable"
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
	Buffers                   *snapshot.Buffers
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
	b.Loaders[wasm.SectionSnapshot] = func(_ string, r section.Reader, length uint32) error {
		b.installDuplicateSnapshotLoader()

		snap, n, err := wasm.ReadSnapshotSection(r)
		if err != nil {
			return err
		}

		if _, err := io.CopyN(io.Discard, r, int64(length)-int64(n)); err != nil {
			return err
		}

		b.SectionMap.Snapshot = b.SectionMap.Sections[section.Custom]
		b.Snapshot = &snap
		return nil
	}

	b.Loaders[wasm.SectionExport] = func(_ string, r section.Reader, length uint32) error {
		if b.Snapshot == nil {
			return badprogram.Error("gate.export section without gate.snapshot section")
		}

		if length < 2 { // Minimum standard section frame size.
			return badprogram.Error("gate.export section is too short")
		}

		id, err := r.ReadByte()
		if err != nil {
			return err
		}
		if id != byte(section.Export) {
			return badprogram.Error("gate.export section does not contain a standard export section")
		}
		if err := r.UnreadByte(); err != nil {
			return err
		}

		// Don't read payload; wag will load it as a standard section.
		// Duplicate sections are checked automatically.

		b.SectionMap.ExportWrap = b.SectionMap.Sections[section.Custom]
		return section.Unwrapped
	}

	b.Loaders[wasm.SectionBuffer] = func(_ string, r section.Reader, length uint32) error {
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
func (b *Build) BindFunctions(entryName string) error {
	if err := binding.BindImports(&b.Module, b.Image.ImportResolver()); err != nil {
		return err
	}

	if b.SectionMap.ExportWrap.Size != 0 {
		// Exports are hidden.
		if entryName != "" {
			return notfound.ErrSuspended
		}
	} else {
		index, err := build.ResolveEntryFunc(b.Module, entryName, b.Snapshot != nil)
		if err != nil {
			return err
		}
		b.entryIndex = index
	}

	return nil
}

func (b *Build) CodeConfig(mapper compile.ObjectMapper) *compile.CodeConfig {
	if b.Snapshot != nil {
		b.breakpoints = make(map[uint32]compile.Breakpoint, len(b.Snapshot.Breakpoints))
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

func (b *Build) VerifyBreakpoints() error {
	if b.Snapshot == nil {
		return nil
	}

	for _, offset := range b.Snapshot.Breakpoints {
		if offset > math.MaxUint32 {
			return badprogram.Errorf("breakpoint could not be set at offset 0x%x", offset)
		}
	}

	for offset, bp := range b.breakpoints {
		if !bp.Set {
			return badprogram.Errorf("breakpoint could not be set at offset 0x%x", offset)
		}
	}

	return nil
}

func (b *Build) InstallSnapshotDataLoaders() {
	b.Loaders[wasm.SectionBuffer] = func(_ string, r section.Reader, length uint32) error {
		b.installLateSnapshotLoader()
		b.installDuplicateBufferLoader()

		if b.Snapshot == nil {
			return badprogram.Error("gate.buffer section without gate.snapshot section")
		}

		bs, readLen, dataBuf, err := wasm.ReadBufferSectionHeader(r, length)
		if err != nil {
			return err
		}

		b.Buffers = bs
		b.bufferSectionHeaderLength = readLen

		if _, err := io.ReadFull(r, dataBuf); err != nil {
			return err
		}

		if _, err := io.CopyN(io.Discard, r, int64(length)-int64(b.bufferSectionHeaderLength)-int64(len(dataBuf))); err != nil {
			return err
		}

		b.SectionMap.Buffer = b.SectionMap.Sections[section.Custom]
		return nil
	}

	b.Loaders[wasm.SectionStack] = func(_ string, r section.Reader, length uint32) error {
		b.installLateSnapshotLoader()
		b.installLateBufferLoader()
		b.installDuplicateStackLoader()

		if b.Snapshot == nil {
			return badprogram.Error("gate.stack section without gate.snapshot section")
		}

		if b.entryIndex >= 0 {
			return notfound.ErrSuspended
		}

		if length > uint32(b.StackSize)-executable.StackUsageOffset {
			return badprogram.Error("gate.stack section is too large")
		}

		if err := b.finishImageText(int(length)); err != nil {
			return err
		}

		if err := b.Image.ReadStack(r, b.Module.Types(), b.Module.FuncTypeIndexes()); err != nil {
			return err
		}

		b.SectionMap.Stack = b.SectionMap.Sections[section.Custom]
		return nil
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
func (b *Build) FinishImageText() error {
	if b.SectionMap.Stack.Start != 0 {
		return nil // Already done by stack section loader.
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
