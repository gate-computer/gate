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

	"github.com/tsavola/gate/image"
	internal "github.com/tsavola/gate/internal/error/runtime"
	"github.com/tsavola/gate/internal/executable"
	"github.com/tsavola/wag/object/abi"
	"github.com/tsavola/wag/trap"
)

const (
	magicNumber1 = 0x53058f3a
	magicNumber2 = 0x7e1c5d67
)

var pagesize = os.Getpagesize()

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

type ExecutableRef = executable.Ref

type InitRoutine uint16

const (
	InitStart  = InitRoutine(abi.TextAddrStart)
	InitEnter  = InitRoutine(abi.TextAddrEnter)
	InitResume = InitRoutine(abi.TextAddrResume)
)

// Process is used to execute a single program image once.
type Process struct {
	execution execProcess // Executor's low-level process state.
	writer    *os.File
	reader    *os.File
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
		exeImage = ref.(*executable.FileRef).Ref()
		inputR   *os.File
		inputW   *os.File
		outputR  *os.File
		outputW  *os.File
		debugR   *os.File
		debugW   *os.File
	)

	defer func() {
		if exeImage != nil {
			exeImage.Close()
		}
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

	err = e.execute(ctx, &p.execution, exeImage, inputR, outputW, debugW)
	if err != nil {
		return
	}

	if debug != nil {
		done := make(chan struct{})
		go copyDebug(done, debug, debugR)
		p.debugging = done
	}

	p.writer = inputW
	p.reader = outputR

	exeImage = nil
	inputR = nil
	inputW = nil
	outputR = nil
	outputW = nil
	debugR = nil
	debugW = nil
	return
}

// Start the program.  The executable must be the one that was referenced in
// NewProcess, and it must have been built by now.
//
// This function can be called before or during Serve.
func (p *Process) Start(exe *image.Executable, initRoutine InitRoutine) (err error) {
	manifest := exe.Manifest().(*executable.Manifest)

	textAddr, heapAddr, stackAddr, err := readRandAddrs()
	if err != nil {
		return
	}

	info := imageInfo{
		MagicNumber1:   magicNumber1,
		PageSize:       uint32(pagesize),
		TextAddr:       textAddr,
		StackAddr:      stackAddr,
		HeapAddr:       heapAddr,
		TextSize:       uint32(manifest.TextSize),
		StackSize:      uint32(manifest.StackSize),
		StackUnused:    uint32(manifest.StackUnused),
		GlobalsSize:    uint32(manifest.GlobalsSize),
		InitMemorySize: uint32(manifest.MemorySize),
		GrowMemorySize: uint32(manifest.MaxMemorySize),
		InitRoutine:    uint16(initRoutine),
		DebugFlag:      0,
		MagicNumber2:   magicNumber2,
	}

	if p.debugging != nil {
		info.DebugFlag = 1
	}

	// imageInfo fits into pipe buffer so this doesn't block.
	err = binary.Write(p.writer, binary.LittleEndian, &info)
	if err != nil {
		return
	}

	return
}

// Serve the user program until it terminates.
//
// Nothing happens until Start is called, so this can be called even before
// the executable is built.
func (p *Process) Serve(ctx context.Context, services ServiceRegistry,
) (exit int, trapID trap.ID, err error) {
	err = ioLoop(ctx, services, p)
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
		err = fmt.Errorf("process termination signal: %d", status.Signal())
		return

	default:
		err = fmt.Errorf("unknown process status: %d", status)
		return
	}
}

// Kill the process.  Serve call will return.  Can be called multiple times.
func (p *Process) Kill() {
	if p.writer == nil {
		return
	}

	p.execution.kill(false)
	p.writer.Close()
	p.reader.Close()

	p.writer = nil
	p.reader = nil
}

func (p *Process) suspend() {
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

func readRandAddrs() (textAddr, heapAddr, stackAddr uint64, err error) {
	b := make([]byte, 12)

	_, err = rand.Read(b)
	if err != nil {
		return
	}

	textAddr = randAddr(executable.MinTextAddr, executable.MaxTextAddr, b[0:4])
	heapAddr = randAddr(executable.MinHeapAddr, executable.MaxHeapAddr, b[4:8])
	stackAddr = randAddr(executable.MinStackAddr, executable.MaxStackAddr, b[8:12])
	return
}

func randAddr(minAddr, maxAddr uint64, b []byte) uint64 {
	minPage := minAddr / uint64(pagesize)
	maxPage := maxAddr / uint64(pagesize)
	page := minPage + uint64(binary.LittleEndian.Uint32(b))%(maxPage-minPage)
	return page * uint64(pagesize)
}

var _ struct{} = internal.ErrorsInitialized
