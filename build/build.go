// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package build

import (
	"io"
	"io/ioutil"
	"math"

	"github.com/tsavola/gate/image"
	internal "github.com/tsavola/gate/internal/build"
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
			err = badprogram.Err("gate.export section without gate.snapshot section")
			return
		}

		if length < 2 { // Minimum standard section frame size.
			err = badprogram.Err("gate.export section is too short")
			return
		}

		id, err := r.ReadByte()
		if err != nil {
			return
		}
		if id != byte(section.Export) {
			err = badprogram.Err("gate.export section does not contain a standard export section")
			return
		}
		err = r.UnreadByte()
		if err != nil {
			return
		}

		// Don't read payload; wag will load it as a standard section.
		// Duplicate sections are checked automatically.

		b.SectionMap.ExportWrap = b.SectionMap.Sections[section.Custom]
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

func (b *Build) ModuleConfig() *compile.ModuleConfig {
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

// BindFunctions (imports and entry function) after initial module sections
// have been loaded.
func (b *Build) BindFunctions(entryName string) (err error) {
	m := &b.SectionMap
	if m.ExportWrap.Length != 0 {
		exportLen := m.Sections[section.Export].Length

		// We didn't read the custom export section content, so offsets are
		// off.  Fix them.
		for i := int(section.Export); i < len(m.Sections); i++ {
			m.Sections[i].Offset -= exportLen
		}
		if m.Buffer.Length != 0 {
			m.Buffer.Offset -= exportLen
		}
		if m.Stack.Length != 0 {
			m.Stack.Offset -= exportLen
		}

		// Validate export wrapper payload length before accessing exports.
		exportEnd := m.Sections[section.Export].Offset + exportLen
		wrapperEnd := m.ExportWrap.Offset + m.ExportWrap.Length
		if exportEnd != wrapperEnd {
			err = badprogram.Err("gate.export section length does not match wrapped export section length")
			return
		}
	}

	err = binding.BindImports(&b.Module, b.Image.ImportResolver())
	if err != nil {
		return
	}

	if m.ExportWrap.Length != 0 {
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
			err = badprogram.Err("gate.buffer section without gate.snapshot section")
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
