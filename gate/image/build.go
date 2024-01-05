// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"crypto/rand"
	"errors"
	"io"
	"syscall"
	"unsafe"

	runtimeabi "gate.computer/gate/runtime/abi"
	"gate.computer/gate/snapshot"
	"gate.computer/internal/error/notfound"
	"gate.computer/internal/error/resourcelimit"
	internal "gate.computer/internal/executable"
	"gate.computer/internal/file"
	"gate.computer/internal/manifest"
	"gate.computer/wag/buffer"
	"gate.computer/wag/compile"
	"gate.computer/wag/object"
	"gate.computer/wag/wa"
)

const TextRevision = 0

const (
	progTextOffset        = int64(0x000000000)
	_                     = int64(0x080000000) // Stack space; aligned against globals.
	progGlobalsPageOffset = int64(0x100000000) // Globals and memory in consecutive pages.
	progModuleOffset      = int64(0x200000000) // Module and object map packed with minimal alignment.
	progManifestOffset    = int64(0x400000000)
	progMaxOffset         = int64(0x480000000)
)

type programBuild struct {
	file      *file.File
	text      buffer.Static
	textSize  int
	moduleMem []byte
	module    buffer.Static
	objectMap *object.CallMap
}

type instanceBuild struct {
	enabled   bool
	file      *file.File
	stackSize int
}

// Build a program and optionally an instance.  FinishText, FinishProgram and
// (optionally) FinishInstance must be called in that order.
type Build struct {
	storage     Storage
	prog        programBuild
	inst        instanceBuild
	imports     runtimeabi.ImportResolver
	compileMem  []byte
	textAddr    uint64
	stack       []byte
	stackUsage  int
	data        buffer.Static
	globalsSize int
	memorySize  int
}

// NewBuild for a program and optionally an instance.
func NewBuild(storage Storage, moduleSize, maxTextSize int, objectMap *object.CallMap, instance bool) (*Build, error) {
	b := &Build{
		storage: storage,
		prog: programBuild{
			objectMap: objectMap,
		},
		inst: instanceBuild{
			enabled: instance,
		},
	}

	var ok bool
	var err error

	b.prog.file, err = b.storage.newProgramFile()
	if err != nil {
		return nil, err
	}
	defer func() {
		if !ok {
			b.munmapAll()
			b.prog.file.Close()
		}
	}()

	// Program text.
	if err := mmapp(&b.compileMem, b.prog.file, progTextOffset, maxTextSize); err != nil {
		return nil, err
	}

	b.prog.text = buffer.MakeStatic(b.compileMem[:0:maxTextSize])

	if moduleSize > 0 {
		// Program module.
		if err := mmapp(&b.prog.moduleMem, b.prog.file, progModuleOffset, moduleSize); err != nil {
			return nil, err
		}

		b.prog.module = buffer.MakeStatic(b.prog.moduleMem[:0:moduleSize])
	}

	ok = true
	return b, nil
}

func (b *Build) ObjectMap() *object.CallMap {
	return b.prog.objectMap
}

// ModuleWriter is valid after NewBuild.  The module must be written before
// FinishProgram is called.
func (b *Build) ModuleWriter() io.Writer {
	return &b.prog.module
}

func (b *Build) ImportResolver() interface {
	ResolveFunc(module, field string, sig wa.FuncType) (index uint32, err error)
	ResolveGlobal(module, field string, t wa.Type) (value uint64, err error)
} {
	return &b.imports
}

// TextBuffer is valid after NewBuild.  It must be populated before FinishText
// is called.
func (b *Build) TextBuffer() interface {
	Bytes() []byte
	Extend(n int) []byte
	PutByte(byte)
	PutUint32(uint32) // Little-endian byte order.
} {
	return &b.prog.text
}

// FinishText after TextBuffer has been populated.
func (b *Build) FinishText(stackSize, stackUsage, globalsSize, memorySize int) error {
	if stackSize < internal.StackUsageOffset+stackUsage {
		return resourcelimit.Error("call stack size limit exceeded")
	}

	b.prog.textSize = b.prog.text.Len()
	b.prog.text = buffer.Static{}

	munmapp(&b.compileMem)

	var (
		stackMapSize   = alignPageSize(stackUsage)
		globalsMapSize = alignPageSize(globalsSize)
		dataMapSize    = globalsMapSize + alignPageSize(memorySize)
		mapSize        = stackMapSize + dataMapSize
	)

	if !b.inst.enabled {
		// Program stack, globals and memory contents.
		err := mmapp(&b.compileMem, b.prog.file, progGlobalsPageOffset-int64(stackMapSize), mapSize)
		if err != nil {
			return err
		}
	} else {
		b.inst.stackSize = alignPageSize(stackSize)

		f, err := b.storage.newInstanceFile()
		if err != nil {
			return err
		}
		defer func() {
			if f != nil {
				f.Close()
			}
		}()

		// Instance stack, globals and memory contents.
		err = mmapp(&b.compileMem, f, int64(b.inst.stackSize-stackMapSize), mapSize)
		if err != nil {
			return err
		}

		b.inst.file = f
		f = nil
	}

	b.stack = b.compileMem[stackMapSize-stackUsage : stackMapSize : stackMapSize]
	b.data = buffer.MakeStatic(b.compileMem[stackMapSize:stackMapSize:mapSize])
	b.globalsSize = globalsSize
	b.memorySize = memorySize

	// Copy or write object map to program.
	var (
		progCallSitesOffset = progModuleOffset + align8(int64(b.prog.module.Cap()))
		progFuncAddrsOffset = progCallSitesOffset + int64(callSitesSize(b.prog.objectMap))
		progObjectMapEnd    = alignPageOffset(progFuncAddrsOffset + int64(funcAddrsSize(b.prog.objectMap)))
	)

	if progObjectMapEnd-progModuleOffset <= int64(len(b.prog.moduleMem)) {
		copyObjectMapTo(b.prog.moduleMem[progCallSitesOffset-progModuleOffset:], b.prog.objectMap)
	} else {
		writeObjectMapAt(b.prog.file, b.prog.objectMap, progCallSitesOffset)
	}

	return nil
}

// ReadStack if FinishText has been called with nonzero stackUsage.  It must
// not be called after FinishProgram.
func (b *Build) ReadStack(r io.Reader, types []wa.FuncType, funcTypeIndexes []uint32) error {
	if _, err := io.ReadFull(r, b.stack); err != nil {
		return err
	}

	textAddr, err := generateRandTextAddr()
	if err != nil {
		return err
	}

	if err := importStack(b.stack, textAddr, *b.prog.objectMap, types, funcTypeIndexes); err != nil {
		return err
	}

	b.textAddr = textAddr
	b.stackUsage = len(b.stack)
	return nil
}

// GlobalsMemoryBuffer is valid after FinishText.  It must be populated before
// FinishProgram is called.
func (b *Build) GlobalsMemoryBuffer() interface {
	Bytes() []byte
	ResizeBytes(n int) []byte
} {
	return &b.data
}

// MemoryAlignment of GlobalsMemoryBuffer.
func (b *Build) MemoryAlignment() int {
	return internal.PageSize
}

// FinishProgram after module, stack, globals and memory have been populated.
func (b *Build) FinishProgram(sectionMap SectionMap, mod compile.Module, startFuncIndex int, entryFuncs bool, snap *snapshot.Snapshot, bufferSectionHeaderLength int) (*Program, error) {
	if b.stackUsage != len(b.stack) {
		return nil, errors.New("stack was not populated")
	}

	if b.inst.enabled {
		// Copy stack, globals and memory from instance file to program file.
		var (
			stackMapSize = alignPageSize(b.stackUsage)
			off1         = int64(b.inst.stackSize - stackMapSize)
			off2         = progGlobalsPageOffset - int64(stackMapSize)
			copyLen      = stackMapSize + alignPageSize(b.data.Len())
		)

		if err := copyFileRange(b.inst.file, &off1, b.prog.file, &off2, copyLen); err != nil {
			return nil, err
		}
	}

	if err := b.storage.protectProgramFile(b.prog.file); err != nil {
		return nil, err
	}

	man := &manifest.Program{
		LibraryChecksum:         runtimeabi.LibraryChecksum(),
		TextRevision:            TextRevision,
		TextAddr:                b.textAddr,
		TextSize:                uint32(b.prog.textSize),
		StackUsage:              uint32(b.stackUsage),
		GlobalsSize:             uint32(b.globalsSize),
		MemorySize:              uint32(b.memorySize),
		MemorySizeLimit:         int64(mod.MemorySizeLimit()),
		MemoryDataSize:          uint32(b.data.Len() - alignPageSize(b.globalsSize)),
		ModuleSize:              int64(b.prog.module.Cap()),
		Sections:                sectionMap.manifestSections(),
		SnapshotSection:         manifestByteRange(sectionMap.Snapshot),
		BufferSection:           manifestByteRange(sectionMap.Buffer),
		BufferSectionHeaderSize: uint32(bufferSectionHeaderLength),
		StackSection:            manifestByteRange(sectionMap.Stack),
		GlobalTypes:             globalTypeBytes(mod.GlobalTypes()),
		CallSitesSize:           uint32(callSitesSize(b.prog.objectMap)),
		FuncAddrsSize:           uint32(funcAddrsSize(b.prog.objectMap)),
		Random:                  b.imports.Random,
	}
	if startFuncIndex >= 0 {
		man.StartFunc = &manifest.Function{
			Index: uint32(startFuncIndex),
			Addr:  b.prog.objectMap.FuncAddrs[startFuncIndex],
		}
	}
	if entryFuncs {
		man.InitEntryFuncs(mod, b.prog.objectMap.FuncAddrs)
	}
	if snap != nil {
		man.Snapshot = &manifest.Snapshot{
			MonotonicTime: snap.MonotonicTime,
			Breakpoints:   manifest.SortDedupUint64(snap.Breakpoints),
		}
	}

	b.stack = nil
	b.data = buffer.Static{}
	b.prog.module = buffer.Static{}

	b.munmapAll()

	prog := &Program{
		Map:     *b.prog.objectMap,
		storage: b.storage,
		man:     man,
		file:    b.prog.file,
	}
	b.prog.file = nil
	return prog, nil
}

// FinishInstance after FinishProgram.  Applicable only if an instance storage
// was specified in NewBuild call.
func (b *Build) FinishInstance(prog *Program, maxMemorySize, entryFuncIndex int) (*Instance, error) {
	maxMemorySize, err := maxInstanceMemory(prog, maxMemorySize)
	if err != nil {
		return nil, err
	}

	if entryFuncIndex >= 0 && b.stackUsage != 0 {
		return nil, notfound.ErrSuspended
	}

	inst := &Instance{
		man: &manifest.Instance{
			TextAddr:      b.textAddr,
			StackSize:     uint32(b.inst.stackSize),
			StackUsage:    uint32(b.stackUsage),
			GlobalsSize:   uint32(b.globalsSize),
			MemorySize:    uint32(b.memorySize),
			MaxMemorySize: uint32(maxMemorySize),
			StartFunc:     prog.man.StartFunc,
			EntryFunc:     prog.man.EntryFunc(entryFuncIndex),
			Snapshot:      prog.man.Snapshot.Clone(),
		},
		file:     b.inst.file,
		coherent: true,
	}
	b.inst.file = nil
	return inst, nil
}

func (b *Build) Close() (err error) {
	setError := func(e error) {
		if err == nil {
			err = e
		}
	}

	b.prog.text = buffer.Static{}
	b.stack = nil
	b.data = buffer.Static{}
	b.prog.module = buffer.Static{}

	if b.prog.file != nil {
		setError(b.prog.file.Close())
		b.prog.file = nil
	}
	if b.inst.file != nil {
		setError(b.inst.file.Close())
		b.inst.file = nil
	}

	b.munmapAll()
	return
}

func (b *Build) munmapAll() {
	munmapp(&b.compileMem)
	munmapp(&b.prog.moduleMem)
}

// mmapp rounds length up to page.
func mmapp(ptr *[]byte, f *file.File, offset int64, length int) error {
	if *ptr != nil {
		panic("memory already mapped")
	}

	if length == 0 {
		return nil
	}

	b, err := mmap(f.FD(), offset, alignPageSize(length), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		return err
	}

	*ptr = b
	return nil
}

func munmapp(ptr *[]byte) {
	b := *ptr
	*ptr = nil

	if b != nil {
		mustMunmap(b)
	}
}

func globalTypeBytes(array []wa.GlobalType) []byte {
	if len(array) == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(&array[0])), len(array))
}

func generateRandTextAddr() (uint64, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return 0, err
	}

	return internal.RandAddr(internal.MinTextAddr, internal.MaxTextAddr, b), nil
}
