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
	"github.com/tsavola/gate/snapshot"
	"github.com/tsavola/gate/trap"
	"github.com/tsavola/wag/object/abi"
	"github.com/tsavola/wag/object/stack"
	"github.com/tsavola/wag/wa"
)

var ErrInvalidState = errors.New("instance state is invalid")

const (
	instManifestOffset = int64(0x180000000)
	instMaxOffset      = int64(0x200000000)
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
	storeInstance(inst *Instance, name string) error
	LoadInstance(name string) (*Instance, error)
}

// Instance is a program state.  It may be undergoing mutation.
type Instance struct {
	man      manifest.Instance
	manDirty bool // Manifest needs to be written to file.
	coherent bool // File is not being mutated and looks okay.
	file     *file.File
	dir      *file.File // Non-nil means that store is supported and instance is stored.
	name     string     // Non-empty means that instance is in stored state.
}

func NewInstance(prog *Program, maxMemorySize, maxStackSize int, entryFuncIndex int,
) (inst *Instance, err error) {
	maxMemorySize, err = maxInstanceMemory(prog, maxMemorySize)
	if err != nil {
		return
	}

	var (
		instStackSize  = alignPageSize(maxStackSize)
		instStackUsage int
		instTextAddr   uint64
	)

	if prog.man.StackUsage != 0 {
		if entryFuncIndex >= 0 {
			err = notfound.ErrSuspended
			return
		}

		instStackUsage = int(prog.man.StackUsage)
		instTextAddr = prog.man.TextAddr
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

	// Copy stack, globals and memory from program file to instance file.
	var (
		stackMapSize   = alignPageSize(instStackUsage)
		globalsMapSize = alignPageSize32(prog.man.GlobalsSize)
		memoryMapSize  = alignPageSize32(prog.man.MemoryDataSize)
		off1           = progGlobalsOffset - int64(stackMapSize)
		off2           = int64(instStackSize - stackMapSize)
		copyLen        = stackMapSize + globalsMapSize + memoryMapSize
	)
	if copyLen > 0 {
		if prog.storage.instanceFileWriteSupported() {
			err = copyFileRange(prog.file, &off1, instFile, &off2, copyLen)
			if err != nil {
				return
			}
		} else {
			var dest []byte

			dest, err = mmap(instFile.Fd(), off2, copyLen, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
			if err != nil {
				return
			}
			defer mustMunmap(dest)

			_, err = prog.file.ReadAt(dest[:copyLen], off1)
			if err != nil {
				return
			}
		}
	}

	inst = &Instance{
		man: manifest.Instance{
			TextAddr:      instTextAddr,
			StackSize:     uint32(instStackSize),
			StackUsage:    uint32(instStackUsage),
			GlobalsSize:   prog.man.GlobalsSize,
			MemorySize:    prog.man.MemorySize,
			MaxMemorySize: uint32(maxMemorySize),
			StartFunc:     prog.man.StartFunc,
			EntryFunc:     prog.man.EntryFunc(entryFuncIndex),
			Snapshot:      prog.man.Snapshot,
		},
		manDirty: true,
		coherent: true,
		file:     instFile,
	}
	return
}

// Store the instance.  The name must not contain path separators.
func (inst *Instance) Store(name string, prog *Program) (err error) {
	if inst.name != "" {
		err = errors.New("instance already stored")
		return
	}

	if !prog.storage.storeInstanceSupported() {
		inst.name = name
		return
	}

	err = inst.CheckMutation()
	if err != nil {
		return
	}

	err = prog.storage.storeInstance(inst, name)
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

	if dir == nil {
		return
	}

	err = unlinkat(dir.Fd(), name)
	if err != nil {
		if os.IsNotExist(err) {
			err = fdatasync(dir.Fd())
		}
		return
	}

	err = fdatasync(dir.Fd())
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
func (inst *Instance) StartAddr() uint32     { return inst.man.StartFunc.Addr }
func (inst *Instance) EntryAddr() uint32     { return inst.man.EntryFunc.Addr }
func (inst *Instance) Flags() snapshot.Flags { return snapshot.Flags(inst.man.Snapshot.Flags) }
func (inst *Instance) Final() bool           { return inst.Flags().Final() }
func (inst *Instance) DebugInfo() bool       { return inst.Flags().DebugInfo() }
func (inst *Instance) Trap() trap.ID         { return trap.ID(inst.man.Snapshot.Trap) }
func (inst *Instance) Result() int32         { return inst.man.Snapshot.Result }
func (inst *Instance) MonotonicTime() uint64 { return inst.man.Snapshot.MonotonicTime }

// Breakpoints are in ascending order and unique.
func (inst *Instance) Breakpoints() []uint64 {
	return inst.man.Snapshot.Breakpoints.Offsets
}

func (inst *Instance) SetFinal() {
	flags := uint64(inst.Flags() | snapshot.FlagFinal)
	if inst.man.Snapshot.Flags == flags {
		return
	}
	inst.man.Snapshot.Flags = flags
	inst.manDirty = true
}

func (inst *Instance) SetDebugInfo(enabled bool) {
	var flags uint64
	if enabled {
		flags = uint64(inst.Flags() | snapshot.FlagDebugInfo)
	} else {
		flags = uint64(inst.Flags() &^ snapshot.FlagDebugInfo)
	}
	if inst.man.Snapshot.Flags == flags {
		return
	}
	inst.man.Snapshot.Flags = flags
	inst.manDirty = true
}

func (inst *Instance) SetTrap(id trap.ID) {
	if inst.man.Snapshot.Trap == int32(id) {
		return
	}
	inst.man.Snapshot.Trap = int32(id)
	inst.manDirty = true
}

func (inst *Instance) SetResult(n int32) {
	if inst.man.Snapshot.Result == n {
		return
	}
	inst.man.Snapshot.Result = n
	inst.manDirty = true
}

// SetBreakpoints which must have been sorted and deduplicated.
func (inst *Instance) SetBreakpoints(a []uint64) {
	if len(inst.man.Snapshot.Breakpoints.Offsets) == len(a) {
		for i, x := range inst.man.Snapshot.Breakpoints.Offsets {
			if x != a[i] {
				goto changed
			}
		}
		return
	}
changed:
	inst.man.Snapshot.Breakpoints.Offsets = a
	inst.manDirty = true
}

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

	return inst.checkMutation(b)
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

	if vars.StackUnused != 0 {
		if vars.StackUnused == inst.man.StackSize {
			inst.man.TextAddr = 0
			inst.man.StackUsage = 0
			inst.man.InitRoutine = abi.TextAddrEnter
		} else {
			inst.man.TextAddr = vars.TextAddr
			inst.man.StackUsage = inst.man.StackSize - vars.StackUnused
			inst.man.InitRoutine = abi.TextAddrResume
		}
		inst.man.MemorySize = vars.CurrentMemoryPages << wa.PageBits
		inst.man.StartFunc = manifest.NoFunction
		inst.man.EntryFunc = manifest.NoFunction
		inst.man.Snapshot.MonotonicTime = vars.MonotonicTimeSnapshot
		inst.manDirty = true
	}

	inst.coherent = true
	return
}

// SetEntry prepares a halted instance for re-entry.
func (inst *Instance) SetEntry(prog *Program, funcIndex int) {
	f := prog.man.EntryFunc(funcIndex)
	if inst.man.EntryFunc == f {
		return
	}
	inst.man.EntryFunc = f
	inst.manDirty = true
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

func (inst *Instance) readStack() (stack []byte, err error) {
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

	stack = b[len(b)-int(inst.man.StackUsage):]
	return
}

func (inst *Instance) ExportStack(textMap stack.TextMap) (stack []byte, err error) {
	b, err := inst.readStack()
	if err != nil || len(b) == 0 {
		return
	}

	err = exportStack(b, b, inst.man.TextAddr, textMap)
	if err != nil {
		return
	}

	stack = b
	return
}

func (inst *Instance) Stacktrace(textMap stack.TextMap, funcTypes []wa.FuncType,
) (stacktrace []stack.Frame, err error) {
	b, err := inst.readStack()
	if err != nil || len(b) == 0 {
		return
	}

	return stack.Trace(b, inst.man.TextAddr, textMap, funcTypes)
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

func maxInstanceMemory(prog *Program, n int) (adjusted int, err error) {
	if prog.man.MemorySizeLimit >= 0 && n > int(prog.man.MemorySizeLimit) {
		n = int(prog.man.MemorySizeLimit)
	}
	if n >= 0 && n < int(prog.man.MemorySize) {
		return n, resourcelimit.New("out of memory")
	}
	return n, nil
}

var pageMask = int64(internal.PageSize - 1)

func align8(n int64) int64             { return (n + 7) &^ 7 }
func alignPageSize(n int) int          { return int(alignPageOffset(int64(n))) }
func alignPageSize32(n uint32) int     { return int(alignPageOffset(int64(n))) }
func alignPageOffset(n int64) int64    { return (n + pageMask) &^ pageMask }
func alignPageOffset32(n uint32) int64 { return alignPageOffset(int64(n)) }
