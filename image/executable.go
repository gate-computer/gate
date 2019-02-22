// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"math"
	"os"

	"github.com/tsavola/gate/internal/error/notfound"
	"github.com/tsavola/gate/internal/error/resourcelimit"
	internal "github.com/tsavola/gate/internal/executable"
	"github.com/tsavola/gate/internal/file"
	"github.com/tsavola/gate/internal/manifest"
	"github.com/tsavola/wag/object/abi"
	"github.com/tsavola/wag/object/stack"
	"github.com/tsavola/wag/wa"
)

const fileSize = 0x1000000000 // 64 GB

var ErrBadTermination = errors.New("execution has not terminated cleanly")

type BackingStore interface {
	newExecutableFile() (*os.File, error)
}

type ExecutableRef = file.OpaqueRef

func NewExecutableRef(back BackingStore) (ref ExecutableRef, err error) {
	f, err := back.newExecutableFile()
	if err != nil {
		return
	}

	ref = file.NewRef(f)
	return
}

// Executable is a stateful program representation.
type Executable struct {
	Man manifest.Executable

	back       BackingStore
	file       *file.Ref
	entryIndex uint32
	entryAddr  uint32
}

// NewExecutable from local storage.  It will be attached to the specified
// executable reference.  BackingStore instance must be the same one that was
// used to create the executable reference.
func NewExecutable(refBack BackingStore, ref ExecutableRef, arc LocalArchive, maxStackSize int, entryIndex, entryAddr uint32,
) (exe *Executable, err error) {
	var (
		arcMan = arc.Manifest()
	)

	// Program state.
	var (
		exeStackSize  = alignSize(maxStackSize)
		exeStackUsage int
		exeStackFrame []byte
		exeTextAddr   uint64
	)

	if arcMan.Exe.InitRoutine == abi.TextAddrResume {
		if entryAddr != 0 {
			// Suspended program has no explicit entry points.  TODO: detailed error
			err = notfound.ErrFunction
			return
		}

		exeStackUsage = int(arcMan.Exe.StackUsage)
		exeTextAddr = arcMan.Exe.TextAddr
	} else {
		exeStackFrame = stack.EntryFrame(entryAddr, nil)
		exeStackUsage = len(exeStackFrame)
	}

	if exeStackUsage > exeStackSize-internal.StackLimitOffset {
		err = resourcelimit.New("call stack size limit exceeded")
		return
	}

	// Target file.
	var (
		exeFile          = ref.(*file.Ref)
		exeTextOffset    = int64(0)
		exeStackOffset   = exeTextOffset + alignSize64(int64(arcMan.Exe.TextSize))
		exeGlobalsOffset = exeStackOffset + int64(exeStackSize)
		exeMemoryOffset  = exeGlobalsOffset + alignSize64(int64(arcMan.Exe.GlobalsSize))
	)

	err = exeFile.Truncate(fileSize)
	if err != nil {
		return
	}

	// Source file.
	var (
		arcFile            = arc.file()
		arcCallSitesOffset = arcModuleOffset + int(arcMan.ModuleSize)
		arcFuncAddrsOffset = arcCallSitesOffset + int(arcMan.CallSitesSize)
		arcTextOffset      = alignSize64(int64(arcFuncAddrsOffset) + int64(arcMan.FuncAddrsSize))
		arcStackOffset     = arcTextOffset + alignSize64(int64(arcMan.Exe.TextSize))
		arcGlobalsOffset   = arcStackOffset + alignSize64(int64(arcMan.Exe.StackSize))
		arcMemoryOffset    = arcGlobalsOffset + alignSize64(int64(arcMan.Exe.GlobalsSize))
	)

	// Copy text.
	off1 := arcTextOffset
	off2 := exeTextOffset
	err = copyFileRange(arcFile.Fd(), &off1, exeFile.Fd(), &off2, alignSize(int(arcMan.Exe.TextSize)))
	if err != nil {
		return
	}

	if exeStackFrame != nil {
		// Write stack.
		off := exeGlobalsOffset - int64(len(exeStackFrame))
		_, err = exeFile.WriteAt(exeStackFrame, off)
		if err != nil {
			return
		}
	} else {
		// Copy stack.
		var (
			stackLen = alignSize(exeStackUsage)
		)

		off1 = arcGlobalsOffset - int64(stackLen)
		off2 = exeGlobalsOffset - int64(stackLen)
		err = copyFileRange(arcFile.Fd(), &off1, exeFile.Fd(), &off2, stackLen)
		if err != nil {
			return
		}
	}

	// TODO: merge stack copy and data copy

	// Copy globals and memory.
	var (
		globalsLen = alignSize(int(arcMan.Exe.GlobalsSize))
		memoryLen  = alignSize(int(arcMan.Exe.MemoryDataSize))
	)

	off1 = arcMemoryOffset - int64(globalsLen)
	off2 = exeMemoryOffset - int64(globalsLen)
	err = copyFileRange(arcFile.Fd(), &off1, exeFile.Fd(), &off2, globalsLen+memoryLen)
	if err != nil {
		return
	}

	// Success; increment reference count.
	exe = &Executable{
		Man:        arcMan.Exe,
		back:       refBack,
		file:       exeFile.Ref(),
		entryIndex: entryIndex,
		entryAddr:  entryAddr,
	}
	exe.Man.TextAddr = exeTextAddr
	exe.Man.StackSize = uint32(exeStackSize)
	exe.Man.StackUsage = uint32(exeStackUsage)
	return
}

func (exe *Executable) Ref() ExecutableRef {
	return exe.file.Ref()
}

func (exe *Executable) Close() (err error) {
	err = exe.file.Close()
	exe.file = nil
	return
}

func (exe *Executable) PageSize() uint32        { return uint32(internal.PageSize) }
func (exe *Executable) TextAddr() uint64        { return exe.Man.TextAddr }
func (exe *Executable) SetTextAddr(addr uint64) { exe.Man.TextAddr = addr }
func (exe *Executable) TextSize() uint32        { return alignSize32(exe.Man.TextSize) }
func (exe *Executable) StackSize() uint32       { return uint32(exe.Man.StackSize) }
func (exe *Executable) StackUsage() uint32      { return uint32(exe.Man.StackUsage) }
func (exe *Executable) GlobalsSize() uint32     { return alignSize32(exe.Man.GlobalsSize) }
func (exe *Executable) MemorySize() uint32      { return uint32(exe.Man.MemorySize) }
func (exe *Executable) MaxMemorySize() uint32   { return uint32(exe.Man.MemorySizeLimit) }
func (exe *Executable) InitRoutine() uint16     { return uint16(exe.Man.InitRoutine) }

// CheckTermination returns nil error if the termination appears to have been
// orderly, and ErrBadTermination if not.  Other errors mean that the check was
// unsuccessful.
func (exe *Executable) CheckTermination() (err error) {
	b := make([]byte, 8)

	_, err = exe.file.ReadAt(b, alignSize64(int64(exe.Man.TextSize)))
	if err != nil {
		return
	}

	if _, _, ok := checkStack(b, int(exe.Man.StackSize)); !ok {
		err = ErrBadTermination
		return
	}

	return
}

func (exe *Executable) Stacktrace(textMap stack.TextMap, funcSigs []wa.FuncType,
) (stacktrace []stack.Frame, err error) {
	b := make([]byte, exe.Man.StackSize)

	_, err = exe.file.ReadAt(b, alignSize64(int64(exe.Man.TextSize)))
	if err != nil {
		return
	}

	unused, _, ok := checkStack(b, len(b))
	if !ok {
		err = ErrBadTermination
		return
	}

	if unused != 0 && int(unused) != len(b) {
		stacktrace, err = stack.Trace(b[unused:], exe.Man.TextAddr, textMap, funcSigs)
	}
	return
}

func checkStack(b []byte, stackSize int) (unused, memorySize uint32, ok bool) {
	if len(b) < 8 {
		return
	}

	memoryPages := binary.LittleEndian.Uint32(b[0:])
	memorySize = memoryPages * wa.PageSize
	unused = binary.LittleEndian.Uint32(b[4:])

	switch {
	case memoryPages > math.MaxInt32/wa.PageSize:
		// Impossible memory state.
		return

	case unused == 0:
		// Suspended before execution started.
		ok = true
		return

	case unused == math.MaxUint32:
		// Execution was suspended by force.
		return

	case unused < internal.StackLimitOffset || unused > uint32(stackSize) || unused&7 != 0:
		// Impossible stack state.
		return

	default:
		ok = true
		return
	}
}

func readRandTextAddr() (textAddr uint64, err error) {
	b := make([]byte, 4)

	_, err = rand.Read(b)
	if err != nil {
		return
	}

	textAddr = internal.RandAddr(internal.MinTextAddr, internal.MaxTextAddr, b)
	return
}

var pageMask = int64(internal.PageSize - 1)

func alignSize64(size int64) int64 {
	return (size + pageMask) &^ pageMask
}

func alignSize32(size uint32) uint32 {
	return uint32(alignSize64(int64(size)))
}

func alignSize(size int) int {
	return int(alignSize64(int64(size)))
}
