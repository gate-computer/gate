// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package facile

import (
	"bytes"
	"context"
	"log/slog"
	"os"

	"gate.computer/gate/runtime"
	"gate.computer/gate/service"
	"gate.computer/gate/service/origin"
	"gate.computer/gate/trap"
	"gate.computer/internal/container"
	"gate.computer/internal/sys"
	"gate.computer/wag/object/stack/stacktrace"
	"golang.org/x/sys/unix"
)

type RuntimeConfig struct {
	c runtime.Config
}

func NewRuntimeConfig() *RuntimeConfig {
	return &RuntimeConfig{runtime.DefaultConfig}
}

func (c *RuntimeConfig) GetMaxProcs() int32          { return int32(c.c.MaxProcs) }
func (c *RuntimeConfig) SetMaxProcs(n int32)         { c.c.MaxProcs = int(n) }
func (c *RuntimeConfig) GetNamespaceDisabled() bool  { return c.c.Container.Namespace.Disabled }
func (c *RuntimeConfig) SetNamespaceDisabled(b bool) { c.c.Container.Namespace.Disabled = b }
func (c *RuntimeConfig) GetExecDir() string          { return c.c.Container.ExecDir }
func (c *RuntimeConfig) SetExecDir(s string)         { c.c.Container.ExecDir = s }

type RuntimeContainer struct {
	conn *os.File
}

// NewRuntimeContainer.
//
// The MaxProcs config value has no effect here.
func NewRuntimeContainer(binary string, config *RuntimeConfig) (*RuntimeContainer, error) {
	var creds *container.NamespaceCreds
	var err error

	if !config.c.Container.Namespace.Disabled {
		creds, err = container.ParseCreds(&config.c.Container.Namespace)
		if err != nil {
			return nil, err
		}
	}

	control, conn, err := sys.SocketFilePair(0)
	if err != nil {
		return nil, err
	}
	defer control.Close()

	cmd, err := container.Start(control, &config.c.Container, creds)
	if err != nil {
		conn.Close()
		return nil, err
	}

	go func() {
		if err := container.Wait(cmd, nil); err != nil {
			slog.Error("container wait failed", "error", err)
		}
	}()

	return &RuntimeContainer{conn}, nil
}

func (x *RuntimeContainer) GetFD() int32 {
	return int32(x.conn.Fd())
}

func (x *RuntimeContainer) CloseFD() error {
	return x.conn.Close()
}

type RuntimeExecutor struct {
	e *runtime.Executor
}

// NewRuntimeExecutor duplicates the container file descriptor (if any).
func NewRuntimeExecutor(config *RuntimeConfig, containerFD int32) (*RuntimeExecutor, error) {
	c := config.c

	if containerFD >= 0 {
		dupFD, err := unix.FcntlInt(uintptr(containerFD), unix.F_DUPFD_CLOEXEC, 0)
		if err != nil {
			return nil, err
		}

		c.ConnFile = os.NewFile(uintptr(dupFD), "container")
		defer c.ConnFile.Close()
	}

	e, err := runtime.NewExecutor(&c)
	if err != nil {
		return nil, err
	}

	return &RuntimeExecutor{e}, nil
}

func (executor *RuntimeExecutor) Close() error {
	return executor.e.Close()
}

type RuntimeProcess struct {
	p *runtime.Process
}

func NewRuntimeProcess(executor *RuntimeExecutor) (*RuntimeProcess, error) {
	p, err := executor.e.NewProcess(context.Background())
	if err != nil {
		return nil, err
	}

	return &RuntimeProcess{p}, nil
}

func (process *RuntimeProcess) Start(code *ProgramImage, state *InstanceImage) error {
	return process.p.Start(code.image, state.image, runtime.ProcessPolicy{
		TimeResolution: 1, // Best.
		DebugLog:       os.Stderr,
	})
}

func (process *RuntimeProcess) Serve(code *ProgramImage, state *InstanceImage) error {
	connector := origin.New(&origin.Config{MaxConns: 1})

	go func() {
		defer connector.Close()

		conn := connector.Connect(context.Background())
		if conn == nil {
			return
		}

		if err := conn(context.Background(), bytes.NewReader(nil), os.Stdout); err != nil {
			slog.Error("origin connection failed", "error", err)
		}
	}()

	var services service.Registry
	if err := services.Register(connector); err != nil {
		return err
	}

	var (
		trapID trap.ID
		err    error
	)

	_, trapID, state.buffers, err = process.p.Serve(context.Background(), &services, state.buffers)
	if err != nil {
		return err
	}
	if trapID == trap.Exit {
		return nil
	}

	trace, err := state.image.Stacktrace(&code.objectMap, code.funcTypes)
	if err == nil {
		err = stacktrace.Fprint(os.Stdout, trace, code.funcTypes, nil, nil)
	}
	if err != nil {
		slog.Error("stacktrace failed", "error", err)
	}

	return nil
}

func (process *RuntimeProcess) Suspend() {
	process.p.Suspend()
}

func (process *RuntimeProcess) Kill() {
	process.p.Kill()
}
