// Copyright (c) 2020 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package executable

import (
	"bufio"
	"errors"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"syscall"

	"gate.computer/grpc/client"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	. "import.name/type/context"
)

// Conn is a connection to a process.
type Conn struct {
	*client.Conn

	cmd  *exec.Cmd
	done <-chan struct{}
}

// Execute a program.  Args includes the command name.
func Execute(ctx Context, path string, args []string) (*Conn, error) {
	sock1, sock2, err := socketFilePair(0)
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

	conn, err := client.NewClient(ctx, "0.0.0.0", grpc.WithContextDialer(dialerFor(sock2)), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	done := make(chan struct{})

	go logErrorOutput(stderr, args[0], done)
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

func dialerFor(conn *os.File) func(Context, string) (net.Conn, error) {
	dialed := false

	return func(Context, string) (net.Conn, error) {
		if dialed {
			return nil, errors.New("reconnection not supported")
		}
		dialed = true
		return net.FileConn(conn)
	}
}

func logErrorOutput(r io.ReadCloser, name string, done chan<- struct{}) {
	defer close(done)
	defer r.Close()

	br := bufio.NewReader(r)
	for {
		s, err := br.ReadString('\n')
		if err != nil {
			break
		}
		log.Printf("%s: %s", name, s)
	}
}

func terminateWhenDone(ctx Context, p *os.Process) {
	defer p.Signal(syscall.SIGTERM)
	<-ctx.Done()
}
