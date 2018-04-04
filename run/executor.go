// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package run

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"os/exec"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/tsavola/gate/internal/defaultlog"
	"github.com/tsavola/gate/internal/publicerror"
)

var errExecutorDead = errors.New("executor died unexpectedly")

// recvEntry is like send_entry in executor.c
type recvEntry struct {
	Pid    int32 // pid_t
	Status int32
}

type execRequest struct {
	p     *process
	files *execFiles
}

type executor struct {
	limiter         FileLimiter
	conn            *net.UnixConn
	execRequests    chan execRequest
	killRequests    chan int32
	doneSending     chan struct{}
	doneReceiving   chan struct{}
	maxProcs        int64
	numProcs        int64 // atomic
	numProcsChanged chan struct{}
	lock            sync.Mutex
	pending         []*process
}

func (e *executor) init(ctx context.Context, config *Config) (err error) {
	errorLog := config.ErrorLog
	if errorLog == nil {
		errorLog = defaultlog.StandardLogger{}
	}

	if config.FileLimiter != nil {
		e.limiter = *config.FileLimiter
	}

	var (
		conn *net.UnixConn
		cmd  *exec.Cmd
	)

	if config.DaemonSocket != "" {
		conn, err = dialContainerDaemon(ctx, e.limiter, config)
	} else {
		cmd, conn, err = startContainer(ctx, e.limiter, config)
	}
	if err != nil {
		return
	}

	e.conn = conn
	e.execRequests = make(chan execRequest) // no buffering; files must be closed
	e.killRequests = make(chan int32, 16)   // TODO: how much buffering?
	e.doneSending = make(chan struct{})
	e.doneReceiving = make(chan struct{})
	e.maxProcs = config.maxProcs()
	e.numProcsChanged = make(chan struct{}, 1)

	go e.sender(errorLog)
	go e.receiver(errorLog)

	if cmd != nil {
		go containerWaiter(cmd, e.doneSending, errorLog)
	}

	return
}

func (e *executor) execute(ctx context.Context, p *process, files *execFiles,
) error {
	p.init(e)

	select {
	case e.execRequests <- execRequest{p, files}:
		return nil

	case <-e.doneSending:
		return errExecutorDead

	case <-e.doneReceiving:
		return errExecutorDead

	case <-ctx.Done():
		return publicerror.Shutdown("executor", ctx.Err())
	}
}

func (e *executor) close() error {
	select {
	case e.killRequests <- 0:
		<-e.doneSending

	case <-e.doneSending:
		// it died on its own
	}

	<-e.doneReceiving

	return e.conn.Close()
}

func (e *executor) sender(errorLog Logger) {
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
			buf          = make([]byte, 4) // sizeof (pid_t)
			files        *execFiles
			cmsg         []byte
		)

		if numProcs < e.maxProcs {
			execRequests = e.execRequests
		}

		select {
		case <-e.numProcsChanged:
			numProcs = atomic.LoadInt64(&e.numProcs)
			continue

		case exec := <-execRequests:
			e.lock.Lock()
			e.pending = append(e.pending, exec.p)
			e.lock.Unlock()

			numProcs++ // conservative estimate

			files = exec.files
			cmsg = syscall.UnixRights(files.fds()...)

		case pid := <-e.killRequests:
			if pid == 0 {
				close(e.doneSending)
				closed = true

				if err := e.conn.CloseWrite(); err != nil {
					errorLog.Printf("executor socket: %v", err)
				}
				return
			}

			endian.PutUint32(buf, uint32(pid)) // sizeof (pid_t)
		}

		_, _, err := e.conn.WriteMsgUnix(buf, cmsg, nil)
		if files != nil {
			files.release(e.limiter)
		}
		if err != nil {
			errorLog.Printf("executor socket: %v", err)
			return
		}
	}
}

func (e *executor) receiver(errorLog Logger) {
	defer close(e.doneReceiving)

	r := bufio.NewReader(e.conn)
	running := make(map[int32]*process)

	var buf recvEntry

	for {
		if err := binary.Read(r, endian, &buf); err != nil {
			if err != io.EOF {
				errorLog.Printf("executor socket: %v", err)
			}
			return
		}

		var p *process

		if buf.Pid < 0 {
			e.lock.Lock()
			p = e.pending[0]
			e.pending = e.pending[1:]
			e.lock.Unlock()

			running[-buf.Pid] = p
		} else {
			p = running[buf.Pid]
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

		p.events <- buf
	}
}

type process struct {
	e      *executor
	events chan recvEntry
	pid    int32 // in another namespace
	killed bool
}

func (p *process) init(e *executor) {
	p.e = e
	p.events = make(chan recvEntry, 2) // space for reply and status
}

func (p *process) initPid() {
	if p.pid == 0 {
		entry := <-p.events
		p.pid = -entry.Pid
	}
}

func (p *process) kill() {
	if p.killed {
		return
	}

	p.initPid()

	select {
	case p.e.killRequests <- p.pid:
		p.killed = true

	case <-p.e.doneSending:

	case <-p.e.doneReceiving:
	}
}

func (p *process) killWait() (status syscall.WaitStatus, err error) {
	p.initPid()

	var killRequests chan<- int32
	if !p.killed {
		killRequests = p.e.killRequests
	}

	for {
		select {
		case killRequests <- p.pid:
			killRequests = nil
			p.killed = true

		case <-p.e.doneSending:
			// no way to kill it anymore
			killRequests = nil

		case entry := <-p.events:
			status = syscall.WaitStatus(entry.Status)
			return

		case <-p.e.doneReceiving:
			err = errExecutorDead
			return
		}
	}
}
