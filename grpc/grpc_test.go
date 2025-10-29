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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	. "import.name/testing/mustr"
	. "import.name/type/context"
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
	tmp := Must(t, R(os.MkdirTemp("", "*.test")))
	defer os.RemoveAll(tmp)

	socket := path.Join(tmp, t.Name()+".sock")

	cmd := exec.Command(binary, "-net", "unix", "-addr", socket)
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start())
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
	var err error
	for range 100 {
		time.Sleep(time.Millisecond * 10)
		c, err = client.NewClient(ctx, "unix:"+socket, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err == nil {
			break
		}
	}
	require.NoError(t, err)

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

	c := Must(t, R(executable.Execute(ctx, binary, []string{"service"}, nil)))
	testServiceRepeat(t, c, c.Services, parallel, restore, suspend)
}

func testServiceRepeat(t *testing.T, c io.Closer, services []*client.Service, parallel, restore, suspend bool) {
	defer func() {
		assert.NoError(t, c.Close())
	}()

	t.Run("#", func(t *testing.T) {
		ctxs := []Context{
			runtime.ContextWithDummyProcessKey(context.Background()),
			runtime.ContextWithDummyProcessKey(context.Background()),
			runtime.ContextWithDummyProcessKey(context.Background()),
		}

		for i := range 10 {
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

func testService(ctx Context, t *testing.T, s *client.Service, restore, suspend bool) {
	assert.Equal(t, s.Properties().Service, service.Service{
		Name:     "test",
		Revision: "0",
	})

	const code = packet.Code(1234)
	const count = 100

	config := service.InstanceConfig{
		Service: packet.Service{
			MaxSendSize: 65536,
			Code:        code,
		},
	}

	var snapshot []byte
	if restore {
		for i := range count / 2 {
			p := packet.MakeCall(code, 1)
			p.SetSize()
			p.Content()[0] = byte(i)
			snapshot = append(snapshot, p...)
		}
	}

	inst := Must(t, R(s.CreateInstance(ctx, config, snapshot)))
	require.NoError(t, inst.Ready(ctx))

	done := make(chan int, 1)
	recv := make(chan packet.Thunk)
	go func() {
		defer close(done)
		for i := 0; i < count; {
			thunk := <-recv
			if p, err := thunk(); err != nil {
				t.Error(err)
			} else if len(p) > 0 {
				assert.Equal(t, p.Domain(), packet.DomainCall)
				assert.Equal(t, p.Code(), code)
				assert.Equal(t, len(p.Content()), 1)
				assert.Equal(t, int(p.Content()[0]), i)
				i++
			}
		}
	}()

	func() {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		require.NoError(t, inst.Start(ctx, recv, func(err error) { t.Fatal(err) }))

		i := 0
		if restore {
			i = count / 2
		}
		for ; i < count; i++ {
			p := packet.MakeCall(code, 1)
			p.Content()[0] = byte(i)

			reply := Must(t, R(inst.Handle(ctx, recv, p)))
			require.Empty(t, reply, "not implemented")
		}

		<-done
	}()

	snapshot = Must(t, R(inst.Shutdown(ctx, suspend)))
	require.Empty(t, snapshot)

	_, err := inst.Shutdown(ctx, true)
	assert.Error(t, err, "redundant instance suspension did not fail")
}
