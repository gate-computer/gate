// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/tsavola/gate/image"
	"github.com/tsavola/gate/internal/error/badprogram"
	"github.com/tsavola/gate/internal/error/public"
	"github.com/tsavola/gate/internal/error/subsystem"
	"github.com/tsavola/gate/internal/principal"
	"github.com/tsavola/gate/runtime"
	"github.com/tsavola/gate/server/event"
	"github.com/tsavola/gate/server/internal/error/failrequest"
	"github.com/tsavola/gate/snapshot"
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

	return failrequest.New(event.FailInstanceIDInvalid, "instance id must be an RFC 4122 UUID version 4")
}

func instanceStorageKey(acc *account, instID string) string {
	return fmt.Sprintf("%s.%s", principal.KeyPrincipalID(acc.Key).String(), instID)
}

type Instance struct {
	acc        *account
	id         string
	prog       *program
	function   string
	lock       sync.Mutex
	status     Status
	image      *image.Instance
	persistent *snapshot.Buffers
	process    *runtime.Process
	timeReso   time.Duration
	services   InstanceServices
	debug      io.WriteCloser
	stopped    chan struct{}
}

// newInstance steals program reference, instance image, process and services.
func newInstance(acc *account, id string, prog *program, persist bool, function string, image *image.Instance, proc *runtime.Process, timeReso time.Duration, services InstanceServices, debugStatus string, debugOutput io.WriteCloser) *Instance {
	inst := &Instance{
		acc:      acc,
		id:       id,
		prog:     prog,
		function: function,
		status: Status{
			State: StateRunning,
			Debug: debugStatus,
		},
		image:    image,
		process:  proc,
		timeReso: timeReso,
		services: services,
		debug:    debugOutput,
		stopped:  make(chan struct{}),
	}

	if persist {
		clone := prog.buffers
		inst.persistent = &clone
	}

	return inst
}

// renew must be called with Instance.lock held.
func (inst *Instance) renew(function string, proc *runtime.Process, timeReso time.Duration, services InstanceServices, debugStatus string, debugOutput io.WriteCloser) {
	if function != "" {
		inst.function = function
	}
	inst.status = Status{
		State: StateRunning,
		Debug: debugStatus,
	}
	inst.process = proc
	inst.timeReso = timeReso
	inst.services = services
	inst.debug = debugOutput
	inst.stopped = make(chan struct{})
}

func (inst *Instance) ID() string {
	return inst.id
}

func (inst *Instance) Status() Status {
	inst.lock.Lock()
	defer inst.lock.Unlock()

	return inst.status
}

func (inst *Instance) Wait(ctx context.Context) Status {
	inst.lock.Lock()
	status := inst.status
	stopped := inst.stopped
	inst.lock.Unlock()

	if status.State != StateRunning {
		return status
	}

	select {
	case <-stopped:
	case <-ctx.Done():
	}

	return inst.Status()
}

func (inst *Instance) suspend(s *Server) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if inst.process != nil {
		inst.process.Suspend()
	}
}

func (inst *Instance) killProcess() {
	if inst.process != nil {
		inst.services.Close()
		inst.process.Kill()
		inst.process = nil
	}

	if inst.debug != nil {
		inst.debug.Close()
		inst.debug = nil
	}
}

func (inst *Instance) Kill(s *Server) {
	s.lock.Lock()
	defer s.lock.Unlock()

	inst.killProcess()

	inst.image.Unstore()
	inst.image.Close()
	inst.image = nil

	inst.prog.unref()
	inst.prog = nil
}

// Connect to a running instance.  Disconnection happens when context is
// canceled, the process terminates, or the program closes the connection.
func (inst *Instance) Connect(ctx context.Context, r io.Reader, w io.Writer) (err error) {
	services := func() InstanceServices {
		inst.lock.Lock()
		defer inst.lock.Unlock()

		if inst.status.State == StateRunning {
			return inst.services
		} else {
			return nil
		}
	}()
	if services == nil {
		return
	}

	conn := services.Connect(ctx)
	if conn == nil {
		return
	}

	err = conn(ctx, r, w)
	return
}

func (inst *Instance) Run(ctx context.Context, s *Server) {
	result := Status{
		Error: "internal server error",
		Debug: inst.status.Debug,
	}

	defer func() {
		inst.lock.Lock()
		defer inst.lock.Unlock()

		inst.status = result
		close(inst.stopped)
		inst.killProcess()
	}()

	policy := runtime.ProcessPolicy{
		TimeResolution: inst.timeReso,
		Debug:          inst.debug,
	}

	err := inst.process.Start(inst.prog.image, inst.image, policy)
	if err != nil {
		// TODO: report error
		return
	}

	exit, trapID, err := inst.process.Serve(ctx, inst.services, inst.persistent)
	if err != nil {
		if x, ok := err.(public.Error); ok {
			result.Error = x.PublicError()
		}

		switch err.(type) {
		case badprogram.Error:
			result.State = StateKilled
			result.Cause = CauseABIViolation

			reportProgramError(ctx, s, inst.acc, inst.prog.key, inst.function, inst.id, err)

		default:
			reportInternalError(ctx, s, inst.acc, inst.prog.key, inst.function, inst.id, "service io", err)
		}

		return
	}

	if inst.persistent != nil {
		err = inst.prog.ensureStorage()
		if err == nil {
			_, err = inst.image.Store(instanceStorageKey(inst.acc, inst.id), inst.prog.image)
		}
		if err != nil {
			if x, ok := err.(public.Error); ok {
				result.Error = x.PublicError()
			}

			reportInternalError(ctx, s, inst.acc, inst.prog.key, inst.function, inst.id, "", err)
			return
		}
	}

	switch trapID {
	case runtime.TrapExit:
		if inst.persistent == nil || inst.persistent.Terminated() {
			result.State = StateTerminated
		} else {
			result.State = StateHalted
		}
		result.Result = int32(exit)

	case runtime.TrapSuspended:
		result.State = StateSuspended

	case runtime.TrapCallStackExhausted, runtime.TrapABIDeficiency:
		result.State = StateSuspended
		result.Cause = Cause(trapID)

	default:
		result.State = StateKilled
		result.Cause = Cause(trapID)
	}

	result.Error = ""
}

func reportProgramError(ctx context.Context, s *Server, acc *account, progHash, function string, instID string, err error) {
	s.Monitor(&event.FailRequest{
		Ctx:      accountContext(ctx, acc),
		Failure:  event.FailProgramError,
		Module:   progHash,
		Function: function,
		Instance: instID,
	}, err)
}

func reportInternalError(ctx context.Context, s *Server, acc *account, progHash, function string, instID, subsys string, err error) {
	if x, ok := err.(subsystem.Error); ok {
		subsys = x.Subsystem()
	}

	s.Monitor(&event.FailInternal{
		Ctx:       accountContext(ctx, acc),
		Module:    progHash,
		Function:  function,
		Instance:  instID,
		Subsystem: subsys,
	}, err)
}
