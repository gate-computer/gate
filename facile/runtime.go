// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package facile

import (
	"bytes"
	"context"
	"log"
	"os"

	"github.com/tsavola/gate/internal/defaultlog"
	"github.com/tsavola/gate/internal/runtimeapi"
	"github.com/tsavola/gate/internal/sys"
	"github.com/tsavola/gate/runtime"
	"github.com/tsavola/gate/service"
	"github.com/tsavola/gate/service/origin"
	"github.com/tsavola/gate/trap"
	"github.com/tsavola/wag/object/stack/stacktrace"
	"golang.org/x/sys/unix"
)

type RuntimeConfig struct {
	c runtime.Config
}

func NewRuntimeConfig() (c *RuntimeConfig) {
	uid := uint(os.Getuid())
	gid := uint(os.Getgid())

	c = &RuntimeConfig{runtime.DefaultConfig}
	c.c.Container.Cred.UID = uid
	c.c.Container.Cred.GID = gid
	c.c.Executor.Cred.UID = uid
	c.c.Executor.Cred.GID = gid
	return
}

func (c *RuntimeConfig) GetMaxProcs() int32      { return int32(c.c.MaxProcs) }
func (c *RuntimeConfig) SetMaxProcs(n int32)     { c.c.MaxProcs = int(n) }
func (c *RuntimeConfig) GetNoNamespaces() bool   { return c.c.NoNamespaces }
func (c *RuntimeConfig) SetNoNamespaces(b bool)  { c.c.NoNamespaces = b }
func (c *RuntimeConfig) GetContainerUID() int64  { return int64(c.c.Container.Cred.UID) }
func (c *RuntimeConfig) SetContainerUID(n int64) { c.c.Container.Cred.UID = uint(n) }
func (c *RuntimeConfig) GetContainerGID() int64  { return int64(c.c.Container.Cred.GID) }
func (c *RuntimeConfig) SetContainerGID(n int64) { c.c.Container.Cred.GID = uint(n) }
func (c *RuntimeConfig) GetExecutorUID() int64   { return int64(c.c.Executor.Cred.UID) }
func (c *RuntimeConfig) SetExecutorUID(n int64)  { c.c.Executor.Cred.UID = uint(n) }
func (c *RuntimeConfig) GetExecutorGID() int64   { return int64(c.c.Executor.Cred.GID) }
func (c *RuntimeConfig) SetExecutorGID(n int64)  { c.c.Executor.Cred.GID = uint(n) }
func (c *RuntimeConfig) GetLibDir() string       { return c.c.LibDir }
func (c *RuntimeConfig) SetLibDir(s string)      { c.c.LibDir = s }

type RuntimeContainer struct {
	conn *os.File
}

// NewRuntimeContainer.
//
// The MaxProcs config value has no effect here.
func NewRuntimeContainer(binary string, config *RuntimeConfig) (container *RuntimeContainer, err error) {
	args, err := runtimeapi.ContainerArgs(binary, config.c.NoNamespaces, config.c.Container.Cred, config.c.Executor.Cred, config.c.Cgroup.Title, config.c.Cgroup.Parent)
	if err != nil {
		return
	}

	control, conn, err := sys.SocketFilePair(0)
	if err != nil {
		return
	}
	defer control.Close()

	cmd, err := runtimeapi.StartContainer(args, control)
	if err != nil {
		conn.Close()
		return
	}

	go func() {
		if err := runtimeapi.WaitForContainer(cmd, nil); err != nil {
			defaultlog.StandardLogger{}.Printf("%v", err)
		}
	}()

	container = &RuntimeContainer{conn}
	return
}

func (container *RuntimeContainer) GetFD() int32 {
	return int32(container.conn.Fd())
}

func (container *RuntimeContainer) CloseFD() error {
	return container.conn.Close()
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

	e, err := runtime.NewExecutor(c)
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
		Debug:          os.Stderr,
	})
}

func (process *RuntimeProcess) Serve(code *ProgramImage, state *InstanceImage) (err error) {
	connector := origin.New(origin.Config{MaxConns: 1})

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
	services.Register(connector)

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
