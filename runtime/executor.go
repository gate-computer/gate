// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/tsavola/gate/internal/defaultlog"
	"github.com/tsavola/gate/internal/file"
)

var errExecutorDead = errors.New("executor died unexpectedly")

// recvEntry is like send_entry in runtime/executor/executor.c
type recvEntry struct {
	Pid    int32 // pid_t
	Status int32
}

// Executor manages Process resources in an isolated environment.
type Executor struct {
	conn            *net.UnixConn
	execRequests    chan execRequest
	killRequests    chan int32
	doneSending     chan struct{}
	doneReceiving   chan struct{}
	maxProcs        int64
	numProcs        int64 // Atomic
	numProcsChanged chan struct{}

	lock    sync.Mutex
	pending []*execProcess
}

func NewExecutor(ctx context.Context, config *Config) (e *Executor, err error) {
	errorLog := config.ErrorLog
	if errorLog == nil {
		errorLog = defaultlog.StandardLogger{}
	}

	var (
		conn *net.UnixConn
		cmd  *exec.Cmd
	)

	if config.DaemonSocket != "" {
		conn, err = dialContainerDaemon(ctx, config)
	} else {
		cmd, conn, err = startContainer(ctx, config)
	}
	if err != nil {
		return
	}

	e = &Executor{
		conn:            conn,
		execRequests:    make(chan execRequest), // No buffering.  Request must be released.
		killRequests:    make(chan int32, 16),   // TODO: how much buffering?
		doneSending:     make(chan struct{}),
		doneReceiving:   make(chan struct{}),
		maxProcs:        config.maxProcs(),
		numProcsChanged: make(chan struct{}, 1),
	}

	go e.sender(errorLog)
	go e.receiver(errorLog)

	if cmd != nil {
		go containerWaiter(cmd, e.doneSending, errorLog)
	}

	return
}

func (e *Executor) execute(ctx context.Context, proc *execProcess, image, input *file.Ref, output, debug *os.File,
) error {
	proc.init(e)

	image.Ref()
	input.Ref()
	defer func() {
		if image != nil {
			image.Close()
		}
		if input != nil {
			input.Close()
		}
	}()

	select {
	case e.execRequests <- execRequest{proc, image, input, output, debug}:
		image = nil
		input = nil
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
	case e.killRequests <- 0:
		<-e.doneSending

	case <-e.doneSending:
		// It died on its own.
	}

	<-e.doneReceiving

	return e.conn.Close()
}

func (e *Executor) sender(errorLog Logger) {
	var closed bool

	defer func() {
		if !closed {
			close(e.doneSending)
		}
	}()

	var numProcs int64

	for {
		var (
			execRequests <-chan execRequest
			execReq      execRequest
			cmsg         []byte
			buf          = make([]byte, 4) // sizeof (pid_t)
		)

		if numProcs < e.maxProcs {
			execRequests = e.execRequests
		}

		select {
		case <-e.numProcsChanged:
			numProcs = atomic.LoadInt64(&e.numProcs)
			continue

		case execReq = <-execRequests:
			e.lock.Lock()
			e.pending = append(e.pending, execReq.proc)
			e.lock.Unlock()

			numProcs++ // Conservative estimate.

			cmsg = syscall.UnixRights(execReq.fds()...)

		case pid := <-e.killRequests:
			if pid == 0 {
				close(e.doneSending)
				closed = true

				if err := e.conn.CloseWrite(); err != nil {
					errorLog.Printf("executor socket: %v", err)
				}
				return
			}

			binary.LittleEndian.PutUint32(buf, uint32(pid)) // sizeof (pid_t)
		}

		_, _, err := e.conn.WriteMsgUnix(buf, cmsg, nil)
		execReq.release()
		if err != nil {
			errorLog.Printf("executor socket: %v", err)
			return
		}
	}
}

func (e *Executor) receiver(errorLog Logger) {
	defer close(e.doneReceiving)

	r := bufio.NewReader(e.conn)
	running := make(map[int32]*execProcess)

	var buf recvEntry

	for {
		if err := binary.Read(r, binary.LittleEndian, &buf); err != nil {
			if err != io.EOF {
				errorLog.Printf("executor socket: %v", err)
			}
			return
		}

		var proc *execProcess

		if buf.Pid < 0 {
			e.lock.Lock()
			proc = e.pending[0]
			e.pending = e.pending[1:]
			e.lock.Unlock()

			running[-buf.Pid] = proc
		} else {
			proc = running[buf.Pid]
			delete(running, buf.Pid)

			e.lock.Lock()
			pendingLen := len(e.pending)
			e.lock.Unlock()

			atomic.StoreInt64(&e.numProcs, int64(pendingLen+len(running)))

			select {
			case e.numProcsChanged <- struct{}{}:
			default:
			}
		}

		proc.events <- buf
	}
}

// Dead channel will be closed when the executor process dies.  If that wasn't
// requested by calling Close, it indicates an internal error.
func (e *Executor) Dead() <-chan struct{} {
	return e.doneReceiving
}

// Low-level process, tightly coupled with Executor.  See process.go for the
// high-level Process type.
type execProcess struct {
	executor *Executor
	events   chan recvEntry
	pid      int32 // In another pid namespace.
	killed   bool
}

func (p *execProcess) init(e *Executor) {
	p.executor = e
	p.events = make(chan recvEntry, 2) // Space for reply and status.
}

func (p *execProcess) initPid() {
	if p.pid == 0 {
		entry := <-p.events
		p.pid = -entry.Pid
	}
}

func (p *execProcess) kill(suspend bool) {
	if p.killed {
		return
	}

	p.initPid()

	value := p.pid
	if suspend {
		value = -value
	}

	select {
	case p.executor.killRequests <- value:
		p.killed = true

	case <-p.executor.doneSending:

	case <-p.executor.doneReceiving:
	}
}

func (p *execProcess) killWait() (status syscall.WaitStatus, err error) {
	p.initPid()

	var killRequests chan<- int32
	if !p.killed {
		killRequests = p.executor.killRequests
	}

	for {
		select {
		case killRequests <- p.pid:
			killRequests = nil
			p.killed = true

		case <-p.executor.doneSending:
			// No way to kill it anymore.
			killRequests = nil

		case entry := <-p.events:
			status = syscall.WaitStatus(entry.Status)
			return

		case <-p.executor.doneReceiving:
			err = errExecutorDead
			return
		}
	}
}

type execRequest struct {
	proc   *execProcess
	image  *file.Ref
	input  *file.Ref
	output *os.File
	debug  *os.File // Optional
}

func (req *execRequest) fds() (fds []int) {
	if req.debug == nil {
		fds = make([]int, 3)
	} else {
		fds = make([]int, 4)
	}

	fds[0] = int(req.input.Fd())
	fds[1] = int(req.output.Fd())
	fds[2] = int(req.image.Fd())

	if req.debug != nil {
		fds[3] = int(req.debug.Fd())
	}
	return
}

func (req *execRequest) release() {
	if req.proc == nil {
		return
	}

	req.image.Close()
	req.input.Close()
	req.output.Close()

	if req.debug != nil {
		req.debug.Close()
	}
}
