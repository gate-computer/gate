// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/tsavola/gate/image"
	"github.com/tsavola/gate/internal/error/badprogram"
	"github.com/tsavola/gate/internal/error/public"
	"github.com/tsavola/gate/internal/error/subsystem"
	"github.com/tsavola/gate/internal/serverapi"
	"github.com/tsavola/gate/runtime"
	"github.com/tsavola/gate/server/event"
	"github.com/tsavola/gate/server/internal/error/failrequest"
)

func makeInstanceID() string {
	return uuid.New().String()
}

func validateInstanceID(s string) error {
	if x, err := uuid.Parse(s); err == nil {
		if x.Version() == 4 && x.Variant() == uuid.RFC4122 {
			return nil
		}
	}

	return failrequest.New(event.FailRequest_InstanceIdInvalid, "instance id must be an RFC 4122 UUID version 4")
}

func makeInstanceFactory(ctx context.Context, s *Server) <-chan *Instance {
	channel := make(chan *Instance, s.PreforkProcs-1)

	go func() {
		defer func() {
			close(channel)

			for inst := range channel {
				inst.kill()
			}
		}()

		for {
			if inst, err := newInstance(ctx, s); err == nil {
				select {
				case channel <- inst:

				case <-ctx.Done():
					inst.kill()
					return
				}
			} else {
				select {
				case <-ctx.Done():
					return

				default:
					reportInternalError(ctx, s, nil, "", "", "", "instance factory", err)
					time.Sleep(time.Second)
				}
			}
		}
	}()

	return channel
}

type Status = serverapi.Status
type InstanceStatus = serverapi.InstanceStatus
type Instances []InstanceStatus

func (a Instances) Len() int           { return len(a) }
func (a Instances) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a Instances) Less(i, j int) bool { return a[i].Instance < a[j].Instance }

type Instance struct {
	terminated chan struct{}       // Set in Instance.newInstance
	ref        image.ExecutableRef // Set in Instance.newInstance
	process    *runtime.Process    // Set in Instance.newInstance
	account    *account            // Set in Server.newInstance
	services   InstanceServices    // Set in Server.newInstance
	id         string              // Set in Server.newInstance or Server.registerInstance
	progHash   string              // Set in Server.registerInstance
	function   string              // Set in Server.registerInstance

	lock   sync.Mutex // Must be held when accessing status.
	status Status     // Set in Server.registerInstance and Instance.Run
}

func newInstance(ctx context.Context, s *Server) (inst *Instance, err error) {
	ref, err := image.NewExecutableRef(s.InstanceStore)
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			ref.Close()
		}
	}()

	proc, err := runtime.NewProcess(ctx, s.Executor, ref, s.Debug)
	if err != nil {
		return
	}

	inst = &Instance{
		terminated: make(chan struct{}),
		ref:        ref,
		process:    proc,
	}
	return
}

func (inst *Instance) PrincipalID() string { return inst.account.PrincipalID }
func (inst *Instance) ID() string          { return inst.id }
func (inst *Instance) ModuleKey() string   { return inst.progHash }

func (inst *Instance) Status() Status {
	inst.lock.Lock()
	defer inst.lock.Unlock()
	return inst.status
}

func (inst *Instance) kill() (err error) {
	inst.process.Kill()
	err = inst.ref.Close()

	if inst.services != nil {
		if closeErr := inst.services.Close(); err == nil {
			err = closeErr
		}
	}
	return
}

// Connect.  Disconnection happens when context is canceled, the process
// terminates, or the program closes the connection.  The program may choose to
// close a connection when a new one is made.
func (inst *Instance) Connect(ctx context.Context, r io.Reader, w io.Writer) (disconnected <-chan error) {
	c := make(chan error)
	disconnected = c

	conn := inst.services.Connect(ctx)
	if conn == nil {
		close(c)
		return
	}

	go func() {
		defer close(c)
		c <- conn(ctx, r, w)
	}()
	return
}

func (inst *Instance) Run(ctx context.Context, s *Server) (result Status) {
	defer inst.kill()
	defer close(inst.terminated)

	result.Error = "internal server error"

	defer func() {
		inst.lock.Lock()
		defer inst.lock.Unlock()
		inst.status = result
	}()

	exit, trapID, err := inst.process.Serve(ctx, inst.services)
	if err != nil {
		if x, ok := err.(public.Error); ok {
			result.Error = x.PublicError()
		}

		switch err.(type) {
		case badprogram.Error:
			result.State = serverapi.Status_TERMINATED
			result.Cause = serverapi.Status_VIOLATION

			reportProgramError(ctx, s, inst.account.PrincipalKey, inst.progHash, inst.function, inst.id, err)

		default:
			reportInternalError(ctx, s, inst.account.PrincipalKey, inst.progHash, inst.function, inst.id, "service io", err)
		}

		return
	}

	result.State = serverapi.Status_TERMINATED
	result.Cause = serverapi.Status_TRAP
	result.Trap = serverapi.Status_TrapId(trapID)
	result.Exit = int32(exit)
	result.Error = ""
	return
}

func reportProgramError(ctx context.Context, s *Server, pri *PrincipalKey, progHash, function string, instID string, err error) {
	s.Monitor(&event.FailRequest{
		Ctx:      Context(ctx, pri),
		Failure:  event.FailRequest_ProgramError,
		Module:   progHash,
		Function: function,
		Instance: instID,
	}, err)
}

func reportInternalError(ctx context.Context, s *Server, pri *PrincipalKey, progHash, function string, instID, subsys string, err error) {
	if x, ok := err.(subsystem.Error); ok {
		subsys = x.Subsystem()
	}

	s.Monitor(&event.FailInternal{
		Ctx:       Context(ctx, pri),
		Module:    progHash,
		Function:  function,
		Instance:  instID,
		Subsystem: subsys,
	}, err)
}
