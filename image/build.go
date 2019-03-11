// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"crypto/rand"
	"errors"
	"io"
	"reflect"
	"syscall"
	"unsafe"

	"github.com/tsavola/gate/internal/error/notfound"
	"github.com/tsavola/gate/internal/error/resourcelimit"
	internal "github.com/tsavola/gate/internal/executable"
	"github.com/tsavola/gate/internal/file"
	"github.com/tsavola/gate/internal/manifest"
	"github.com/tsavola/wag/buffer"
	"github.com/tsavola/wag/object"
	"github.com/tsavola/wag/object/abi"
	"github.com/tsavola/wag/wa"
)

const (
	progTextOffset     = int64(0x000000000)
	_                  = int64(0x080000000) // Stack space; aligned against globals.
	progGlobalsOffset  = int64(0x100000000) // Globals and memory in consecutive pages.
	progModuleOffset   = int64(0x200000000) // Module and object map packed with minimal alignment.
	progManifestOffset = int64(0x400000000)
	progMaxOffset      = int64(0x480000000)
)

type programBuild struct {
	file      *file.File
	text      buffer.Static
	textSize  int
	moduleMem []byte
	module    buffer.Static
	objectMap *object.CallMap
}

func (prog *programBuild) callSitesSize() int {
	return len(prog.objectMap.CallSites) * 8
}

func (prog *programBuild) callSitesBytes() []byte {
	n := prog.callSitesSize()
	if n == 0 {
		return nil
	}

	return *(*[]byte)(unsafe.Pointer(&reflect.SliceHeader{
		Len:  n,
		Cap:  n,
		Data: (uintptr)(unsafe.Pointer(&prog.objectMap.CallSites[0])),
	}))
}

func (prog *programBuild) funcAddrsSize() int {
	return len(prog.objectMap.FuncAddrs) * 4
}

func (prog *programBuild) funcAddrsBytes() []byte {
	n := prog.funcAddrsSize()
	if n == 0 {
		return nil
	}

	return *(*[]byte)(unsafe.Pointer(&reflect.SliceHeader{
		Len:  n,
		Cap:  n,
		Data: (uintptr)(unsafe.Pointer(&prog.objectMap.FuncAddrs[0])),
	}))
}

func (prog *programBuild) copyObjectMapTo(dest []byte) {
	copy(dest, prog.callSitesBytes())
	copy(dest[prog.callSitesSize():], prog.funcAddrsBytes())
}

func (prog *programBuild) writeObjectMapAt(offset int64) (err error) {
	iov := make([]syscall.Iovec, 0, 2)

	if n := prog.callSitesSize(); n > 0 {
		iov = append(iov, syscall.Iovec{
			Base: &prog.callSitesBytes()[0],
			Len:  uint64(n),
		})
	}

	if n := prog.funcAddrsSize(); n > 0 {
		iov = append(iov, syscall.Iovec{
			Base: &prog.funcAddrsBytes()[0],
			Len:  uint64(n),
		})
	}

	return pwritev(prog.file.Fd(), iov, offset)
}

type instanceBuild struct {
	enabled   bool
	file      *file.File
	stackSize int
}

// Build a program and optionally an instance.  FinishText, FinishProgram and
// (optionally) FinishInstance must be called in that order.
type Build struct {
	storage       Storage
	prog          programBuild
	inst          instanceBuild
	compileMem    []byte
	initRoutine   uint8
	textAddr      uint64
	stack         []byte
	stackUsage    int
	data          buffer.Static
	globalsSize   int
	memorySize    int
	maxMemorySize int
}

// NewBuild for a program and optionally an instance.
func NewBuild(storage Storage, moduleSize, maxTextSize int, objectMap *object.CallMap, instance bool,
) (b *Build, err error) {
	b = &Build{
		storage: storage,
		prog: programBuild{
			objectMap: objectMap,
		},
		inst: instanceBuild{
			enabled: instance,
		},
		initRoutine: abi.TextAddrStart,
	}

	b.prog.file, err = b.storage.newProgramFile()
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			b.munmapAll()
			b.prog.file.Close()
		}
	}()

	// Program text.
	err = mmapp(&b.compileMem, b.prog.file, progTextOffset, maxTextSize)
	if err != nil {
		return
	}

	b.prog.text = buffer.MakeStatic(b.compileMem[:0], maxTextSize)

	if moduleSize > 0 {
		// Program module.
		err = mmapp(&b.prog.moduleMem, b.prog.file, progModuleOffset, moduleSize)
		if err != nil {
			return
		}

		b.prog.module = buffer.MakeStatic(b.prog.moduleMem[:0], moduleSize)
	}

	return
}

// ModuleWriter is valid after NewBuild.  The module must be written before
// FinishProgram is called.
func (b *Build) ModuleWriter() io.Writer {
	return &b.prog.module
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
func (b *Build) FinishText(stackSize, stackUsage, globalsSize, memorySize, maxMemorySize int,
) (err error) {
	if stackSize < internal.StackLimitOffset+stackUsage {
		err = resourcelimit.New("call stack size limit exceeded")
		return
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
		err = mmapp(&b.compileMem, b.prog.file, progGlobalsOffset-int64(stackMapSize), mapSize)
		if err != nil {
			return
		}
	} else {
		b.inst.stackSize = alignPageSize(stackSize)

		b.inst.file, err = b.storage.newInstanceFile()
		if err != nil {
			return
		}
		defer func() {
			if err != nil {
				b.inst.file.Close()
			}
		}()

		// Instance stack, globals and memory contents.
		err = mmapp(&b.compileMem, b.inst.file, int64(b.inst.stackSize-stackMapSize), mapSize)
		if err != nil {
			return
		}
	}

	b.stack = b.compileMem[stackMapSize-stackUsage : stackMapSize]
	b.data = buffer.MakeStatic(b.compileMem[stackMapSize:stackMapSize], dataMapSize)
	b.globalsSize = globalsSize
	b.memorySize = memorySize
	b.maxMemorySize = maxMemorySize

	// Copy or write object map to program.
	var (
		progCallSitesOffset = progModuleOffset + align8(int64(b.prog.module.Cap()))
		progFuncAddrsOffset = progCallSitesOffset + int64(b.prog.callSitesSize())
		progObjectMapEnd    = alignPageOffset(progFuncAddrsOffset + int64(b.prog.funcAddrsSize()))
	)

	if progObjectMapEnd-progModuleOffset <= int64(len(b.prog.moduleMem)) {
		b.prog.copyObjectMapTo(b.prog.moduleMem[progCallSitesOffset-progModuleOffset:])
	} else {
		b.prog.writeObjectMapAt(progCallSitesOffset)
	}

	return
}

// ReadStack if FinishText has been called with nonzero stackUsage.  It must
// not be called after FinishProgram.
func (b *Build) ReadStack(r io.Reader, types []wa.FuncType, funcTypeIndexes []uint32,
) (err error) {
	_, err = io.ReadFull(r, b.stack)
	if err != nil {
		return
	}

	textAddr, err := generateRandTextAddr()
	if err != nil {
		return
	}

	err = importStack(b.stack, textAddr, *b.prog.objectMap, types, funcTypeIndexes)
	if err != nil {
		return
	}

	b.initRoutine = abi.TextAddrResume
	b.textAddr = textAddr
	b.stackUsage = len(b.stack)
	return
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
func (*Build) MemoryAlignment() int {
	return internal.PageSize
}

// FinishProgram after module, stack, globals and memory have been populated.
func (b *Build) FinishProgram(sectionMap SectionMap, globalTypes []wa.GlobalType, entryIndexes map[string]uint32, entryAddrs map[uint32]uint32,
) (prog *Program, err error) {
	if b.stackUsage != len(b.stack) {
		err = errors.New("stack was not populated")
		return
	}

	var (
		stackMapSize = alignPageSize(b.stackUsage)
		mapCopyLen   = stackMapSize + alignPageSize(b.data.Len())
	)

	if b.inst.enabled {
		var (
			off1 = int64(b.inst.stackSize - stackMapSize)
			off2 = progGlobalsOffset - int64(stackMapSize)
		)

		if b.storage.singleBackend() {
			// Copy stack, globals and memory from instance file to program file.
			err = copyFileRange(b.inst.file.Fd(), &off1, b.prog.file.Fd(), &off2, mapCopyLen)
		} else {
			// Write stack, globals and memory from instance mapping to program file.
			// TODO: trim range from beginning and end
			_, err = b.prog.file.WriteAt(b.compileMem[:mapCopyLen], off2)
		}
		if err != nil {
			return
		}
	}

	man := manifest.Program{
		InitRoutine:     int32(b.initRoutine),
		TextAddr:        b.textAddr,
		TextSize:        uint32(b.prog.textSize),
		StackUsage:      uint32(b.stackUsage),
		GlobalsSize:     uint32(b.globalsSize),
		MemoryDataSize:  uint32(b.data.Len() - alignPageSize(b.globalsSize)),
		MemorySize:      uint32(b.memorySize),
		MemorySizeLimit: uint32(b.maxMemorySize),
		ModuleSize:      int64(b.prog.module.Cap()),
		Sections:        sectionMap.manifestSections(),
		ServiceSection:  manifestByteRange(sectionMap.Service),
		IoSection:       manifestByteRange(sectionMap.IO),
		BufferSection:   manifestByteRange(sectionMap.Buffer),
		StackSection:    manifestByteRange(sectionMap.Stack),
		GlobalTypes:     globalTypeBytes(globalTypes),
		EntryIndexes:    entryIndexes,
		EntryAddrs:      entryAddrs,
		CallSitesSize:   uint32(b.prog.callSitesSize()),
		FuncAddrsSize:   uint32(b.prog.funcAddrsSize()),
	}

	b.stack = nil
	b.data = buffer.Static{}
	b.prog.module = buffer.Static{}

	var progMem []byte

	if !b.storage.singleBackend() {
		if b.inst.enabled {
			err = mmapp(&progMem, b.prog.file, progGlobalsOffset-int64(stackMapSize), mapCopyLen)
			if err != nil {
				return
			}
		} else {
			progMem = b.compileMem
			b.compileMem = nil
		}
	}

	b.munmapAll()

	prog = &Program{
		Map:     *b.prog.objectMap,
		storage: b.storage,
		man:     man,
		file:    b.prog.file,
		mem:     progMem,
	}
	b.prog.file = nil
	return
}

// FinishInstance after FinishProgram.  Applicable only if an instance storage
// was specified in NewBuild call.
func (b *Build) FinishInstance(entryIndex, entryAddr uint32) (inst *Instance, err error) {
	if entryAddr != 0 && b.stackUsage != 0 {
		err = notfound.ErrSuspended
		return
	}

	inst = &Instance{
		man: manifest.Instance{
			InitRoutine:   int32(b.initRoutine),
			TextAddr:      b.textAddr,
			StackSize:     uint32(b.inst.stackSize),
			StackUsage:    uint32(b.stackUsage),
			GlobalsSize:   uint32(b.globalsSize),
			MemorySize:    uint32(b.memorySize),
			MaxMemorySize: uint32(b.maxMemorySize),
			EntryIndex:    entryIndex,
			EntryAddr:     entryAddr,
		},
		file:     b.inst.file,
		coherent: true,
	}
	b.inst.file = nil
	return
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
func mmapp(ptr *[]byte, f *file.File, offset int64, length int) (err error) {
	if *ptr != nil {
		panic("memory already mapped")
	}

	if length == 0 {
		return
	}

	b, err := mmap(f.Fd(), offset, alignPageSize(length), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		return
	}

	*ptr = b
	return
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

	return *(*[]byte)(unsafe.Pointer(&reflect.SliceHeader{
		Len:  len(array),
		Cap:  cap(array),
		Data: (uintptr)(unsafe.Pointer(&array[0])),
	}))
}

func generateRandTextAddr() (textAddr uint64, err error) {
	b := make([]byte, 4)

	_, err = rand.Read(b)
	if err != nil {
		return
	}

	textAddr = internal.RandAddr(internal.MinTextAddr, internal.MaxTextAddr, b)
	return
}
