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
	"github.com/tsavola/gate/internal/serverapi"
	"github.com/tsavola/gate/runtime"
	"github.com/tsavola/gate/server/event"
	"github.com/tsavola/gate/server/internal/error/failrequest"
	"github.com/tsavola/gate/snapshot"
	"github.com/tsavola/wag/trap"
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

func instanceStorageKey(acc *account, instID string) string {
	// The delimiter must be suitable for URL-safe base64 and UUID.
	return fmt.Sprintf("%s.%s", acc.PrincipalID, instID)
}

type Status = serverapi.Status
type InstanceStatus = serverapi.InstanceStatus
type Instances []InstanceStatus

func (a Instances) Len() int           { return len(a) }
func (a Instances) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a Instances) Less(i, j int) bool { return a[i].Instance < a[j].Instance }

type Instance struct {
	acc      *account
	id       string
	prog     *program
	persist  bool
	function string
	lock     sync.Mutex
	status   Status
	image    *image.Instance
	buffers  snapshot.Buffers
	process  *runtime.Process
	timeReso time.Duration
	services InstanceServices
	debug    io.WriteCloser
	stopped  chan struct{}
}

// newInstance steals program reference, instance image, process and services.
func newInstance(acc *account, id string, prog *program, persist bool, function string, image *image.Instance, proc *runtime.Process, timeReso time.Duration, services InstanceServices, debugStatus string, debugOutput io.WriteCloser) *Instance {
	return &Instance{
		acc:      acc,
		id:       id,
		prog:     prog,
		persist:  persist,
		function: function,
		status: Status{
			State: serverapi.Status_running,
			Debug: debugStatus,
		},
		image:    image,
		process:  proc,
		timeReso: timeReso,
		services: services,
		debug:    debugOutput,
		stopped:  make(chan struct{}),
	}
}

// renew must be called with Instance.lock held.
func (inst *Instance) renew(proc *runtime.Process, timeReso time.Duration, services InstanceServices, debugStatus string, debugOutput io.WriteCloser) {
	inst.status = serverapi.Status{
		State: serverapi.Status_running,
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

func (inst *Instance) PrincipalID() string {
	return inst.acc.PrincipalID
}

func (inst *Instance) Status() Status {
	inst.lock.Lock()
	defer inst.lock.Unlock()

	return inst.status
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
func (inst *Instance) Connect(ctx context.Context, r io.Reader, w io.Writer) (disconnected <-chan error) {
	c := make(chan error)
	disconnected = c

	services := func() InstanceServices {
		inst.lock.Lock()
		defer inst.lock.Unlock()

		if inst.status.State == serverapi.Status_running {
			return inst.services
		} else {
			return nil
		}
	}()
	if services == nil {
		close(c)
		return
	}

	conn := services.Connect(ctx)
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

// Run the program.
//
// The returned error has already been reported, and its message has been
// copied to result.Error.
func (inst *Instance) Run(ctx context.Context, s *Server) (result Status, err error) {
	result.Error = "internal server error"
	result.Debug = inst.status.Debug

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

	err = inst.process.Start(inst.prog.image, inst.image, policy)
	if err != nil {
		return
	}

	exit, trapID, err := inst.process.Serve(ctx, inst.services, &inst.buffers)
	if err != nil {
		if x, ok := err.(public.Error); ok {
			result.Error = x.PublicError()
		}

		switch err.(type) {
		case badprogram.Error:
			result.State = serverapi.Status_killed
			result.Cause = serverapi.Status_abi_violation

			reportProgramError(ctx, s, inst.acc, inst.prog.key, inst.function, inst.id, err)

		default:
			reportInternalError(ctx, s, inst.acc, inst.prog.key, inst.function, inst.id, "service io", err)
		}

		return
	}

	if inst.persist {
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
	case trap.Suspended:
		result.State = serverapi.Status_suspended

	case trap.CallStackExhausted:
		result.State = serverapi.Status_suspended
		result.Cause = serverapi.Status_Cause(trapID)

	default:
		result.State = serverapi.Status_terminated
		result.Cause = serverapi.Status_Cause(trapID)
		result.Result = int32(exit)
	}

	result.Error = ""
	return
}

func reportProgramError(ctx context.Context, s *Server, acc *account, progHash, function string, instID string, err error) {
	s.Monitor(&event.FailRequest{
		Ctx:      accountContext(ctx, acc),
		Failure:  event.FailRequest_ProgramError,
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
