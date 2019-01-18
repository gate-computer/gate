// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"os"
	"syscall"

	"github.com/tsavola/gate/internal/error/resourcelimit"
	internal "github.com/tsavola/gate/internal/executable"
	"github.com/tsavola/wag/buffer"
	"github.com/tsavola/wag/compile"
	"github.com/tsavola/wag/object/stack"
	"github.com/tsavola/wag/section"
	"github.com/tsavola/wag/wa"
)

var ErrBadTermination = errors.New("execution has not terminated cleanly")

// Config values must be specified explicitly; they default to zero.
type Config struct {
	MaxTextSize   int
	StackSize     int
	MaxMemorySize int
}

type Metadata struct {
	MemorySizeLimit int
	GlobalTypes     []wa.GlobalType
	SectionRanges   []section.ByteRange
	EntryAddrs      map[string]uint32
}

type BackingStore interface {
	getPageSize() int
	newExecutableFile() (*os.File, error)
	sealFile(*os.File) error
}

type ExecutableRef = internal.Ref

func NewExecutableRef(back BackingStore) (ref ExecutableRef, err error) {
	f, err := back.newExecutableFile()
	if err != nil {
		return
	}

	ref = internal.NewFileRef(f, back)
	return
}

// Executable is a stateful program representation.
type Executable struct {
	file     *internal.FileRef
	manifest internal.Manifest
	dataSize int
}

// LoadExecutable from storage.  It will be attached to the specified,
// executable reference.
func LoadExecutable(ctx context.Context, ref ExecutableRef, config *Config, ar Archive, stackFrame []byte,
) (exe *Executable, err error) {
	file := ref.(*internal.FileRef)
	back := file.Back.(BackingStore)
	pageSize := back.getPageSize()
	manifest := ar.Manifest()

	// Check that stored executable fits within configured limits.

	textFileSize := roundSize(manifest.TextSize, pageSize)
	if textFileSize > config.MaxTextSize {
		err = resourcelimit.New("program code size limit exceeded")
		return
	}

	stackSize := roundSize(config.StackSize, pageSize)
	if len(stackFrame) > config.StackSize {
		err = resourcelimit.New("call stack size limit exceeded")
		return
	}

	maxMemorySize := manifest.Metadata.MemorySizeLimit
	if maxMemorySize > config.MaxMemorySize {
		maxMemorySize = config.MaxMemorySize
	}
	if manifest.MemorySize > maxMemorySize {
		err = resourcelimit.New("linear memory size limit exceeded")
		return
	}

	// Storage dimensions
	var (
		dataStorageSize = manifest.GlobalsSize + manifest.MemorySize
	)

	// File dimensions
	var (
		stackFileEnd    = textFileSize + stackSize
		globalsFileSize = roundSize(manifest.GlobalsSize, pageSize)
		globalsFileEnd  = stackFileEnd + globalsFileSize
		dataFileOffset  = int64(globalsFileEnd - manifest.GlobalsSize)
	)

	load, err := ar.Open(ctx)
	if err != nil {
		return
	}
	defer load.Close()

	// File size must accommodate all runtime memory allocations.

	err = file.Truncate(int64(globalsFileEnd + maxMemorySize))
	if err != nil {
		return
	}

	// Part 1: Copy text at start of file, possibly using copy_file_range.  If
	// archive has the same backing store, and it supports reflinks, it might
	// not need to copy data.

	err = load.Text.copyToFile(file.File, 0, manifest.TextSize)
	if err != nil {
		return
	}

	// Part 2: Initialize stack which follows text.

	_, err = file.WriteAt(stackFrame, int64(stackFileEnd-len(stackFrame)))
	if err != nil {
		return
	}

	// Part 3: Copy globals and initial memory contents after stack, possibly
	// using copy_file_range.  The reflink note applies here aswell; the data
	// would be copied-on-write during execution.

	err = load.GlobalsMemory.copyToFile(file.File, dataFileOffset, dataStorageSize)
	if err != nil {
		return
	}

	// Apply backing store specific protections, if there are any.

	err = back.sealFile(file.File)
	if err != nil {
		return
	}

	exe = &Executable{
		file: file.Ref(),
		manifest: internal.Manifest{
			TextSize:      textFileSize,
			StackSize:     stackSize,
			StackUnused:   stackSize - len(stackFrame),
			GlobalsSize:   globalsFileSize,
			MemorySize:    manifest.MemorySize,
			MaxMemorySize: maxMemorySize,
		},
		dataSize: dataStorageSize,
	}
	return
}

func (exe *Executable) Close() error          { return exe.file.Close() }
func (exe *Executable) Ref() ExecutableRef    { return exe.file.Ref() }
func (exe *Executable) Manifest() interface{} { return &exe.manifest }

func (exe *Executable) StoreThis(ctx context.Context, key string, metadata *Metadata, storage ArchiveStorage) (ar Archive, err error) {
	if storage, ok := storage.(internalArchiveStorage); ok {
		ar, err = storage.archive(key, metadata, &exe.manifest, exe.file)
		if ar != nil || err != nil {
			return
		}
	}

	return exe.StoreCopy(ctx, key, metadata, storage)
}

func (exe *Executable) StoreCopy(ctx context.Context, key string, metadata *Metadata, storage ArchiveStorage) (ar Archive, err error) {
	var (
		textOffset = int64(0)
		dataOffset = int64(exe.manifest.TextSize + exe.manifest.StackSize)
		dataSize   = exe.manifest.GlobalsSize + exe.manifest.MemorySize
	)

	manifest := &ArchiveManifest{
		TextSize:    exe.manifest.TextSize,
		GlobalsSize: exe.manifest.GlobalsSize,
		MemorySize:  exe.manifest.MemorySize,
		Metadata:    *metadata,
	}

	store, err := storage.CreateArchive(ctx, manifest)
	if err != nil {
		return
	}
	defer store.Close()

	if w, ok := store.Text.(descriptorFile); ok {
		err = copyFileRange(exe.file.Fd(), &textOffset, w.Fd(), nil, exe.manifest.TextSize)
	} else {
		r := &randomAccessReader{exe.file.File, textOffset}
		_, err = io.CopyN(store.Text, r, int64(exe.manifest.TextSize))
	}
	if err != nil {
		return
	}

	if w, ok := store.GlobalsMemory.(descriptorFile); ok {
		err = copyFileRange(exe.file.Fd(), &dataOffset, w.Fd(), nil, dataSize)
	} else {
		r := &randomAccessReader{exe.file.File, dataOffset}
		_, err = io.CopyN(store.GlobalsMemory, r, int64(dataSize))
	}
	if err != nil {
		return
	}

	return store.Archive(key)
}

// CheckTermination returns nil if the termination appears to have been
// orderly, and ErrBadTermination if not.  Other errors mean that the check was
// unsuccessful.
func (exe *Executable) CheckTermination() (err error) {
	b := make([]byte, 16)

	_, err = exe.file.ReadAt(b, int64(exe.manifest.TextSize))
	if err != nil {
		return
	}

	if _, _, _, ok := checkStack(b, exe.manifest.StackSize); !ok {
		err = ErrBadTermination
		return
	}

	return
}

func (exe *Executable) Stacktrace(textMap stack.TextMap, funcSigs []wa.FuncType) (stacktrace []stack.Frame, err error) {
	b := make([]byte, exe.manifest.StackSize)

	_, err = exe.file.ReadAt(b, int64(exe.manifest.TextSize))
	if err != nil {
		return
	}

	unused, _, textAddr, ok := checkStack(b, len(b))
	if !ok {
		err = ErrBadTermination
		return
	}

	return stack.Trace(b[unused:], textAddr, textMap, funcSigs)
}

type BuildConfig struct {
	Config
	GlobalsSize int
	MemorySize  int
}

// Build target buffers.
type Build struct {
	exe           *Executable
	mapping       []byte
	text          buffer.Static
	stack         []byte
	stackUnused   int
	globalsMemory buffer.Static
	memoryOffset  int
	maxMemorySize int
}

func NewBuild(ref ExecutableRef) *Build {
	return &Build{
		exe: &Executable{
			file: ref.(*internal.FileRef).Ref(),
		},
	}
}

func (b *Build) Ref() ExecutableRef {
	return b.exe.Ref()
}

// Configure or reconfigure buffer sizes.  Supported reconfigurations:
//
// - Resize text or stack before writing stack, globals and memory.
// - Resize globals and memory at any stage.
//
func (b *Build) Configure(config *BuildConfig) (err error) {
	pageSize := b.exe.file.Back.(BackingStore).getPageSize()

	// Mapping dimensions
	var (
		textSize       = roundSize(config.MaxTextSize, pageSize)
		stackOffset    = textSize
		stackSize      = roundSize(config.StackSize, pageSize)
		stackEnd       = stackOffset + stackSize
		globalsOffset  = stackEnd
		globalsMapSize = roundSize(config.GlobalsSize, pageSize)
		globalsEnd     = globalsOffset + globalsMapSize
		memoryOffset   = globalsEnd
		dataEnd        = memoryOffset + config.MemorySize
		fileEnd        = memoryOffset + config.MaxMemorySize
	)

	err = b.exe.file.Truncate(int64(fileEnd))
	if err != nil {
		return
	}

	if len(b.mapping) < dataEnd {
		err = b.unmap()
		if err != nil {
			return
		}
	}

	if b.mapping == nil {
		b.mapping, err = syscall.Mmap(int(b.exe.file.Fd()), 0, dataEnd, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
		if err != nil {
			return
		}
	}

	b.text = buffer.MakeStatic(b.mapping[:0], textSize)
	b.stack = b.mapping[stackOffset:stackEnd]
	b.stackUnused = len(b.stack)
	b.globalsMemory = buffer.MakeStatic(b.mapping[globalsOffset:globalsOffset], dataEnd-globalsOffset)
	b.memoryOffset = globalsMapSize
	b.maxMemorySize = config.MaxMemorySize
	return
}

func (b *Build) TextBuffer() compile.CodeBuffer {
	return &b.text
}

func (b *Build) TextSize() int {
	return len(b.text.Bytes())
}

func (b *Build) SetupEntryStackFrame(entryFuncAddr uint32) {
	frameSize := stack.SetupEntryFrame(b.stack, entryFuncAddr, nil)
	b.stackUnused = len(b.stack) - frameSize
}

func (b *Build) GlobalsMemoryBuffer() compile.DataBuffer {
	return &b.globalsMemory
}

func (b *Build) Executable() (exe *Executable, err error) {
	b.exe.manifest = internal.Manifest{
		TextSize:      b.text.Cap(),
		StackSize:     len(b.stack),
		StackUnused:   b.stackUnused,
		GlobalsSize:   b.memoryOffset,
		MemorySize:    b.globalsMemory.Cap() - b.memoryOffset,
		MaxMemorySize: b.maxMemorySize,
	}
	b.exe.dataSize = len(b.globalsMemory.Bytes())

	err = b.unmap()
	if err != nil {
		return
	}

	err = b.exe.file.Back.(BackingStore).sealFile(b.exe.file.File)
	if err != nil {
		return
	}

	exe = b.exe
	b.exe = nil
	return
}

func (b *Build) Close() error {
	err := b.unmap()

	if b.exe != nil {
		if err := b.exe.Close(); err != nil {
			return err
		}
	}

	return err
}

func (b *Build) unmap() (err error) {
	if b.mapping != nil {
		err = syscall.Munmap(b.mapping)
		b.mapping = nil
		b.text = buffer.Static{}
		b.stack = nil
		b.globalsMemory = buffer.Static{}
	}
	return
}

func checkStack(b []byte, stackSize int) (unused, memorySize uint32, textAddr uint64, ok bool) {
	if len(b) < 16 {
		return
	}

	memoryPages := binary.LittleEndian.Uint32(b[0:])
	memorySize = memoryPages * wa.PageSize
	unused = binary.LittleEndian.Uint32(b[4:])
	textAddr = binary.LittleEndian.Uint64(b[8:])

	ok = memoryPages <= math.MaxInt32/wa.PageSize && unused > 0 && unused < uint32(stackSize) && unused&7 == 0 && textAddr >= internal.MinTextAddr && textAddr <= internal.MaxTextAddr && textAddr&uint64(memPageSize-1) == 0
	return
}

func roundSize(n, pageSize int) int {
	mask := pageSize - 1
	return (n + mask) &^ mask
}
