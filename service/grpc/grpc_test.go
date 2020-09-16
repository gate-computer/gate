// Copyright (c) 2020 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package grpc_test

import (
	"context"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"strconv"
	"syscall"
	"testing"
	"time"

	"gate.computer/gate/packet"
	"gate.computer/gate/runtime"
	"gate.computer/gate/service"
	grpcservice "gate.computer/gate/service/grpc"
	"gate.computer/gate/service/grpc/executable"
	"google.golang.org/grpc"
)

const binary = "../../lib/gate/service/test"

func TestDial(t *testing.T)                       { testDial(t, false, false, false) }
func TestDialSuspend(t *testing.T)                { testDial(t, false, false, true) }
func TestDialRestore(t *testing.T)                { testDial(t, false, true, false) }
func TestDialRestoreSuspend(t *testing.T)         { testDial(t, false, true, true) }
func TestDialParallel(t *testing.T)               { testDial(t, true, false, false) }
func TestDialParallelSuspend(t *testing.T)        { testDial(t, true, false, true) }
func TestDialParallelRestore(t *testing.T)        { testDial(t, true, true, false) }
func TestDialParallelRestoreSuspend(t *testing.T) { testDial(t, true, true, true) }

func testDial(t *testing.T, parallel, restore, suspend bool) {
	tmp, err := ioutil.TempDir("", "*.test")
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
			if exit, ok := err.(*exec.ExitError); !(ok && exit.Success()) {
				t.Error(err)
			}
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	var c *grpcservice.Conn
	for i := 0; i < 100; i++ {
		time.Sleep(time.Millisecond * 10)
		c, err = grpcservice.DialContext(ctx, "unix:"+socket, grpc.WithInsecure())
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

	c, err := executable.Execute(ctx, binary, []string{"service"}, log.New(os.Stderr, "", 0))
	if err != nil {
		t.Fatal(err)
	}

	testServiceRepeat(t, c, c.Services, parallel, restore, suspend)
}

func testServiceRepeat(t *testing.T, c io.Closer, services []*grpcservice.Service, parallel, restore, suspend bool) {
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

func testService(ctx context.Context, t *testing.T, s *grpcservice.Service, restore, suspend bool) {
	if x := s.ServiceName(); x != "test" {
		t.Error(x)
	}
	if x := s.ServiceRevision(); x != "0" {
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

	var inst service.Instance
	if restore {
		var err error
		var snapshot []byte

		for i := 0; i < count/2; i++ {
			p := packet.MakeCall(code, 1)
			p.SetSize()
			p.Content()[0] = byte(i)

			snapshot = append(snapshot, p...)
		}

		inst, err = s.RestoreInstance(ctx, config, snapshot)
		if err != nil {
			t.Fatal(err)
		}
	} else {
		inst = s.CreateInstance(ctx, config)
	}

	done := make(chan int, 1)
	recv := make(chan packet.Buf)
	go func() {
		defer close(done)
		for i := 0; i < count; i++ {
			p := <-recv
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
		}
	}()

	func() {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		i := 0
		if restore {
			i = count / 2
		}
		for ; i < count; i++ {
			p := packet.MakeCall(code, 1)
			p.Content()[0] = byte(i)

			inst.Handle(ctx, recv, p)
		}

		<-done
	}()

	if suspend {
		if snapshot := inst.Suspend(ctx); len(snapshot) != 0 {
			t.Error(snapshot)
		}
	} else {
		inst.Shutdown(ctx)
	}

	func() {
		defer func() {
			if recover() == nil {
				t.Error("redundant instance suspension did not panic")
			}
		}()
		inst.Suspend(ctx)
	}()
}
