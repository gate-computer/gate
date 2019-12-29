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

	internal "github.com/tsavola/gate/internal/error/runtime"
	"github.com/tsavola/gate/internal/executable"
	"github.com/tsavola/gate/internal/file"
	"github.com/tsavola/gate/snapshot"
	"github.com/tsavola/wag/object/abi"
	"github.com/tsavola/wag/trap"
)

const imageInfoSize = 104

// imageInfo is like the info object in runtime/loader/loader.c
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
	Debug          io.Writer
}

type ProcessFactory interface {
	NewProcess(context.Context) (*Process, error)
}

type TrapID trap.ID

const (
	TrapExit = TrapID(trap.Exit)

	TrapSuspended                     = TrapID(trap.Suspended)
	TrapUnreachable                   = TrapID(trap.Unreachable)
	TrapCallStackExhausted            = TrapID(trap.CallStackExhausted)
	TrapMemoryAccessOutOfBounds       = TrapID(trap.MemoryAccessOutOfBounds)
	TrapIndirectCallIndexOutOfBounds  = TrapID(trap.IndirectCallIndexOutOfBounds)
	TrapIndirectCallSignatureMismatch = TrapID(trap.IndirectCallSignatureMismatch)
	TrapIntegerDivideByZero           = TrapID(trap.IntegerDivideByZero)
	TrapIntegerOverflow               = TrapID(trap.IntegerOverflow)

	TrapABIDeficiency = TrapID(26)
)

func (id TrapID) String() string {
	switch id {
	case TrapABIDeficiency:
		return "ABI deficiency"

	default:
		return trap.ID(id).String()
	}
}

// Process is used to execute a single program image once.  Created via an
// Executor or a derivative ProcessFactory.
//
// A process is idle until its Start method is called.  Kill must eventually be
// called to release resources.
type Process struct {
	execution execProcess // Executor's low-level process state.
	writer    *file.File
	writerOut *file.Ref
	reader    *os.File
	suspended chan struct{}
	debugFile *os.File
	debugging <-chan struct{}
}

func newProcess(ctx context.Context, e *Executor) (*Process, error) {
	inputR, inputW, err := socketPipe()
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			inputW.Close()
			inputR.Close()
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

	err = e.execute(ctx, &p.execution, inputR, outputW)
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
// This function must be called before Serve, and must not be called after
// Kill.
func (p *Process) Start(code ProgramCode, state ProgramState, policy ProcessPolicy) (err error) {
	textAddr, heapAddr, stackAddr, random, err := getRand(state.TextAddr(), code.Random())
	if err != nil {
		return
	}

	if policy.TimeResolution <= 0 || policy.TimeResolution > time.Second {
		policy.TimeResolution = time.Second
	}
	timeMask := ^(1<<uint(bits.Len32(uint32(policy.TimeResolution))) - 1)

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
		MonotonicTime:  state.MonotonicTime(),
		TimeMask:       uint32(timeMask),
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
	if policy.Debug != nil {
		debugReader, debugWriter, err = os.Pipe()
		if err != nil {
			return
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
		return
	}

	stateFile, err := state.BeginMutation(textAddr)
	if err != nil {
		return
	}

	var cmsg []byte
	if policy.Debug == nil {
		cmsg = unixRights(int(textFile.Fd()), int(stateFile.Fd()))
	} else {
		cmsg = unixRights(int(debugWriter.Fd()), int(textFile.Fd()), int(stateFile.Fd()))
	}

	err = sendmsg(p.writer.Fd(), buf.Bytes(), cmsg, nil, 0)
	if err != nil {
		return
	}

	if policy.Debug != nil {
		done := make(chan struct{})
		go copyDebug(done, policy.Debug, debugReader)
		p.debugging = done
	}

	return
}

// Serve the user program until the process terminates.  Canceling the context
// suspends the program.
//
// Start must have been called before this.  This must not be called after
// Kill.
//
// Buffers will be mutated (unless nil).
func (p *Process) Serve(ctx context.Context, services ServiceRegistry, buffers *snapshot.Buffers,
) (exit int, trapID TrapID, err error) {
	err = ioLoop(ctx, services, p, buffers)
	if err != nil {
		return
	}

	status, err := p.killWait()
	if err != nil {
		return
	}

	if p.debugging != nil {
		<-p.debugging
	}

	switch {
	case status.Exited():
		code := status.ExitStatus()

		if code >= 0 && code <= 3 {
			exit = code & 1
			if code&2 != 0 && buffers != nil {
				buffers.SetTerminated()
			}
			return
		}

		if code >= 100 && code <= 127 {
			trapID = TrapID(code - 100)
			return
		}

		err = internal.ProcessError(code)
		return

	case status.Signaled():
		if status.Signal() == syscall.SIGXCPU {
			trapID = TrapSuspended // During initialization (ok) or by force (stack is dirty).
			return
		}

		err = fmt.Errorf("process termination signal: %v", status.Signal())
		return

	default:
		err = fmt.Errorf("unknown process status: %d", status)
		return
	}
}

// Suspend the program unless the process has already been terminated.  Serve
// call will return with github.com/tsavola/wag/trap.Suspended error.  This is
// a no-op if the process has already been killed or suspended.
//
// This can be called concurrently with Start, Serve and Kill.
func (p *Process) Suspend() {
	select {
	case p.suspended <- struct{}{}:
	default:
	}
}

// Kill the process and release resources.  If the program has been suspended
// successfully, this only releases resources.
//
// This function can be called multiple times.
func (p *Process) Kill() {
	p.execution.kill(false)

	if p.reader != nil {
		p.reader.Close()
		p.reader = nil
	}

	if p.writer != nil {
		p.writer.Close()
		p.writer = nil
	}

	if p.writerOut != nil {
		p.writerOut.Close()
		p.writerOut = nil
	}

	if p.debugFile != nil {
		p.debugFile.Close()
		p.debugFile = nil
	}
}

func (p *Process) killSuspend() {
	p.execution.kill(true)
}

func (p *Process) killWait() (syscall.WaitStatus, error) {
	return p.execution.killWait()
}

func getRand(fixedTextAddr uint64, needData bool,
) (textAddr, heapAddr, stackAddr uint64, randData [16]byte, err error) {
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
	}

	return
}

var _ struct{} = internal.ErrorsInitialized
