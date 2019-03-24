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

const imageInfoSize = 72

// imageInfo is like the info object in runtime/loader/loader.c
type imageInfo struct {
	MagicNumber1   uint16
	InitRoutine    uint8
	RandomGlobal   int8
	PageSize       uint32
	TextAddr       uint64
	StackAddr      uint64
	HeapAddr       uint64
	RandomValue    uint64
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
	RandomGlobal() int8
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

type ProcessFactory interface {
	NewProcess(context.Context) (*Process, error)
}

// Process is used to execute a single program image once.  Created via an
// Executor or a derivative ProcessFactory.
//
// A process is idle until its Start method is called.  Kill must eventually be
// called to release resources.
type Process struct {
	execution execProcess // Executor's low-level process state.
	writer    *os.File
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
func (p *Process) Start(code ProgramCode, state ProgramState, debugOutput io.Writer) (err error) {
	randGlobal := code.RandomGlobal()
	textAddr := state.TextAddr()
	textAddr, heapAddr, stackAddr, randVal, err := getRand(textAddr, randGlobal != 0)
	if err != nil {
		return
	}

	info := imageInfo{
		MagicNumber1:   magicNumber1,
		InitRoutine:    state.InitRoutine(),
		RandomGlobal:   randGlobal,
		PageSize:       uint32(code.PageSize()),
		TextAddr:       textAddr,
		StackAddr:      stackAddr,
		HeapAddr:       heapAddr,
		RandomValue:    randVal,
		TextSize:       uint32(code.TextSize()),
		StackSize:      uint32(state.StackSize()),
		StackUnused:    uint32(state.StackSize() - state.StackUsage()),
		GlobalsSize:    uint32(state.GlobalsSize()),
		InitMemorySize: uint32(state.MemorySize()),
		GrowMemorySize: uint32(state.MaxMemorySize()), // TODO: check policy too
		EntryAddr:      state.EntryAddr(),
		MagicNumber2:   magicNumber2,
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

	var (
		debugReader *os.File
		debugWriter *os.File
	)
	if debugOutput != nil {
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
	if debugOutput == nil {
		cmsg = unixRights(int(textFile.Fd()), int(stateFile.Fd()))
	} else {
		cmsg = unixRights(int(debugWriter.Fd()), int(textFile.Fd()), int(stateFile.Fd()))
	}

	err = sendmsg(p.writer.Fd(), buf.Bytes(), cmsg, nil, 0)
	if err != nil {
		return
	}

	if debugOutput != nil {
		done := make(chan struct{})
		go copyDebug(done, debugOutput, debugReader)
		p.debugging = done
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

func getRand(fixedTextAddr uint64, needRandVal bool,
) (textAddr, heapAddr, stackAddr, randVal uint64, err error) {
	n := 4 + 4
	if fixedTextAddr == 0 {
		n += 4
	}
	if needRandVal {
		n += 8
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

	if needRandVal {
		randVal = binary.LittleEndian.Uint64(b)
		b = b[8:]
	}

	return
}

var _ struct{} = internal.ErrorsInitialized
