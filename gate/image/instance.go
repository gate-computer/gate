// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"runtime"
	"syscall"

	"gate.computer/gate/snapshot"
	"gate.computer/gate/trap"
	"gate.computer/internal/error/notfound"
	"gate.computer/internal/error/resourcelimit"
	"gate.computer/internal/executable"
	"gate.computer/internal/file"
	pb "gate.computer/internal/pb/image"
	"gate.computer/wag/object/abi"
	"gate.computer/wag/object/stack"
	"gate.computer/wag/wa"
)

var ErrInvalidState = errors.New("instance state is invalid")

const (
	instStackOffset    = int64(0)
	instManifestOffset = int64(0x180000000)
	instMaxOffset      = int64(0x200000000)
)

const stackMagic = 0x7b53c485c17322fe

// stackVars is like StackVars in runtime/loader/loader.cpp
type stackVars struct {
	StackUnused           uint32 // Other fields are meaningless if this is zero.
	CurrentMemoryPages    uint32 // WebAssembly pages.
	MonotonicTimeSnapshot uint64
	RandomAvail           int32
	_                     uint32 // Used by runtime.
	TextAddr              uint64
	Result                [2]uint64 // Index is a wa.ScalarCategory.
	Magic                 [2]uint64
}

type InstanceStorage interface {
	Instances() (names []string, err error)
	LoadInstance(name string) (*Instance, error)

	newInstanceFile() (*file.File, error)
	instanceFileWriteSupported() bool
	storeInstanceSupported() bool
	storeInstance(inst *Instance, name string) error
}

// Instance is a program state.  It may be undergoing mutation.
type Instance struct {
	man      *pb.InstanceManifest
	manDirty bool // Manifest needs to be written to file.
	coherent bool // File is not being mutated and looks okay.
	file     *file.File
	dir      *file.File // Non-nil means that store is supported and instance is stored.
	name     string     // Non-empty means that instance is in stored state.
}

func NewInstance(prog *Program, maxMemorySize, maxStackSize, entryFuncIndex int) (*Instance, error) {
	maxMemorySize, err := maxInstanceMemory(prog, maxMemorySize)
	if err != nil {
		return nil, err
	}

	var (
		instStackSize  = alignPageSize(maxStackSize)
		instStackUsage int
		instTextAddr   uint64
	)

	if prog.man.StackUsage != 0 {
		if entryFuncIndex >= 0 {
			return nil, notfound.ErrSuspended
		}

		instStackUsage = int(prog.man.StackUsage)
		instTextAddr = prog.man.TextAddr
	}

	if instStackUsage > instStackSize-executable.StackUsageOffset {
		return nil, resourcelimit.Error("call stack size limit exceeded")
	}

	instFile, err := prog.storage.newInstanceFile()
	if err != nil {
		return nil, err
	}
	defer func() {
		if instFile != nil {
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
			if err := copyFileRange(prog.file, &off1, instFile, &off2, copyLen); err != nil {
				return nil, err
			}
		} else {
			dest, err := mmap(instFile.FD(), off2, copyLen, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
			if err != nil {
				return nil, err
			}
			defer mustMunmap(dest)

			if _, err := prog.file.ReadAt(dest[:copyLen], off1); err != nil {
				return nil, err
			}
		}
	}

	inst := &Instance{
		man: &pb.InstanceManifest{
			TextAddr:      instTextAddr,
			StackSize:     uint32(instStackSize),
			StackUsage:    uint32(instStackUsage),
			GlobalsSize:   prog.man.GlobalsSize,
			MemorySize:    prog.man.MemorySize,
			MaxMemorySize: uint32(maxMemorySize),
			StartFunc:     prog.man.StartFunc,
			EntryFunc:     programEntryFunc(prog.man, entryFuncIndex),
			Snapshot:      snapshot.Clone(prog.man.Snapshot),
		},
		manDirty: true,
		coherent: true,
		file:     instFile,
	}
	instFile = nil
	return inst, nil
}

func (inst *Instance) SetEntryFunc(prog *Program, index int) error {
	if !inst.coherent {
		return ErrInvalidState
	}
	if index >= 0 && inst.man.StackUsage != 0 {
		return notfound.ErrSuspended
	}

	inst.man.EntryFunc = programEntryFunc(prog.man, index)
	inst.manDirty = true
	return nil
}

// Store the instance.  The names must not contain path separators.
func (inst *Instance) Store(name, progName string, prog *Program) error {
	if !inst.coherent {
		return ErrInvalidState
	}
	if inst.name != "" {
		return errors.New("instance already stored")
	}

	if prog.storage.storeInstanceSupported() {
		return prog.storage.storeInstance(inst, name)
	}

	inst.name = name
	return nil
}

func (inst *Instance) Unstore() error {
	if inst.name == "" {
		return nil
	}

	dir := inst.dir
	name := inst.name
	inst.dir = nil
	inst.name = ""

	if dir == nil {
		return nil
	}

	if err := syscall.Unlinkat(dir.FD(), name); err != nil {
		if err == syscall.ENOENT {
			return fdatasync(dir.FD())
		}

		return fmt.Errorf("unlinkat instance %q: %w", name, err)
	}

	return fdatasync(dir.FD())
}

func (inst *Instance) Close() error {
	err := inst.file.Close()
	inst.file = nil
	return err
}

func (inst *Instance) TextAddr() uint64      { return inst.man.TextAddr }
func (inst *Instance) StackSize() int        { return int(inst.man.StackSize) }
func (inst *Instance) StackUsage() int       { return int(inst.man.StackUsage) }
func (inst *Instance) GlobalsSize() int      { return alignPageSize32(inst.man.GlobalsSize) }
func (inst *Instance) MemorySize() int       { return int(inst.man.MemorySize) }
func (inst *Instance) MaxMemorySize() int    { return int(inst.man.MaxMemorySize) }
func (inst *Instance) StartAddr() uint32     { return inst.man.StartFunc.GetAddr() }
func (inst *Instance) EntryAddr() uint32     { return inst.man.EntryFunc.GetAddr() }
func (inst *Instance) Final() bool           { return inst.man.Snapshot.GetFinal() }
func (inst *Instance) Trap() trap.ID         { return trap.ID(inst.man.Snapshot.GetTrap()) }
func (inst *Instance) Result() int32         { return inst.man.Snapshot.GetResult() }
func (inst *Instance) MonotonicTime() uint64 { return inst.man.Snapshot.GetMonotonicTime() }

func (inst *Instance) globalsPageOffset() int64 {
	return instStackOffset + int64(inst.man.StackSize)
}

func (inst *Instance) memoryOffset() int64 {
	return inst.globalsPageOffset() + alignPageOffset32(inst.man.GlobalsSize)
}

// Breakpoints are in ascending order and unique.
func (inst *Instance) Breakpoints() []uint64 {
	return inst.man.Snapshot.GetBreakpoints()
}

func (inst *Instance) SetFinal() {
	if inst.man.Snapshot.GetFinal() {
		return
	}
	inflate(&inst.man.Snapshot).Final = true
	inst.manDirty = true
}

func (inst *Instance) SetTrap(id trap.ID) {
	if inst.man.Snapshot.GetTrap() == id {
		return
	}
	inflate(&inst.man.Snapshot).Trap = id
	inst.manDirty = true
}

func (inst *Instance) SetResult(n int32) {
	if inst.man.Snapshot.GetResult() == n {
		return
	}
	inflate(&inst.man.Snapshot).Result = n
	inst.manDirty = true
}

// SetBreakpoints which must have been sorted and deduplicated.
func (inst *Instance) SetBreakpoints(a []uint64) {
	if len(inst.man.Snapshot.GetBreakpoints()) == len(a) {
		for i, x := range inst.man.Snapshot.GetBreakpoints() {
			if x != a[i] {
				goto changed
			}
		}
		return
	}
changed:
	inflate(&inst.man.Snapshot).Breakpoints = a
	inst.manDirty = true
}

// BeginMutation must be invoked when mutation starts.  CheckMutation and Close
// may be called during the mutation.  The returned file handle is valid until
// the next Instance method call.
func (inst *Instance) BeginMutation(textAddr uint64) (file interface{ Fd() uintptr }, err error) {
	if !inst.coherent {
		return nil, ErrInvalidState
	}

	if err := inst.Unstore(); err != nil {
		return nil, err
	}

	// Text address is currently unused, as it's later read from stack vars.

	inst.coherent = false
	return inst.file, nil
}

// CheckMutation can be invoked after the mutation has ended.
func (inst *Instance) CheckMutation() error {
	_, err := inst.checkMutation()
	return err
}

// CheckHaltedMutation is like CheckMutation, but it also returns the result of
// the top-level function.  The result is undefined if the program terminated
// in some other way.
//
// This is useful only with ReplaceCallStack, if you know that the program
// exits by returning.
func (inst *Instance) CheckHaltedMutation(result wa.ScalarCategory) (uint64, error) {
	vars, err := inst.checkMutation()
	if err != nil {
		return 0, err
	}
	return vars.Result[result], nil
}

func (inst *Instance) checkMutation() (stackVars, error) {
	if inst.coherent {
		return stackVars{}, nil
	}

	b := make([]byte, executable.StackVarsSize)

	if _, err := inst.file.ReadAt(b, 0); err != nil {
		return stackVars{}, err
	}

	vars, ok := checkStack(b, inst.man.StackSize)
	if !ok {
		return stackVars{}, ErrInvalidState
	}

	if vars.StackUnused != 0 {
		if vars.StackUnused == inst.man.StackSize {
			inst.man.TextAddr = 0
			inst.man.StackUsage = 0
		} else {
			inst.man.TextAddr = vars.TextAddr
			inst.man.StackUsage = inst.man.StackSize - vars.StackUnused
		}
		inst.man.MemorySize = vars.CurrentMemoryPages << wa.PageBits
		inst.man.StartFunc = nil
		inst.man.EntryFunc = nil
		inflate(&inst.man.Snapshot).MonotonicTime = vars.MonotonicTimeSnapshot
		inst.manDirty = true
	}

	inst.coherent = true
	return vars, nil
}

func (inst *Instance) Globals(prog *Program) ([]uint64, error) {
	var (
		instGlobalsEnd    = int64(inst.man.StackSize) + alignPageOffset32(inst.man.GlobalsSize)
		instGlobalsOffset = instGlobalsEnd - int64(inst.man.GlobalsSize)
	)

	b := make([]byte, inst.man.GlobalsSize)
	if _, err := inst.file.ReadAt(b, instGlobalsOffset); err != nil {
		return nil, err
	}

	values := make([]uint64, len(prog.man.GlobalTypes))
	for i := range values {
		values[i] = binary.LittleEndian.Uint64(b[len(b)-(i+1)*8:])
	}

	return values, nil
}

// ReplaceCallStack with a "suspended" function call with given arguments.
// Pending start function, entry function, and existing suspended state are
// discarded.  Arguments are not checked against function signature.
//
// This is a low-level API primarily useful for testing.
func (inst *Instance) ReplaceCallStack(funcAddr uint32, funcArgs []uint64) error {
	if !inst.coherent {
		return ErrInvalidState
	}

	textAddr := inst.man.TextAddr
	if textAddr == 0 {
		b := make([]byte, 4)
		if _, err := rand.Read(b); err != nil {
			return err
		}
		textAddr = executable.RandAddr(executable.MinTextAddr, executable.MaxTextAddr, b)
	}

	count := 1 + 1 + len(funcArgs) // Resume address, link address and params.
	stack := make([]byte, count*8)

	if runtime.GOARCH == "arm64" {
		// Resume after first instruction of function prologue, to avoid
		// storing link register back on stack.
		funcAddr += 4
	}

	b := stack
	binary.LittleEndian.PutUint64(b, textAddr+uint64(funcAddr)) // Resume at function start.
	b = b[8:]
	binary.LittleEndian.PutUint64(b, textAddr+abi.TextAddrExit) // Return to exit routine.
	b = b[8:]
	for i := len(funcArgs) - 1; i >= 0; i-- {
		binary.LittleEndian.PutUint64(b, funcArgs[i])
		b = b[8:]
	}

	if _, err := inst.file.WriteAt(stack, int64(inst.man.StackSize)-int64(len(stack))); err != nil {
		return err
	}

	inst.man.TextAddr = textAddr
	inst.man.StackUsage = uint32(len(stack))
	inst.man.StartFunc = nil
	inst.man.EntryFunc = nil
	inst.manDirty = true
	return nil
}

func (inst *Instance) readStack() ([]byte, error) {
	b := make([]byte, inst.man.StackSize)

	if _, err := inst.file.ReadAt(b, 0); err != nil {
		return nil, err
	}

	if inst.man.StackUsage == 0 {
		return nil, nil
	}

	return b[len(b)-int(inst.man.StackUsage):], nil
}

func (inst *Instance) ExportStack(textMap stack.TextMap) ([]byte, error) {
	b, err := inst.readStack()
	if err != nil || len(b) == 0 {
		return nil, err
	}

	if err := exportStack(b, b, inst.man.TextAddr, textMap); err != nil {
		return nil, err
	}
	return b, nil
}

func (inst *Instance) Stacktrace(textMap stack.TextMap, funcTypes []wa.FuncType) ([]stack.Frame, error) {
	b, err := inst.readStack()
	if err != nil || len(b) == 0 {
		return nil, err
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
	case vars.StackUnused < executable.StackUsageOffset || vars.StackUnused > stackSize || vars.StackUnused&7 != 0:
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

func maxInstanceMemory(prog *Program, n int) (int, error) {
	if prog.man.MemorySizeLimit >= 0 && n > int(prog.man.MemorySizeLimit) {
		n = int(prog.man.MemorySizeLimit)
	}
	if n >= 0 && n < int(prog.man.MemorySize) {
		return n, resourcelimit.Error("out of memory")
	}
	return n, nil
}

var pageMask = int64(executable.PageSize - 1)

func align8(n int64) int64             { return (n + 7) &^ 7 }
func alignPageSize(n int) int          { return int(alignPageOffset(int64(n))) }
func alignPageSize32(n uint32) int     { return int(alignPageOffset(int64(n))) }
func alignPageOffset(n int64) int64    { return (n + pageMask) &^ pageMask }
func alignPageOffset32(n uint32) int64 { return alignPageOffset(int64(n)) }

func inflate(p **snapshot.Snapshot) *snapshot.Snapshot {
	if *p == nil {
		*p = new(snapshot.Snapshot)
	}
	return *p
}
