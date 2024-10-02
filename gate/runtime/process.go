// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"math/bits"
	"os"
	"syscall"
	"time"
	"unsafe"

	"gate.computer/gate/snapshot"
	"gate.computer/gate/trap"
	"gate.computer/internal/error/badprogram"
	internal "gate.computer/internal/error/runtime"
	"gate.computer/internal/executable"
	"gate.computer/internal/file"
	"gate.computer/wag/object/abi"

	. "import.name/type/context"
)

const imageInfoSize = 104

// imageInfo is like ImageInfo in runtime/loader/loader.cpp
type imageInfo struct {
	MagicNumber1   uint32
	PageSize       uint32
	TextAddr       uint64
	StackAddr      uint64
	HeapAddr       uint64
	Random         [16]byte
	TextSize       uint32
	StackSize      uint32
	StackUnused    uint32
	GlobalsSize    uint32
	InitMemorySize uint32
	GrowMemorySize uint32
	InitRoutine    uint32
	StartAddr      uint32
	EntryAddr      uint32
	TimeMask       uint32
	MonotonicTime  uint64
	MagicNumber2   uint64
}

type ProgramCode interface {
	PageSize() int
	TextSize() int
	Random() bool
	Text() (interface{ Fd() uintptr }, error)
}

type ProgramState interface {
	TextAddr() uint64
	StackSize() int
	StackUsage() int
	GlobalsSize() int
	MemorySize() int
	MaxMemorySize() int
	StartAddr() uint32
	EntryAddr() uint32
	MonotonicTime() uint64
	BeginMutation(textAddr uint64) (interface{ Fd() uintptr }, error)
}

type ProcessPolicy struct {
	TimeResolution time.Duration
	DebugLog       io.Writer
}

type ProcessFactory interface {
	NewProcess(Context) (*Process, error)
}

type Result struct {
	code int
}

// ResultSuccess is the Result.Value() which indicates success.
const ResultSuccess = 0

// Terminated instead of halting?
func (r Result) Terminated() bool { return r.code&2 != 0 }
func (r Result) Value() int       { return r.code & 1 }

func (r Result) String() string {
	if r.code < 0 || r.code > 3 {
		return "invalid result code"
	}

	if r.Terminated() {
		if r.Value() == ResultSuccess {
			return "terminated successfully"
		} else {
			return "terminated unsuccessfully"
		}
	} else {
		if r.Value() == ResultSuccess {
			return "halted successfully"
		} else {
			return "halted unsuccessfully"
		}
	}
}

// Process is used to execute a single program image once.  Created via an
// Executor or a derivative ProcessFactory.
//
// A process is idle until its Start method is called.  Close must eventually
// be called to release resources.
type Process struct {
	execution execProcess // Executor's low-level process state.
	writer    *file.File
	writerOut file.Ref
	reader    *os.File
	suspended chan struct{}
	debugFile *os.File
	debugging <-chan struct{}
}

func newProcess(ctx Context, e *Executor, group file.Ref) (*Process, error) {
	inputR, inputW, err := socketPipe()
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			inputW.Close()
			inputR.Unref()
		}
	}()

	outputR, outputW, err := pipe2(syscall.O_NONBLOCK)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			outputW.Close()
			outputR.Close()
		}
	}()

	p := new(Process)

	err = e.execute(ctx, &p.execution, inputR, outputW, group)
	if err != nil {
		return nil, err
	}

	p.writer = inputW
	p.writerOut = inputR
	p.reader = outputR
	p.suspended = make(chan struct{}, 1)

	return p, nil
}

// Start the program.  The program state will be undergoing mutation until the
// process terminates.
//
// This function must be called before Serve.
func (p *Process) Start(code ProgramCode, state ProgramState, policy ProcessPolicy) error {
	textAddr, heapAddr, stackAddr, random, err := getRand(state.TextAddr(), code.Random())
	if err != nil {
		return err
	}

	if policy.TimeResolution <= 0 || policy.TimeResolution > time.Second {
		policy.TimeResolution = time.Second
	}
	timeMask := ^uint32(1<<uint(bits.Len32(uint32(policy.TimeResolution/time.Nanosecond))) - 1)

	info := imageInfo{
		MagicNumber1:   magicNumber1,
		PageSize:       uint32(code.PageSize()),
		TextAddr:       textAddr,
		StackAddr:      stackAddr,
		HeapAddr:       heapAddr,
		Random:         random,
		TextSize:       uint32(code.TextSize()),
		StackSize:      uint32(state.StackSize()),
		StackUnused:    uint32(state.StackSize() - state.StackUsage()),
		GlobalsSize:    uint32(state.GlobalsSize()),
		InitMemorySize: uint32(state.MemorySize()),
		GrowMemorySize: uint32(state.MaxMemorySize()), // TODO: check policy too
		StartAddr:      state.StartAddr(),
		EntryAddr:      state.EntryAddr(),
		TimeMask:       timeMask,
		MonotonicTime:  state.MonotonicTime(),
		MagicNumber2:   magicNumber2,
	}
	if info.StackUnused == info.StackSize {
		info.InitRoutine = abi.TextAddrEnter
	} else {
		info.InitRoutine = abi.TextAddrResume
	}

	buf := bytes.NewBuffer(make([]byte, 0, imageInfoSize))

	if err := binary.Write(buf, binary.LittleEndian, &info); err != nil {
		panic(err)
	}

	var (
		debugReader *os.File
		debugWriter *os.File
	)
	if policy.DebugLog != nil {
		debugReader, debugWriter, err = os.Pipe()
		if err != nil {
			return err
		}
		defer func() {
			debugWriter.Close()
			if err != nil {
				debugReader.Close()
			}
		}()
	}

	textFile, err := code.Text()
	if err != nil {
		return err
	}

	stateFile, err := state.BeginMutation(textAddr)
	if err != nil {
		return err
	}

	var cmsg []byte
	if policy.DebugLog == nil {
		cmsg = syscall.UnixRights(int(textFile.Fd()), int(stateFile.Fd()))
	} else {
		cmsg = syscall.UnixRights(int(debugWriter.Fd()), int(textFile.Fd()), int(stateFile.Fd()))
	}

	if err := syscall.Sendmsg(p.writer.FD(), buf.Bytes(), cmsg, nil, 0); err != nil {
		return err
	}

	if policy.DebugLog != nil {
		done := make(chan struct{})
		go copyDebug(done, policy.DebugLog, debugReader)
		p.debugging = done
	}

	return nil
}

// Serve the user program until the process terminates.  Canceling the context
// suspends the program.
//
// Start must have been called before this.
//
// Buffers will be mutated (unless nil).
//
// A meaningful trap id is returned also when an error is returned.  The result
// is meaningful when trap is Exit.
func (p *Process) Serve(ctx Context, services ServiceRegistry, buffers *snapshot.Buffers) (Result, trap.ID, error) {
	if err := ioLoop(contextWithProcess(ctx, p), services, p, buffers); err != nil {
		trapID := trap.InternalError
		if badprogram.Is(err) {
			trapID = trap.ABIViolation
		}
		return Result{}, trapID, err
	}

	status, err := p.execution.finalize()
	if err != nil {
		return Result{}, trap.InternalError, err
	}

	if p.debugging != nil {
		<-p.debugging
	}

	switch {
	case status.Exited():
		var trapID trap.ID
		var result Result

		switch n := status.ExitStatus(); {
		case n >= 0 && n <= 3:
			trapID = trap.Exit
			result = Result{n}

		case n >= 100 && n <= 127:
			trapID = trap.ID(n - 100)

		default:
			return Result{}, trap.InternalError, internal.ProcessError(n)
		}

		if p.execution.killRequested() {
			switch trapID {
			case trap.Exit, trap.Suspended, trap.Breakpoint, trap.ABIDeficiency:
				trapID = trap.Killed
			}
		}

		return result, trapID, nil

	case status.Signaled():
		switch s := status.Signal(); {
		case s == os.Kill && p.execution.killRequested():
			return Result{}, trap.Killed, nil

		case s == syscall.SIGXCPU:
			// During initialization (ok) or by force (instance stack is dirty).
			return Result{}, trap.Suspended, nil

		default:
			return Result{}, trap.InternalError, fmt.Errorf("process termination signal: %s", s)
		}

	default:
		return Result{}, trap.InternalError, fmt.Errorf("unknown process status: %d", status)
	}
}

// Suspend the program if it is still running.  If suspended, Serve call will
// return with the Suspended trap.  (The program may get suspended also through
// other means.)
//
// This can be called multiple times, concurrently with Start, Serve, Kill,
// Close and itself.
func (p *Process) Suspend() {
	select {
	case p.suspended <- struct{}{}:
	default:
	}
}

// Kill the process if it is still alive.  If killed, Serve call will return
// with the Killed trap.
//
// This can be called multiple times, concurrently with Start, Serve, Suspend,
// Close and itself.
func (p *Process) Kill() {
	p.execution.kill()
}

// Close must not be called concurrently with Start or Serve.
func (p *Process) Close() error {
	p.execution.kill()

	if p.reader != nil {
		p.reader.Close()
		p.reader = nil
	}

	if p.writer != nil {
		p.writer.Close()
		p.writer = nil
	}

	p.writerOut.Unref()

	if p.debugFile != nil {
		p.debugFile.Close()
		p.debugFile = nil
	}

	return nil
}

type contextProcessValueKey struct{}

func contextWithProcess(ctx Context, p *Process) Context {
	return context.WithValue(ctx, contextProcessValueKey{}, p)
}

// ContextWithDummyProcessKey for testing.
func ContextWithDummyProcessKey(ctx Context) Context {
	invalid := new(Process)
	return context.WithValue(ctx, contextProcessValueKey{}, invalid)
}

// ProcessKey is an opaque handle to a single instance of a program image being
// executed.
type ProcessKey struct{ p *Process }

func (key ProcessKey) Compare(other ProcessKey) int {
	a := uintptr(unsafe.Pointer(key.p))
	b := uintptr(unsafe.Pointer(other.p))
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func MustContextProcessKey(ctx Context) ProcessKey {
	return ProcessKey{ctx.Value(contextProcessValueKey{}).(*Process)}
}

func getRand(fixedTextAddr uint64, needData bool) (textAddr, heapAddr, stackAddr uint64, randData [16]byte, err error) {
	n := 4 + 4
	if fixedTextAddr == 0 {
		n += 4
	}
	if needData {
		n += 16
	}

	b := make([]byte, n)
	_, err = rand.Read(b)
	if err != nil {
		return
	}

	heapAddr = executable.RandAddr(executable.MinHeapAddr, executable.MaxHeapAddr, b)
	b = b[4:]

	stackAddr = executable.RandAddr(executable.MinStackAddr, executable.MaxStackAddr, b)
	b = b[4:]

	if fixedTextAddr != 0 {
		textAddr = fixedTextAddr
	} else {
		textAddr = executable.RandAddr(executable.MinTextAddr, executable.MaxTextAddr, b)
		b = b[4:]
	}

	if needData {
		copy(randData[:], b)
		b = b[16:]
		_ = b
	}

	return
}
