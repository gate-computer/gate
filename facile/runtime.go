// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package facile

import (
	"bytes"
	"context"
	"log"
	"os"

	"gate.computer/gate/internal/container"
	"gate.computer/gate/internal/defaultlog"
	"gate.computer/gate/internal/sys"
	"gate.computer/gate/runtime"
	"gate.computer/gate/service"
	"gate.computer/gate/service/origin"
	"gate.computer/gate/trap"
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
func (c *RuntimeConfig) GetLibDir() string           { return c.c.Container.LibDir }
func (c *RuntimeConfig) SetLibDir(s string)          { c.c.Container.LibDir = s }

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
		creds, err = container.ParseCreds(&config.c.Container.Namespace.User)
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
			defaultlog.StandardLogger{}.Printf("%v", err)
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
func NewRuntimeExecutor(config *RuntimeConfig, containerFD int32,
) (executor *RuntimeExecutor, err error) {
	c := config.c

	if containerFD >= 0 {
		var dupFD int

		dupFD, err = unix.FcntlInt(uintptr(containerFD), unix.F_DUPFD_CLOEXEC, 0)
		if err != nil {
			return
		}

		c.ConnFile = os.NewFile(uintptr(dupFD), "container")
		defer c.ConnFile.Close()
	}

	e, err := runtime.NewExecutor(&c)
	if err != nil {
		return
	}

	executor = &RuntimeExecutor{e}
	return
}

func (executor *RuntimeExecutor) Close() error {
	return executor.e.Close()
}

type RuntimeProcess struct {
	p *runtime.Process
}

func NewRuntimeProcess(executor *RuntimeExecutor) (process *RuntimeProcess, err error) {
	p, err := executor.e.NewProcess(context.Background())
	if err != nil {
		return
	}

	process = &RuntimeProcess{p}
	return
}

func (process *RuntimeProcess) Start(code *ProgramImage, state *InstanceImage) error {
	return process.p.Start(code.image, state.image, runtime.ProcessPolicy{
		TimeResolution: 1, // Best.
		DebugLog:       os.Stderr,
	})
}

func (process *RuntimeProcess) Serve(code *ProgramImage, state *InstanceImage) (err error) {
	connector := origin.New(&origin.Config{MaxConns: 1})

	go func() {
		defer connector.Close()

		conn := connector.Connect(context.Background())
		if conn == nil {
			return
		}

		if err := conn(context.Background(), bytes.NewReader(nil), os.Stdout); err != nil {
			log.Print(err)
		}
	}()

	var services service.Registry
	err = services.Register(connector)
	if err != nil {
		return
	}

	_, trapID, err := process.p.Serve(context.Background(), &services, &state.buffers)
	if err != nil {
		return
	}
	if trapID == trap.Exit {
		return
	}

	trace, e := state.image.Stacktrace(&code.objectMap, code.funcTypes)
	if e == nil {
		e = stacktrace.Fprint(os.Stdout, trace, code.funcTypes, nil, nil)
	}
	if e != nil {
		log.Printf("stacktrace: %v", e)
	}

	return
}

func (process *RuntimeProcess) Suspend() {
	process.p.Suspend()
}

func (process *RuntimeProcess) Kill() {
	process.p.Kill()
}
