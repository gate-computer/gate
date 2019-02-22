// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"errors"
	"io"
	"reflect"
	"syscall"
	"unsafe"

	"github.com/tsavola/gate/internal/error/resourcelimit"
	internal "github.com/tsavola/gate/internal/executable"
	"github.com/tsavola/gate/internal/manifest"
	"github.com/tsavola/wag/buffer"
	"github.com/tsavola/wag/object"
	"github.com/tsavola/wag/object/abi"
	"github.com/tsavola/wag/object/stack"
	"github.com/tsavola/wag/wa"
)

const arcModuleOffset = manifest.MaxSize

type mappedFile struct {
	file *internal.FileRef
	mem  []byte
}

func (m *mappedFile) mmap(offset int64, length int) (err error) {
	if m.mem != nil {
		panic("memory already mapped")
	}

	b, err := syscall.Mmap(int(m.file.Fd()), offset, length, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		return
	}

	m.mem = b
	return
}

func (m *mappedFile) munmap() {
	b := m.mem
	m.mem = nil

	if err := syscall.Munmap(b); err != nil {
		panic(err)
	}
}

type archiveBuild struct {
	back LocalStorage
	mappedFile
	module buffer.Static

	objectMap          *object.CallMap
	text               buffer.Limited // Used only when there is no executable file.
	textDone           chan error
	stackSize          int
	stackDataThreshold int64
}

func (arc *archiveBuild) callSitesSize() int {
	return len(arc.objectMap.CallSites) * 8
}

func (arc *archiveBuild) copyCallSitesTo(dest []byte) {
	n := arc.callSitesSize()
	if n == 0 {
		return
	}

	copy(dest, *(*[]byte)(unsafe.Pointer(&reflect.SliceHeader{
		Len:  n,
		Cap:  n,
		Data: (uintptr)(unsafe.Pointer(&arc.objectMap.CallSites[0])),
	})))
}

func (arc *archiveBuild) funcAddrsSize() int {
	return len(arc.objectMap.FuncAddrs) * 4
}

func (arc *archiveBuild) copyFuncAddrsTo(dest []byte) {
	n := arc.funcAddrsSize()
	if n == 0 {
		return
	}

	copy(dest, *(*[]byte)(unsafe.Pointer(&reflect.SliceHeader{
		Len:  n,
		Cap:  n,
		Data: (uintptr)(unsafe.Pointer(&arc.objectMap.FuncAddrs[0])),
	})))
}

func (arc *archiveBuild) waitForText() error {
	if arc.textDone == nil {
		return nil
	}

	err, ok := <-arc.textDone
	arc.textDone = nil
	if !ok {
		return errors.New("panic during text archiving")
	}

	return err
}

type executableBuild struct {
	back BackingStore
	mappedFile
	text               buffer.Static
	stackDataThreshold int64
	entryIndex         uint32
	entryAddr          uint32
}

// Build a local archive and optionally an executable.
type Build struct {
	arc           archiveBuild
	exe           executableBuild
	textSize      int
	textAddr      uint64 // Set when importing stack, zero otherwise.
	stack         []byte
	stackUsage    int
	data          buffer.Static
	globalsSize   int
	memorySize    int
	maxMemorySize int
	initRoutine   int32
}

// NewBuild into local storage and optionally an executable reference.
// BackingStore instance must be the same one that was used to create the
// executable reference.
func NewBuild(arcBack LocalStorage, exeBack BackingStore, exeRef ExecutableRef, moduleSize, maxTextSize int, objectMap *object.CallMap,
) (build *Build, err error) {
	b := Build{
		arc: archiveBuild{
			back:      arcBack,
			objectMap: objectMap,
		},
		initRoutine: abi.TextAddrStart,
	}

	arcFile, err := arcBack.newArchiveFile()
	if err != nil {
		return
	}
	b.arc.file = internal.NewFileRef(arcFile)
	defer func() {
		if err != nil {
			b.arc.file.Close()
		}
	}()

	err = b.arc.file.Truncate(fileSize)
	if err != nil {
		return
	}

	// Module goes directly into archive file.

	var (
		arcModuleEnd = arcModuleOffset + moduleSize
	)

	err = b.arc.mmap(0, alignSize(arcModuleEnd))
	if err != nil {
		return
	}

	b.arc.module = buffer.MakeStatic(b.arc.mem[arcModuleOffset:arcModuleOffset], moduleSize)

	if exeRef == nil {
		// Text is buffered when there is no executable file.

		b.arc.text = buffer.MakeLimited(nil, maxTextSize)
	} else {
		b.exe = executableBuild{
			back: exeBack,
			mappedFile: mappedFile{
				file: exeRef.(*internal.FileRef), // Refcount increased at the end.
			},
		}

		err = b.exe.file.Truncate(fileSize)
		if err != nil {
			return
		}

		// Text goes directly into executable file.

		err = b.exe.mmap(0, alignSize(maxTextSize))
		if err != nil {
			return
		}

		b.exe.text = buffer.MakeStatic(b.exe.mem[:0], maxTextSize)

		b.exe.file.Ref()
	}

	build = &b
	return
}

func (b *Build) ModuleWriter() io.Writer {
	return &b.arc.module
}

func (b *Build) TextBuffer() interface {
	Bytes() []byte
	Extend(n int) []byte
	PutByte(byte)
	PutUint32(uint32) // Little-endian byte order.
} {
	if b.exe.file != nil {
		return &b.exe.text
	}
	return &b.arc.text
}

func (b *Build) FinishText(minStackSize, maxStackSize, globalsSize, memorySize, maxMemorySize int,
) (err error) {
	if b.exe.file != nil {
		b.textSize = b.exe.text.Len()
	} else {
		b.textSize = b.arc.text.Len()
	}

	// Archive file: unfinished module and space for object map, text, and
	// stack, globals and memory contents.
	var (
		arcCallSitesOffset = arcModuleOffset + b.arc.module.Cap()
		arcFuncAddrsOffset = arcCallSitesOffset + b.arc.callSitesSize()
		arcObjectMapEnd    = alignSize(arcFuncAddrsOffset + b.arc.funcAddrsSize())
		arcTextSize        = alignSize(b.textSize)
		arcTextOffset      = alignSize(arcObjectMapEnd)
		arcStackSize       = alignSize(internal.StackLimitOffset + minStackSize)
		arcStackOffset     = arcTextOffset + arcTextSize
		arcGlobalsSize     = alignSize(globalsSize)
		arcDataSize        = alignSize(arcGlobalsSize + memorySize)
		arcDataOffset      = arcStackOffset + arcStackSize
		arcDataEnd         = arcDataOffset + arcDataSize
	)

	if b.exe.file == nil {
		b.arc.munmap()

		err = b.arc.mmap(0, arcDataEnd)
		if err != nil {
			return
		}

		// Flush buffered text to archive file.

		b.arc.textDone = make(chan error, 1)

		go func(text []byte) { // TODO: profile with large input program
			defer close(b.arc.textDone)

			_, err := b.arc.file.WriteAt(text, int64(arcTextOffset))
			b.arc.textDone <- err
		}(b.arc.text.Bytes())

		b.arc.text = buffer.Limited{}
		b.stack = b.arc.mem[arcStackOffset : arcStackOffset+arcStackSize]
		b.data = buffer.MakeStatic(b.arc.mem[arcDataOffset:arcDataOffset], arcDataSize)
	} else {
		if len(b.arc.mem) < arcObjectMapEnd {
			b.arc.munmap()

			err = b.arc.mmap(0, arcObjectMapEnd)
			if err != nil {
				return
			}
		}

		// Executable file: text and space for stack, globals, and memory
		// allocations.
		var (
			exeTextSize      = alignSize(b.textSize)
			exeStackSize     = alignSize(maxStackSize)
			exeStackOffset   = int64(exeTextSize)
			exeGlobalsSize   = alignSize(globalsSize)
			exeGlobalsOffset = exeStackOffset + int64(exeStackSize)
			exeMemorySize    = alignSize(memorySize)
			exeMemoryOffset  = exeGlobalsOffset + int64(exeGlobalsSize)
			exeDataSize      = exeGlobalsSize + exeMemorySize
			exeDataEnd       = exeMemoryOffset + int64(exeMemorySize)
		)

		b.exe.munmap()

		err = b.exe.mmap(exeStackOffset, int(exeDataEnd-exeStackOffset))
		if err != nil {
			return
		}

		var (
			mapDataOffset = exeGlobalsOffset - exeStackOffset
		)

		b.exe.text = buffer.Static{}
		b.exe.stackDataThreshold = int64(exeStackOffset) + int64(exeStackSize)
		b.stack = b.exe.mem[:exeStackSize]
		b.data = buffer.MakeStatic(b.exe.mem[mapDataOffset:mapDataOffset], exeDataSize)

		// Copy text to archive file.

		b.arc.textDone = make(chan error, 1)

		go func() { // TODO: profile with large input program
			defer close(b.arc.textDone)

			off1 := int64(0)
			off2 := int64(arcTextOffset)
			b.arc.textDone <- copyFileRange(b.exe.file.Fd(), &off1, b.arc.file.Fd(), &off2, alignSize(b.textSize))
		}()
	}

	b.arc.module = buffer.MakeStatic(b.arc.mem[arcModuleOffset:arcModuleOffset+b.arc.module.Len()], b.arc.module.Cap())
	b.arc.stackSize = arcStackSize
	b.arc.stackDataThreshold = int64(arcStackOffset) + int64(arcStackSize)
	b.globalsSize = globalsSize
	b.memorySize = memorySize
	b.maxMemorySize = maxMemorySize

	// Copy object map to archive file.

	b.arc.copyCallSitesTo(b.arc.mem[arcCallSitesOffset:])
	b.arc.copyFuncAddrsTo(b.arc.mem[arcFuncAddrsOffset:])
	return
}

func (b *Build) SetupEntryStackFrame(entryIndex, entryAddr uint32) {
	size := stack.SetupEntryFrame(b.stack, entryAddr, nil)

	b.exe.entryIndex = entryIndex
	b.exe.entryAddr = entryAddr
	b.textAddr = 0
	b.stackUsage = size
	b.initRoutine = abi.TextAddrStart
}

func (b *Build) ReadSuspendedStack(r io.Reader, size int, types []wa.FuncType, funcTypeIndexes []uint32,
) (err error) {
	if size > len(b.stack)-internal.StackLimitOffset {
		err = resourcelimit.New("call stack size limit exceeded")
		return
	}
	buf := b.stack[len(b.stack)-size:]

	_, err = io.ReadFull(r, buf)
	if err != nil {
		return
	}

	textAddr, err := readRandTextAddr()
	if err != nil {
		return
	}

	err = importStack(buf, textAddr, *b.arc.objectMap, types, funcTypeIndexes)
	if err != nil {
		return
	}

	b.exe.entryIndex = 0
	b.exe.entryAddr = 0
	b.textAddr = textAddr
	b.stackUsage = size
	b.initRoutine = abi.TextAddrResume
	return
}

func (b *Build) GlobalsMemoryBuffer() interface {
	Bytes() []byte
	ResizeBytes(n int) []byte
} {
	return &b.data
}

func (*Build) MemoryAlignment() int {
	return internal.PageSize
}

// FinishArchiveExecutable returns an executable only if a nonzero executable
// reference was passed to NewBuild.
func (b *Build) FinishArchiveExecutable(arcKey string, sectionMap SectionMap, globalTypes []wa.GlobalType, entryIndexes map[string]uint32, entryAddrs map[uint32]uint32,
) (arc LocalArchive, exe *Executable, err error) {
	var (
		moduleSize   = b.arc.module.Cap()
		stackBufSize = len(b.stack)
		dataSize     = b.data.Len()
	)

	b.arc.module = buffer.Static{}
	b.stack = nil
	b.data = buffer.Static{}

	b.arc.munmap()

	var (
		arcStackUsage int
	)
	if b.arc.stackSize != 0 {
		arcStackUsage = b.stackUsage
	}

	if b.exe.file != nil {
		b.exe.munmap()

		// Copy stack, globals and memory contents to archive file.
		var (
			stackLen = alignSize(arcStackUsage)
			dataLen  = alignSize(dataSize)
		)

		off1 := b.exe.stackDataThreshold - int64(stackLen)
		off2 := b.arc.stackDataThreshold - int64(stackLen)
		err = copyFileRange(b.exe.file.Fd(), &off1, b.arc.file.Fd(), &off2, stackLen+dataLen)
		if err != nil {
			return
		}
	}

	err = b.arc.waitForText()
	if err != nil {
		return
	}

	arcMan := manifest.Archive{
		ModuleSize:    int64(moduleSize),
		Sections:      sectionMap.manifestSections(),
		StackSection:  manifestByteRange(sectionMap.Stack),
		GlobalTypes:   globalTypeBytes(globalTypes),
		EntryIndexes:  entryIndexes,
		EntryAddrs:    entryAddrs,
		CallSitesSize: uint32(b.arc.callSitesSize()),
		FuncAddrsSize: uint32(b.arc.funcAddrsSize()),
		Exe: manifest.Executable{
			TextAddr:        b.textAddr,
			TextSize:        uint32(b.textSize),
			StackSize:       uint32(b.arc.stackSize),
			StackUsage:      uint32(arcStackUsage),
			GlobalsSize:     uint32(b.globalsSize),
			MemoryDataSize:  uint32(dataSize - alignSize(b.globalsSize)),
			MemorySize:      uint32(b.memorySize),
			MemorySizeLimit: uint32(b.maxMemorySize),
			InitRoutine:     b.initRoutine,
		},
	}

	arc, err = b.arc.back.give(arcKey, arcMan, b.arc.file, *b.arc.objectMap)
	if err != nil {
		return
	}

	if b.exe.file != nil {
		exe = &Executable{
			Man:        arcMan.Exe,
			back:       b.exe.back,
			file:       b.exe.file,
			entryIndex: b.exe.entryIndex,
			entryAddr:  b.exe.entryAddr,
		}
		exe.Man.StackSize = uint32(stackBufSize)
		exe.Man.StackUsage = uint32(b.stackUsage)
		b.exe.file = nil
	}
	return
}

// FinishExecutable without an Archive.
func (b *Build) FinishExecutable() (exe *Executable, err error) {
	arc, exe, err := b.FinishArchiveExecutable("", SectionMap{}, nil, nil, nil)
	if err != nil {
		return
	}

	err = arc.Close()
	if err != nil {
		return
	}

	return
}

func (b *Build) Close() (err error) {
	setError := func(e error) {
		if err == nil {
			err = e
		}
	}

	setError(b.arc.waitForText())

	b.arc.module = buffer.Static{}
	b.arc.text = buffer.Limited{}
	b.exe.text = buffer.Static{}
	b.stack = nil
	b.data = buffer.Static{}

	if b.arc.file != nil {
		setError(b.arc.file.Close())
		b.arc.file = nil
	}
	if b.exe.file != nil {
		setError(b.exe.file.Close())
		b.exe.file = nil
	}

	if b.arc.mem != nil {
		b.arc.munmap()
	}
	if b.exe.mem != nil {
		b.exe.munmap()
	}
	return
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
