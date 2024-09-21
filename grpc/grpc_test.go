// Copyright (c) 2020 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package grpc_test

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path"
	goruntime "runtime"
	"strconv"
	"syscall"
	"testing"
	"time"

	"gate.computer/gate/packet"
	"gate.computer/gate/runtime"
	"gate.computer/gate/service"
	"gate.computer/grpc/client"
	"gate.computer/grpc/executable"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var binary = path.Join("../lib", goruntime.GOARCH, "test-grpc-service")

func TestDial(t *testing.T)                       { testDial(t, false, false, false) }
func TestDialSuspend(t *testing.T)                { testDial(t, false, false, true) }
func TestDialRestore(t *testing.T)                { testDial(t, false, true, false) }
func TestDialRestoreSuspend(t *testing.T)         { testDial(t, false, true, true) }
func TestDialParallel(t *testing.T)               { testDial(t, true, false, false) }
func TestDialParallelSuspend(t *testing.T)        { testDial(t, true, false, true) }
func TestDialParallelRestore(t *testing.T)        { testDial(t, true, true, false) }
func TestDialParallelRestoreSuspend(t *testing.T) { testDial(t, true, true, true) }

func testDial(t *testing.T, parallel, restore, suspend bool) {
	tmp, err := os.MkdirTemp("", "*.test")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmp)

	socket := path.Join(tmp, t.Name()+".sock")

	cmd := exec.Command(binary, "-net", "unix", "-addr", socket)
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
			t.Error(err)
			return
		}
		if err := cmd.Wait(); err != nil {
			var exit *exec.ExitError
			if !errors.As(err, &exit) || !exit.Success() {
				t.Error(err)
			}
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	var c *client.Conn
	for i := 0; i < 100; i++ {
		time.Sleep(time.Millisecond * 10)
		c, err = client.NewClient(ctx, "unix:"+socket, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err == nil {
			break
		}
	}
	if err != nil {
		t.Fatal(err)
	}

	testServiceRepeat(t, c, c.Services, parallel, restore, suspend)
}

func TestExecutable(t *testing.T)                       { testExecutable(t, false, false, false) }
func TestExecutableSuspend(t *testing.T)                { testExecutable(t, false, false, true) }
func TestExecutableRestore(t *testing.T)                { testExecutable(t, false, true, false) }
func TestExecutableRestoreSuspend(t *testing.T)         { testExecutable(t, false, true, true) }
func TestExecutableParallel(t *testing.T)               { testExecutable(t, true, false, false) }
func TestExecutableParallelSuspend(t *testing.T)        { testExecutable(t, true, false, true) }
func TestExecutableParallelRestore(t *testing.T)        { testExecutable(t, true, true, false) }
func TestExecutableParallelRestoreSuspend(t *testing.T) { testExecutable(t, true, true, true) }

func testExecutable(t *testing.T, parallel, restore, suspend bool) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	c, err := executable.Execute(ctx, binary, []string{"service"})
	if err != nil {
		t.Fatal(err)
	}

	testServiceRepeat(t, c, c.Services, parallel, restore, suspend)
}

func testServiceRepeat(t *testing.T, c io.Closer, services []*client.Service, parallel, restore, suspend bool) {
	defer func() {
		if err := c.Close(); err != nil {
			t.Error(err)
		}
	}()

	t.Run("#", func(t *testing.T) {
		ctxs := []context.Context{
			runtime.ContextWithDummyProcessKey(context.Background()),
			runtime.ContextWithDummyProcessKey(context.Background()),
			runtime.ContextWithDummyProcessKey(context.Background()),
		}

		for i := 0; i < 10; i++ {
			ctx := ctxs[i%len(ctxs)]

			t.Run(strconv.Itoa(i), func(t *testing.T) {
				if parallel {
					t.Parallel()
				}

				for _, s := range services {
					testService(ctx, t, s, restore, suspend)
				}
			})
		}
	})
}

func testService(ctx context.Context, t *testing.T, s *client.Service, restore, suspend bool) {
	if x := s.Properties().Service.Name; x != "test" {
		t.Error(x)
	}
	if x := s.Properties().Service.Revision; x != "0" {
		t.Error(x)
	}

	const code = 1234
	const count = 100

	config := service.InstanceConfig{
		Service: packet.Service{
			MaxSendSize: 65536,
			Code:        code,
		},
	}

	var snapshot []byte
	if restore {
		for i := 0; i < count/2; i++ {
			p := packet.MakeCall(code, 1)
			p.SetSize()
			p.Content()[0] = byte(i)
			snapshot = append(snapshot, p...)
		}
	}

	inst, err := s.CreateInstance(ctx, config, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := inst.Ready(ctx); err != nil {
		t.Fatal(err)
	}

	done := make(chan int, 1)
	recv := make(chan packet.Thunk)
	go func() {
		defer close(done)
		for i := 0; i < count; {
			thunk := <-recv
			if p, err := thunk(); err != nil {
				t.Error(err)
			} else if len(p) > 0 {
				if x := p.Domain(); x != packet.DomainCall {
					t.Error(x)
				}
				if x := p.Code(); x != code {
					t.Error(x)
				}
				if x := len(p.Content()); x != 1 {
					t.Error(x)
				}
				if x := int(p.Content()[0]); x != i {
					t.Error(x)
				}
				i++
			}
		}
	}()

	func() {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		if err := inst.Start(ctx, recv, func(e error) { t.Fatal(e) }); err != nil {
			t.Fatal(err)
		}

		i := 0
		if restore {
			i = count / 2
		}
		for ; i < count; i++ {
			p := packet.MakeCall(code, 1)
			p.Content()[0] = byte(i)

			reply, err := inst.Handle(ctx, recv, p)
			if err != nil {
				t.Fatal(err)
			}
			if len(reply) > 0 {
				t.Fatal(reply) // Not implemented.
			}
		}

		<-done
	}()

	if suspend {
		if snapshot, err := inst.Shutdown(ctx, true); err != nil {
			t.Error(err)
		} else if len(snapshot) != 0 {
			t.Error(snapshot)
		}
	} else {
		if _, err := inst.Shutdown(ctx, false); err != nil {
			t.Error(err)
		}
	}

	if _, err := inst.Shutdown(ctx, true); err == nil {
		t.Error("redundant instance suspension did not fail")
	}
}
