// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"math"
	"net"
	"os/exec"
	"sync"
	"sync/atomic"
	"syscall"

	"gate.computer/internal/container"
	"gate.computer/internal/file"
	"import.name/lock"

	. "import.name/type/context"
)

var errExecutorDead = errors.New("executor died unexpectedly")

const (
	execOpCreate uint8 = iota
	execOpKill
	execOpSuspend
)

// Executor manages Process resources in an isolated environment.
type Executor struct {
	conn          *net.UnixConn
	ids           chan int16
	execRequests  chan execRequest
	killRequests  chan int16
	doneSending   chan struct{}
	doneReceiving chan struct{}

	mu    sync.Mutex
	procs map[int16]*execProcess
}

func NewExecutor(config *Config) (*Executor, error) {
	maxProcs := config.MaxProcs
	if maxProcs == 0 {
		maxProcs = MaxProcs
	}
	if maxProcs > MaxProcs {
		return nil, errors.New("executor process limit is too high")
	}

	log := config.Log
	if log == nil {
		log = slog.Default()
	}

	var (
		conn *net.UnixConn
		cmd  *exec.Cmd
		err  error
	)

	switch {
	case config.ConnFile != nil:
		c, err := net.FileConn(config.ConnFile)
		if err != nil {
			return nil, err
		}
		conn = c.(*net.UnixConn)

	case config.DaemonSocket != "":
		conn, err = dialContainerDaemon(config.DaemonSocket)
		if err != nil {
			return nil, err
		}

	default:
		cmd, conn, err = startContainer(&config.Container)
		if err != nil {
			return nil, err
		}
	}

	e := &Executor{
		conn:          conn,
		ids:           make(chan int16, maxProcs),
		execRequests:  make(chan execRequest), // No buffering.  Request must be released.
		killRequests:  make(chan int16, 16),   // TODO: how much buffering?
		doneSending:   make(chan struct{}),
		doneReceiving: make(chan struct{}),
		procs:         make(map[int16]*execProcess),
	}

	for i := 0; i < maxProcs; i++ {
		e.ids <- int16(i)
	}

	go e.sender(log)
	go e.receiver(log)

	if cmd != nil {
		go func() {
			if err := container.Wait(cmd, e.doneSending); err != nil {
				log.Error("runtime: container wait error", "error", err)
			}
		}()
	}

	return e, nil
}

// NewProcess creates a process into the default group.
func (e *Executor) NewProcess(ctx Context) (*Process, error) {
	return newProcess(ctx, e, file.Ref{})
}

// NewGroupProcess creates a process into the specified group.  If group is
// nil, the default group will be used.
func (e *Executor) NewGroupProcess(ctx Context, group *ProcessGroup) (*Process, error) {
	if group == nil {
		return e.NewProcess(ctx)
	}
	return newProcess(ctx, e, group.dir)
}

// Group returns a factory type which creates processes into the specified
// group.  If group is nil, the default group will be used.
func (e *Executor) Group(group *ProcessGroup) GroupExecutor {
	return GroupExecutor{e, group}
}

func (e *Executor) execute(ctx Context, proc *execProcess, input file.Ref, output *file.File, group file.Ref) error {
	select {
	case id, ok := <-e.ids:
		if !ok {
			return context.Canceled // TODO: ?
		}
		proc.init(e, id)

	case <-ctx.Done():
		return ctx.Err()
	}

	var (
		myInput = input.MustRef()
		myGroup = group.Ref()
		unref   = true
	)
	defer func() {
		if unref {
			myInput.Unref()
			myGroup.Unref()
		}
	}()

	select {
	case e.execRequests <- execRequest{int16(proc.id), proc, myInput, output, myGroup}:
		unref = false
		return nil

	case <-e.doneSending:
		return errExecutorDead

	case <-e.doneReceiving:
		return errExecutorDead

	case <-ctx.Done():
		return ctx.Err() // TODO: include subsystem in error
	}
}

// Close kills all processes.
func (e *Executor) Close() error {
	select {
	case e.killRequests <- math.MaxInt16:
		<-e.doneSending

	case <-e.doneSending:
		// It died on its own.
	}

	<-e.doneReceiving

	return e.conn.Close()
}

func (e *Executor) sender(log *slog.Logger) {
	var closed bool
	defer func() {
		if !closed {
			close(e.doneSending)
		}
	}()

	buf := make([]byte, 4) // sizeof(ExecRequest)

	// TODO: send multiple entries at once
	for {
		var (
			req  execRequest
			cmsg []byte
		)

		select {
		case req = <-e.execRequests:
			lock.Guard(&e.mu, func() {
				e.procs[req.pid] = req.proc
			})

			// This is like ExecRequest in runtime/executor/executor.cpp
			binary.LittleEndian.PutUint16(buf[0:], uint16(req.pid))
			buf[2] = execOpCreate

			cmsg = syscall.UnixRights(req.fds()...)

		case id := <-e.killRequests:
			if id == math.MaxInt16 {
				close(e.doneSending)
				closed = true

				if err := e.conn.CloseWrite(); err != nil {
					log.Error("runtime: executor socket write-half shutdown failed", "error", err)
				}
				return
			}

			op := execOpKill
			if id < 0 {
				id = ^id
				op = execOpSuspend
			}

			// This is like ExecRequest in runtime/executor/executor.cpp
			binary.LittleEndian.PutUint16(buf[0:], uint16(id))
			buf[2] = op
		}

		_, _, err := e.conn.WriteMsgUnix(buf, cmsg, nil)
		req.release()
		if err != nil {
			log.Error("runtime: executor socket send failed", "error", err)
			return
		}
	}
}

func (e *Executor) receiver(log *slog.Logger) {
	defer close(e.doneReceiving)

	buf := make([]byte, 512*8) // N * sizeof(ExecStatus)
	buffered := 0

	for {
		n, err := e.conn.Read(buf[buffered:])
		if err != nil {
			if err != io.EOF {
				log.Error("runtime: executor socket read failed", "error", err)
			}
			return
		}

		buffered += n
		b := buf[:buffered]

		lock.Guard(&e.mu, func() {
			for ; len(b) >= 8; b = b[8:] {
				// This is like ExecStatus in runtime/executor/executor.cpp
				var (
					id     = int16(binary.LittleEndian.Uint16(b[0:]))
					status = int32(binary.LittleEndian.Uint32(b[4:]))
				)

				p := e.procs[id]
				delete(e.procs, id)
				p.status = syscall.WaitStatus(status)
				close(p.dead)
			}
		})

		buffered = copy(buf, b)
	}
}

// Dead channel will be closed when the executor process dies.  If that wasn't
// requested by calling Close, it indicates an internal error.
func (e *Executor) Dead() <-chan struct{} {
	return e.doneReceiving
}

const (
	execProcessIDFinalized int32 = -1
	execProcessIDKilled    int32 = -2
	execProcessIDSuspended int32 = -3
)

// Low-level process, tightly coupled with Executor.  See process.go for the
// high-level Process type.
type execProcess struct {
	executor *Executor
	id       int32 // Atomic.
	dead     chan struct{}
	status   syscall.WaitStatus // Valid after dead is closed.
}

func (p *execProcess) init(e *Executor, id int16) {
	p.executor = e
	p.id = int32(id)
	p.dead = make(chan struct{})
}

func (p *execProcess) killRequested() bool {
	return atomic.LoadInt32(&p.id) == execProcessIDKilled
}

func (p *execProcess) kill()    { p.killSuspend(false, execProcessIDKilled) }
func (p *execProcess) suspend() { p.killSuspend(true, execProcessIDSuspended) }

func (p *execProcess) killSuspend(suspend bool, replacement int32) {
	n := atomic.LoadInt32(&p.id)
	if n < 0 || !atomic.CompareAndSwapInt32(&p.id, n, replacement) {
		return
	}
	id := int16(n)

	value := id
	if suspend {
		value = ^value
	}

	select {
	case p.executor.killRequests <- value:
		p.executor.ids <- id

	case <-p.executor.doneSending:
	case <-p.executor.doneReceiving:
	}
}

func (p *execProcess) finalize() (status syscall.WaitStatus, err error) {
	var (
		id           int16 = -1
		killRequests chan<- int16
	)

	if n := atomic.LoadInt32(&p.id); n >= 0 {
		if atomic.CompareAndSwapInt32(&p.id, n, execProcessIDFinalized) {
			id = int16(n)
			killRequests = p.executor.killRequests
		}
	}

	for {
		select {
		case killRequests <- id:
			killRequests = nil
			p.executor.ids <- id
			id = -1

		case <-p.executor.doneSending:
			// No way to kill it anymore.
			killRequests = nil

		case <-p.dead:
			if id >= 0 {
				p.executor.ids <- id
			}
			status = p.status
			return

		case <-p.executor.doneReceiving:
			err = errExecutorDead
			return
		}
	}
}

type execRequest struct {
	pid    int16
	proc   *execProcess
	input  file.Ref
	output *file.File
	group  file.Ref
}

func (req *execRequest) fds() []int {
	fds := make([]int, 2, 3)
	fds[0] = req.input.File().FD()
	fds[1] = req.output.FD()
	if f := req.group.File(); f != nil {
		fds = append(fds, f.FD())
	}
	return fds
}

func (req *execRequest) release() {
	if req.proc == nil {
		return
	}

	req.input.Unref()
	req.output.Close()
	req.group.Unref()
}

// GroupExecutor overrides the default process group of an executor.
type GroupExecutor struct {
	*Executor
	*ProcessGroup
}

// NewProcess creates a process into ge.ProcessGroup.
func (ge GroupExecutor) NewProcess(ctx Context) (*Process, error) {
	return ge.Executor.NewGroupProcess(ctx, ge.ProcessGroup)
}

// NewGroupProcess creates a process into the specified group.  If group is
// nil, ge.ProcessGroup will be used.
func (ge GroupExecutor) NewGroupProcess(ctx Context, group *ProcessGroup) (*Process, error) {
	if group == nil {
		group = ge.ProcessGroup
	}
	return ge.Executor.NewGroupProcess(ctx, group)
}
