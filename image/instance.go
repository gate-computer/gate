// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"os"
	"syscall"

	"github.com/tsavola/gate/internal/error/notfound"
	"github.com/tsavola/gate/internal/error/resourcelimit"
	internal "github.com/tsavola/gate/internal/executable"
	"github.com/tsavola/gate/internal/file"
	"github.com/tsavola/gate/internal/manifest"
	"github.com/tsavola/wag/object/abi"
	"github.com/tsavola/wag/object/stack"
	"github.com/tsavola/wag/wa"
)

var ErrInvalidState = errors.New("instance state is invalid")

const (
	instMaxOffset = int64(0x180000000) // 0x80000000 * 3
)

const stackMagic = 0x7b53c485c17322fe

// stackVars is like stack_vars in runtime/loader/loader.c
type stackVars struct {
	StackUnused           uint32 // Other fields are meaningless if this is zero.
	CurrentMemoryPages    uint32 // WebAssembly pages.
	MonotonicTimeSnapshot uint64
	RandomAvail           int32
	_                     uint32 // Used by runtime.
	TextAddr              uint64
	Magic                 [4]uint64
}

type InstanceStorage interface {
	newInstanceFile() (*file.File, error)
	instanceFileWriteSupported() bool
	storeInstanceSupported() bool
	storeInstance(inst *Instance, name string) (manifest.Instance, error)
	LoadInstance(name string, man manifest.Instance) (*Instance, error)
	instanceBackend() interface{}
}

// Instance is a program state.  It may be undergoing mutation.
type Instance struct {
	man      manifest.Instance
	file     *file.File
	coherent bool
	dir      *file.File
	name     string
}

func NewInstance(prog *Program, maxStackSize int, entryIndex, entryAddr uint32,
) (inst *Instance, err error) {
	man := prog.Manifest()

	var (
		instStackSize  = alignPageSize(maxStackSize)
		instStackUsage int
		instTextAddr   uint64
	)

	if man.InitRoutine == abi.TextAddrResume {
		if entryAddr != 0 {
			err = notfound.ErrSuspended
			return
		}

		instStackUsage = int(man.StackUsage)
		instTextAddr = man.TextAddr
	}

	if instStackUsage > instStackSize-internal.StackUsageOffset {
		err = resourcelimit.New("call stack size limit exceeded")
		return
	}

	instFile, err := prog.storage.newInstanceFile()
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			instFile.Close()
		}
	}()

	var (
		stackMapSize   = alignPageSize(instStackUsage)
		globalsMapSize = alignPageSize32(man.GlobalsSize)
		memoryMapSize  = alignPageSize32(man.MemoryDataSize)
		copyLen        = stackMapSize + globalsMapSize + memoryMapSize

		off1 = progGlobalsOffset - int64(stackMapSize)
		off2 = int64(instStackSize - stackMapSize)
	)

	if copyLen > 0 {
		switch {
		case !prog.storage.instanceFileWriteSupported():
			var dest []byte

			// Copy stack, globals and memory from program mapping to temporary
			// instance mapping.
			dest, err = mmap(instFile.Fd(), off2, copyLen, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
			if err != nil {
				return
			}
			copy(dest, prog.mem[:copyLen])
			mustMunmap(dest)

		case prog.storage.singleBackend():
			// Copy stack, globals and memory from program file to instance
			// file.
			err = copyFileRange(prog.file.Fd(), &off1, instFile.Fd(), &off2, copyLen)
			if err != nil {
				return
			}

		default:
			// Write stack, globals and memory from program mapping to instance
			// file.
			// TODO: trim range from beginning and end
			_, err = instFile.WriteAt(prog.mem[:copyLen], off2)
			if err != nil {
				return
			}
		}
	}

	inst = &Instance{
		man: manifest.Instance{
			InitRoutine:   man.InitRoutine,
			TextAddr:      instTextAddr,
			StackSize:     uint32(instStackSize),
			StackUsage:    uint32(instStackUsage),
			GlobalsSize:   man.GlobalsSize,
			MemorySize:    man.MemorySize,
			MaxMemorySize: man.MemorySizeLimit,
			EntryIndex:    entryIndex,
			EntryAddr:     entryAddr,
			Snapshot:      man.Snapshot,
		},
		file:     instFile,
		coherent: true,
	}
	return
}

// Store the instance.  The name must not contain path separators.
func (inst *Instance) Store(name string, prog *Program) (man manifest.Instance, err error) {
	if !prog.storage.storeInstanceSupported() {
		// Zero manifest value represents nonexistent instance.
		return
	}

	err = inst.CheckMutation()
	if err != nil {
		return
	}

	man, err = prog.storage.storeInstance(inst, name)
	if err != nil {
		return
	}

	return
}

func (inst *Instance) Unstore() (err error) {
	if inst.name == "" {
		return
	}

	dir := inst.dir
	name := inst.name
	inst.dir = nil
	inst.name = ""

	err = unlinkat(dir.Fd(), name)
	if err != nil {
		if os.IsNotExist(err) {
			err = fdatasync(dir.Fd())
		}
		return
	}

	err = fdatasync(dir.Fd())
	if err != nil {
		return
	}

	return
}

func (inst *Instance) Close() (err error) {
	err = inst.file.Close()
	inst.file = nil
	return
}

func (inst *Instance) TextAddr() uint64      { return inst.man.TextAddr }
func (inst *Instance) StackSize() int        { return int(inst.man.StackSize) }
func (inst *Instance) StackUsage() int       { return int(inst.man.StackUsage) }
func (inst *Instance) GlobalsSize() int      { return alignPageSize32(inst.man.GlobalsSize) }
func (inst *Instance) MemorySize() int       { return int(inst.man.MemorySize) }
func (inst *Instance) MaxMemorySize() int    { return int(inst.man.MaxMemorySize) }
func (inst *Instance) InitRoutine() uint32   { return inst.man.InitRoutine }
func (inst *Instance) EntryAddr() uint32     { return inst.man.EntryAddr }
func (inst *Instance) MonotonicTime() uint64 { return inst.man.Snapshot.MonotonicTime }

// BeginMutation is invoked by a mutator when it takes exclusive ownership of
// the instance state.  CheckMutation and Close may be called during the
// mutation.  The returned file handle is valid until the next Instance method
// call.
func (inst *Instance) BeginMutation(textAddr uint64) (file interface{ Fd() uintptr }, err error) {
	if !inst.coherent {
		err = ErrInvalidState
		return
	}

	err = inst.Unstore()
	if err != nil {
		return
	}

	// Text address is currently unused, as it's later read from stack vars.

	inst.coherent = false
	file = inst.file
	return
}

// CheckMutation returns nil if the instance state is not undergoing mutation
// and the previous mutator (if any) has terminated cleanly.  ErrInvalidState
// is returned if the opposite is true.  Other errors mean that the check
// failed.
func (inst *Instance) CheckMutation() (err error) {
	if inst.coherent {
		return
	}

	b := make([]byte, internal.StackVarsSize)

	_, err = inst.file.ReadAt(b, 0)
	if err != nil {
		return
	}

	err = inst.checkMutation(b)
	if err != nil {
		return
	}

	return
}

func (inst *Instance) checkMutation(stack []byte) (err error) {
	if inst.coherent {
		return
	}

	vars, ok := checkStack(stack, inst.man.StackSize)
	if !ok {
		err = ErrInvalidState
		return
	}

	if vars.StackUnused == 0 {
		inst.man.InitRoutine = abi.TextAddrEnter
		inst.man.StackUsage = 0
		inst.man.TextAddr = 0
	} else {
		inst.man.Snapshot.MonotonicTime = vars.MonotonicTimeSnapshot
		inst.man.InitRoutine = abi.TextAddrResume
		inst.man.StackUsage = inst.man.StackSize - vars.StackUnused
		inst.man.TextAddr = vars.TextAddr
	}

	inst.man.MemorySize = vars.CurrentMemoryPages << wa.PageBits
	inst.coherent = true
	return
}

// ResetEntry prepares a halted instance for re-entry.
func (inst *Instance) ResetEntry(entryIndex, entryAddr uint32) {
	inst.man.EntryIndex = entryIndex
	inst.man.EntryAddr = entryAddr
	inst.man.InitRoutine = abi.TextAddrEnter
	inst.man.StackUsage = 0
}

func (inst *Instance) Globals(prog *Program) (values []uint64, err error) {
	var (
		instGlobalsEnd    = int64(inst.man.StackSize) + alignPageOffset32(inst.man.GlobalsSize)
		instGlobalsOffset = instGlobalsEnd - int64(inst.man.GlobalsSize)
	)

	b := make([]byte, inst.man.GlobalsSize)

	_, err = inst.file.ReadAt(b, instGlobalsOffset)
	if err != nil {
		return
	}

	values = make([]uint64, len(prog.man.GlobalTypes))

	for i := range values {
		values[i] = binary.LittleEndian.Uint64(b[len(b)-(i+1)*8:])
	}

	return
}

func (inst *Instance) Stacktrace(textMap stack.TextMap, funcSigs []wa.FuncType,
) (stacktrace []stack.Frame, err error) {
	b := make([]byte, inst.man.StackSize)

	_, err = inst.file.ReadAt(b, 0)
	if err != nil {
		return
	}

	err = inst.checkMutation(b)
	if err != nil {
		return
	}

	if inst.man.StackUsage == 0 {
		return
	}

	return stack.Trace(b[len(b)-int(inst.man.StackUsage):], inst.man.TextAddr, textMap, funcSigs)
}

func checkStack(b []byte, stackSize uint32) (vars stackVars, ok bool) {
	if binary.Read(bytes.NewReader(b), binary.LittleEndian, &vars) != nil {
		return
	}

	if vars.StackUnused == 0 {
		// Suspended before execution started.
		ok = true
		return
	}

	switch {
	case vars.StackUnused == math.MaxUint32: // Execution was suspended by force.
	case vars.StackUnused < internal.StackUsageOffset || vars.StackUnused > stackSize || vars.StackUnused&7 != 0:
	case vars.CurrentMemoryPages > math.MaxInt32/wa.PageSize:
	case vars.RandomAvail > 16:
	case !checkStackMagic(vars.Magic[:]):

	default:
		// All values seem legit.
		ok = true
		return
	}

	return
}

func checkStackMagic(numbers []uint64) bool {
	for _, n := range numbers {
		if n != stackMagic {
			return false
		}
	}
	return true
}

var pageMask = int64(internal.PageSize - 1)

func align8(n int64) int64             { return (n + 7) &^ 7 }
func alignPageSize(n int) int          { return int(alignPageOffset(int64(n))) }
func alignPageSize32(n uint32) int     { return int(alignPageOffset(int64(n))) }
func alignPageOffset(n int64) int64    { return (n + pageMask) &^ pageMask }
func alignPageOffset32(n uint32) int64 { return alignPageOffset(int64(n)) }
