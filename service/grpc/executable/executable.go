// Copyright (c) 2020 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package executable

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"syscall"
	"time"

	"gate.computer/gate/internal/sys"
	grpcservice "gate.computer/gate/service/grpc"
	"google.golang.org/grpc"
)

// Conn is a connection to a process.
type Conn struct {
	*grpcservice.Conn

	cmd  *exec.Cmd
	done <-chan struct{}
}

// Execute a program.  Args includes the command name.
func Execute(ctx context.Context, path string, args []string, log Logger) (*Conn, error) {
	sock1, sock2, err := sys.SocketFilePair(0)
	if err != nil {
		return nil, err
	}
	defer sock1.Close()
	defer sock2.Close()

	cmd := exec.CommandContext(ctx, path)
	cmd.Args = args
	cmd.ExtraFiles = []*os.File{sock1}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	defer func() {
		if stderr != nil {
			stderr.Close()
		}
	}()

	if err := cmd.Start(); err != nil {
		return nil, err
	}
	defer func() {
		if cmd != nil {
			if cmd.Process.Signal(syscall.SIGTERM) == nil {
				cmd.Wait()
			}
		}
	}()

	conn, err := grpcservice.DialContext(ctx, "socket", grpc.WithDialer(dialerFor(sock2)), grpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	done := make(chan struct{})

	go logErrorOutput(stderr, log, args[0], done)
	stderr = nil

	go terminateWhenDone(ctx, cmd.Process)

	c := &Conn{conn, cmd, done}
	cmd = nil
	return c, nil
}

// Close terminates the process.
func (c *Conn) Close() error {
	errClose := c.Conn.Close()
	if errClose != nil {
		c.cmd.Process.Signal(syscall.SIGTERM)
	}
	errWait := c.cmd.Wait()
	<-c.done
	if errClose != nil {
		return errClose
	}
	return errWait
}

func dialerFor(conn *os.File) func(string, time.Duration) (net.Conn, error) {
	dialed := false

	return func(string, time.Duration) (net.Conn, error) {
		if dialed {
			return nil, errors.New("reconnection not supported")
		}
		dialed = true
		return net.FileConn(conn)
	}
}

func logErrorOutput(r io.ReadCloser, l Logger, name string, done chan<- struct{}) {
	defer close(done)
	defer r.Close()

	br := bufio.NewReader(r)
	for {
		s, err := br.ReadString('\n')
		if err != nil {
			break
		}
		l.Printf("%s: %s", name, s)
	}
}

func terminateWhenDone(ctx context.Context, p *os.Process) {
	defer p.Signal(syscall.SIGTERM)
	<-ctx.Done()
}
