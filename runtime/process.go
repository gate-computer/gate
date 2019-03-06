// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"

	internal "github.com/tsavola/gate/internal/error/runtime"
	"github.com/tsavola/gate/internal/executable"
	"github.com/tsavola/gate/internal/file"
	"github.com/tsavola/gate/snapshot"
	"github.com/tsavola/wag/object/abi"
	"github.com/tsavola/wag/trap"
)

const imageInfoSize = 64

// imageInfo is like the info object in runtime/loader/loader.c
type imageInfo struct {
	MagicNumber1   uint16
	DebugFlag      uint8
	InitRoutine    uint8
	PageSize       uint32
	TextAddr       uint64
	StackAddr      uint64
	HeapAddr       uint64
	TextSize       uint32
	StackSize      uint32
	StackUnused    uint32
	GlobalsSize    uint32
	InitMemorySize uint32
	GrowMemorySize uint32
	EntryAddr      uint32
	MagicNumber2   uint32
}

type ProgramCode interface {
	PageSize() int
	TextSize() int
	Text() (interface{ Fd() uintptr }, error)
}

type ProgramState interface {
	TextAddr() uint64
	StackSize() int
	StackUsage() int
	GlobalsSize() int
	MemorySize() int
	MaxMemorySize() int
	InitRoutine() uint8
	EntryAddr() uint32
	BeginMutation(textAddr uint64) (interface{ Fd() uintptr }, error)
}

// Process is used to execute a single program image once.
type Process struct {
	execution execProcess // Executor's low-level process state.
	writer    *os.File
	writerOut *file.Ref
	reader    *os.File
	suspended chan struct{}
	debugging <-chan struct{}
}

// Allocate a process using the given executor.
//
// The process is idle until its Start method is called.  Kill must be
// eventually called to release resources.
func NewProcess(ctx context.Context, e *Executor, debug io.Writer) (p *Process, err error) {
	var (
		inputR  *file.Ref
		inputW  *os.File
		outputR *os.File
		outputW *file.File
		debugR  *os.File
		debugW  *os.File
	)

	defer func() {
		if inputR != nil {
			inputR.Close()
		}
		if inputW != nil {
			inputW.Close()
		}
		if outputR != nil {
			outputR.Close()
		}
		if outputW != nil {
			outputW.Close()
		}
		if debugR != nil {
			debugR.Close()
		}
		if debugW != nil {
			debugW.Close()
		}
	}()

	inputR, inputW, err = socketPipe()
	if err != nil {
		return
	}

	outputR, outputW, err = pipe2(syscall.O_NONBLOCK)
	if err != nil {
		return
	}

	if debug != nil {
		debugR, debugW, err = os.Pipe()
		if err != nil {
			return
		}
	}

	p = new(Process)

	err = e.execute(ctx, &p.execution, inputR, outputW, debugW)
	if err != nil {
		return
	}

	p.writer = inputW
	p.writerOut = inputR
	p.reader = outputR
	p.suspended = make(chan struct{}, 1)

	if debug != nil {
		done := make(chan struct{})
		go copyDebug(done, debug, debugR)
		p.debugging = done
	}

	inputR = nil
	inputW = nil
	outputR = nil
	outputW = nil
	debugR = nil
	debugW = nil
	return
}

// Start the program.  The program state will be undergoing mutation until the
// process terminates.
//
// This function must be called before Serve, and must not be called after
// Kill.
func (p *Process) Start(code ProgramCode, state ProgramState) (err error) {
	textAddr, heapAddr, stackAddr, err := generateRandAddrs(state.TextAddr())
	if err != nil {
		return
	}

	info := imageInfo{
		MagicNumber1:   magicNumber1,
		InitRoutine:    state.InitRoutine(),
		PageSize:       uint32(code.PageSize()),
		TextAddr:       textAddr,
		StackAddr:      stackAddr,
		HeapAddr:       heapAddr,
		TextSize:       uint32(code.TextSize()),
		StackSize:      uint32(state.StackSize()),
		StackUnused:    uint32(state.StackSize() - state.StackUsage()),
		GlobalsSize:    uint32(state.GlobalsSize()),
		InitMemorySize: uint32(state.MemorySize()),
		GrowMemorySize: uint32(state.MaxMemorySize()), // TODO: check policy too
		EntryAddr:      state.EntryAddr(),
		MagicNumber2:   magicNumber2,
	}

	if p.debugging != nil {
		info.DebugFlag = 1
	}

	switch info.InitRoutine {
	case abi.TextAddrNoFunction, abi.TextAddrStart, abi.TextAddrEnter:

	case abi.TextAddrResume:
		if info.StackUnused == info.StackSize {
			err = errors.New("resuming without stack contents")
			return
		}

	default:
		panic(info.InitRoutine)
	}

	buf := bytes.NewBuffer(make([]byte, 0, imageInfoSize))

	if err := binary.Write(buf, binary.LittleEndian, &info); err != nil {
		panic(err)
	}

	textFile, err := code.Text()
	if err != nil {
		return
	}

	stateFile, err := state.BeginMutation(textAddr)
	if err != nil {
		return
	}

	cmsg := unixRights(int(textFile.Fd()), int(stateFile.Fd()))

	err = sendmsg(p.writer.Fd(), buf.Bytes(), cmsg, nil, 0)
	if err != nil {
		return
	}

	return
}

// Serve the user program until it terminates.  Canceling the context suspends
// the program.
//
// Start must have been called before this.  This must not be called after
// Kill.
//
// Buffers will be mutated.
func (p *Process) Serve(ctx context.Context, services ServiceRegistry, buffers *snapshot.Buffers,
) (exit int, trapID trap.ID, err error) {
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

		switch code {
		case 0, 1:
			exit = code
			return
		}

		if n := code - 100; n >= 0 && n < int(trap.NumTraps) {
			trapID = trap.ID(n)
			return
		}

		err = internal.ProcessError(code)
		return

	case status.Signaled():
		if status.Signal() == syscall.SIGXCPU {
			trapID = trap.Suspended // During initialization (ok) or by force (stack is dirty).
			return
		}

		err = fmt.Errorf("process termination signal: %d", status.Signal())
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
	if p.reader == nil {
		return
	}

	p.execution.kill(false)

	p.reader.Close()
	p.reader = nil

	if p.writer != nil {
		p.writer.Close()
		p.writer = nil
	}

	if p.writerOut != nil {
		p.writerOut.Close()
		p.writerOut = nil
	}
}

func (p *Process) killSuspend() {
	p.execution.kill(true)
}

func (p *Process) killWait() (syscall.WaitStatus, error) {
	return p.execution.killWait()
}

func generateRandAddrs(fixedTextAddr uint64) (textAddr, heapAddr, stackAddr uint64, err error) {
	b := make([]byte, 12)

	if fixedTextAddr != 0 {
		_, err = rand.Read(b[:8])
		if err != nil {
			return
		}

		textAddr = fixedTextAddr
	} else {
		_, err = rand.Read(b[:12])
		if err != nil {
			return
		}

		textAddr = executable.RandAddr(executable.MinTextAddr, executable.MaxTextAddr, b[8:])
	}

	heapAddr = executable.RandAddr(executable.MinHeapAddr, executable.MaxHeapAddr, b[4:])
	stackAddr = executable.RandAddr(executable.MinStackAddr, executable.MaxStackAddr, b[0:])
	return
}

var _ struct{} = internal.ErrorsInitialized
