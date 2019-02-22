// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"syscall"

	internal "github.com/tsavola/gate/internal/error/runtime"
	"github.com/tsavola/gate/internal/executable"
	"github.com/tsavola/gate/internal/file"
	"github.com/tsavola/wag/trap"
)

// imageInfo is like the info object in runtime/loader/loader.c
type imageInfo struct {
	MagicNumber1   uint32
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
	InitRoutine    uint16
	DebugFlag      uint16
	MagicNumber2   uint32
}

type ExecutableRef = file.OpaqueRef

type Executable interface {
	PageSize() uint32
	TextAddr() uint64
	SetTextAddr(uint64)
	TextSize() uint32
	StackSize() uint32
	StackUsage() uint32
	GlobalsSize() uint32
	MemorySize() uint32
	MaxMemorySize() uint32
	InitRoutine() uint16
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

// Allocate a process using the given executor.  It references an executable
// which doesn't have to have been built yet.
//
// The process is idle until its Start method is called.  Kill must be
// eventually called to release resources.
func NewProcess(ctx context.Context, e *Executor, ref ExecutableRef, debug io.Writer,
) (p *Process, err error) {
	var (
		exeImage = ref.(*file.Ref)
		inputR   *file.Ref
		inputW   *os.File
		outputR  *os.File
		outputW  *os.File
		debugR   *os.File
		debugW   *os.File
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

	osInputR, inputW, err := socketPipe()
	if err != nil {
		return
	}
	inputR = file.NewRef(osInputR)

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

	err = e.execute(ctx, &p.execution, exeImage, inputR, outputW, debugW)
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

// Start the program.  The executable must be the same that was referenced in
// NewProcess, and it must have been built by now.
//
// This function must be called before Serve, and must not be called after
// Kill.
func (p *Process) Start(exe Executable) (err error) {
	textAddr, heapAddr, stackAddr, err := readRandAddrs(exe.TextAddr())
	if err != nil {
		return
	}

	exe.SetTextAddr(textAddr)

	info := imageInfo{
		MagicNumber1:   magicNumber1,
		PageSize:       exe.PageSize(),
		TextAddr:       textAddr,
		StackAddr:      stackAddr,
		HeapAddr:       heapAddr,
		TextSize:       exe.TextSize(),
		StackSize:      exe.StackSize(),
		StackUnused:    exe.StackSize() - exe.StackUsage(),
		GlobalsSize:    exe.GlobalsSize(),
		InitMemorySize: exe.MemorySize(),
		GrowMemorySize: exe.MaxMemorySize(), // TODO: check policy too
		InitRoutine:    exe.InitRoutine(),
		DebugFlag:      0,
		MagicNumber2:   magicNumber2,
	}

	if p.debugging != nil {
		info.DebugFlag = 1
	}

	// imageInfo fits into pipe buffer so this doesn't block.
	return binary.Write(p.writer, binary.LittleEndian, &info)
}

// Serve the user program until it terminates.  Canceling the context suspends
// the program.
//
// Start must have been called before this.  This must not be called after
// Kill.
//
// The IOState object is mutated.
func (p *Process) Serve(ctx context.Context, services ServiceRegistry, ioState *IOState,
) (exit int, trapID trap.ID, err error) {
	err = ioLoop(ctx, services, p, ioState)
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

func pipe2(flags int) (r, w *os.File, err error) {
	var p [2]int

	err = syscall.Pipe2(p[:], syscall.O_CLOEXEC|flags)
	if err != nil {
		return
	}

	r = os.NewFile(uintptr(p[0]), "|0")
	w = os.NewFile(uintptr(p[1]), "|1")
	return
}

func socketPipe() (r, w *os.File, err error) {
	p, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			syscall.Close(p[0])
			syscall.Close(p[1])
		}
	}()

	err = syscall.Shutdown(p[0], syscall.SHUT_WR)
	if err != nil {
		return
	}

	err = syscall.Shutdown(p[1], syscall.SHUT_RD)
	if err != nil {
		return
	}

	r = os.NewFile(uintptr(p[0]), "|0")
	w = os.NewFile(uintptr(p[1]), "|1")
	return
}

func readRandAddrs(fixedTextAddr uint64) (textAddr, heapAddr, stackAddr uint64, err error) {
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
