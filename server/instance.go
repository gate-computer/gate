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
	"github.com/tsavola/gate/server/internal/error/resourcenotfound"
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

type instanceLock struct{}
type instanceMutex struct{ sync.Mutex }

func (m *instanceMutex) Lock() instanceLock {
	m.Mutex.Lock()
	return instanceLock{}
}

type Instance struct {
	acc *account
	id  string

	mu         instanceMutex // Guards the fields below.
	status     Status
	function   string
	image      *image.Instance
	persistent *snapshot.Buffers // Exclusive to Run while status is RUNNING.
	process    *runtime.Process
	services   InstanceServices
	timeReso   time.Duration
	debug      io.WriteCloser
	stopped    chan struct{}
}

// newInstance steals instance image, persistent buffers, process, and services.
func newInstance(acc *account, id string, function string, image *image.Instance, persistent *snapshot.Buffers, proc *runtime.Process, services InstanceServices, timeReso time.Duration, debugStatus string, debugOutput io.WriteCloser) *Instance {
	return &Instance{
		acc: acc,
		id:  id,
		status: Status{
			State: StateRunning,
			Debug: debugStatus,
		},
		function:   function,
		image:      image,
		persistent: persistent,
		process:    proc,
		services:   services,
		timeReso:   timeReso,
		debug:      debugOutput,
		stopped:    make(chan struct{}),
	}
}

func (inst *Instance) startOrAnnihilate(prog *program) (err error) {
	lock := inst.mu.Lock()
	defer inst.mu.Unlock()

	policy := runtime.ProcessPolicy{
		TimeResolution: inst.timeReso,
		Debug:          inst.debug,
	}

	err = inst.process.Start(prog.image, inst.image, policy)
	if err != nil {
		inst.status.State = StateNonexistent
		inst.stop(lock)
		inst.image.Close()
		inst.image = nil
	}
	return
}

func (inst *Instance) stop(instanceLock) {
	close(inst.stopped)

	inst.process.Close()

	inst.services.Close()
	inst.services = nil

	if inst.debug != nil {
		inst.debug.Close()
		inst.debug = nil
	}
}

func (inst *Instance) ID() string {
	return inst.id
}

func (inst *Instance) Status() Status {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	return inst.status
}

func (inst *Instance) Wait(ctx context.Context) (status Status) {
	inst.mu.Lock()
	status = inst.status
	stopped := inst.stopped
	inst.mu.Unlock()

	if status.State != StateRunning {
		return
	}

	select {
	case <-stopped:
	case <-ctx.Done():
	}

	return inst.Status()
}

func (inst *Instance) Kill() {
	inst.mu.Lock()
	proc := inst.process
	inst.mu.Unlock()

	proc.Kill()
}

func (inst *Instance) suspend() {
	inst.mu.Lock()
	proc := inst.process
	inst.mu.Unlock()

	proc.Suspend()
}

func (inst *Instance) checkResume(prog *program, function string) (err error) {
	lock := inst.mu.Lock()
	defer inst.mu.Unlock()

	_, err = inst.resumeCheck(lock, prog, function)
	return
}

func (inst *Instance) resumeCheck(_ instanceLock, prog *program, function string,
) (entryIndex int, err error) {
	entryIndex = -1

	switch inst.status.State {
	case StateNonexistent:
		err = resourcenotfound.ErrInstance
		return

	case StateSuspended:
		if function != "" {
			err = failrequest.Errorf(event.FailInstanceStatus, "function specified for suspended instance")
			return
		}

	case StateHalted:
		if function == "" {
			err = failrequest.Errorf(event.FailInstanceStatus, "function must be specified when resuming halted instance")
			return
		}
		entryIndex, err = prog.image.ResolveEntryFunc(function)
		if err != nil {
			return
		}

	default:
		err = failrequest.Errorf(event.FailInstanceStatus, "instance must be suspended or halted")
		return
	}

	err = inst.image.CheckMutation()
	return
}

// doResume steals services, proc and debugOutput.
func (inst *Instance) doResume(prog *program, function string, proc *runtime.Process, services InstanceServices, timeReso time.Duration, debugStatus string, debugOutput io.WriteCloser,
) (err error) {
	lock := inst.mu.Lock()
	defer inst.mu.Unlock()

	// Check again in case of a race condition.  (CheckMutation caches result.)
	entryIndex, err := inst.resumeCheck(lock, prog, function)
	if err != nil {
		return
	}

	inst.status = Status{
		State: StateRunning,
		Debug: debugStatus,
	}
	if function != "" {
		inst.function = function
	}
	inst.image.SetEntry(prog.image, entryIndex)
	inst.process = proc
	inst.services = services
	inst.timeReso = timeReso
	inst.debug = debugOutput
	inst.stopped = make(chan struct{})
	return
}

// Connect to a running instance.  Disconnection happens when context is
// canceled, the process terminates, or the program closes the connection.
func (inst *Instance) Connect(ctx context.Context, r io.Reader, w io.Writer) error {
	conn := inst.connect(ctx)
	if conn == nil {
		return nil
	}

	return conn(ctx, r, w)
}

func (inst *Instance) connect(ctx context.Context) func(context.Context, io.Reader, io.Writer) error {
	inst.mu.Lock()
	s := inst.services
	inst.mu.Unlock()

	if s == nil {
		return nil
	}

	return s.Connect(ctx)
}

func (inst *Instance) snapshot(prog *program,
) (progImage *image.Program, buffers snapshot.Buffers, err error) {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	switch inst.status.State {
	case StateNonexistent:
		err = resourcenotfound.ErrInstance
		return

	case StateSuspended, StateHalted, StateTerminated:
		// ok

	default:
		err = failrequest.Errorf(event.FailInstanceStatus, "instance must be suspended, halted or terminated")
		return
	}

	if inst.persistent == nil {
		err = resourcenotfound.ErrInstance
		return
	}

	buffers = *inst.persistent
	progImage, err = image.Snapshot(prog.image, inst.image, buffers, inst.status.State == StateSuspended)
	return
}

// annihilate a stopped instance into nonexistence.
func (inst *Instance) annihilate() (status Status, err error) {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	switch inst.status.State {
	case StateNonexistent:
		err = resourcenotfound.ErrInstance
		return

	case StateSuspended, StateHalted, StateTerminated, StateKilled:
		// ok

	default:
		status = inst.status
		err = failrequest.Errorf(event.FailInstanceStatus, "instance must be suspended, halted, terminated or killed")
		return
	}

	inst.status = Status{Debug: inst.status.Debug}
	inst.image.Unstore()
	inst.image.Close()
	inst.image = nil
	return
}

func (inst *Instance) drive(ctx context.Context, prog *program) (Event, error) {
	res := Status{Error: "internal server error"}
	defer func() {
		lock := inst.mu.Lock()
		defer inst.mu.Unlock()

		res.Debug = inst.status.Debug
		inst.status = res
		inst.stop(lock)
	}()

	exit, trap, err := inst.process.Serve(ctx, inst.services, inst.persistent)
	if err != nil {
		switch err.(type) {
		case badprogram.Error:
			res.State = StateKilled
			res.Cause = CauseABIViolation
			if x, ok := err.(public.Error); ok {
				res.Error = x.PublicError()
			} else {
				res.Error = ""
			}
			return programFailure(ctx, inst.acc, prog.hash, inst.function, inst.id), err

		default:
			if x, ok := err.(public.Error); ok {
				res.Error = x.PublicError()
			}
			return internalFailure(ctx, inst.acc, prog.hash, inst.function, inst.id, "service io", err), err
		}
	}

	switch trap {
	case runtime.TrapExit:
		if inst.persistent == nil || inst.persistent.Terminated() {
			res.State = StateTerminated
		} else {
			res.State = StateHalted
		}

	case runtime.TrapSuspended:
		res.State = StateSuspended

	case runtime.TrapCallStackExhausted, runtime.TrapABIDeficiency:
		res.State = StateSuspended
		res.Cause = Cause(trap)

	case runtime.TrapKilled:
		res.State = StateKilled

	default:
		res.State = StateKilled
		res.Cause = Cause(trap)
	}

	if inst.persistent != nil && (res.State == StateSuspended || res.State == StateHalted || res.State == StateTerminated) {
		err = prog.ensureStorage()
		if err == nil {
			_, err = inst.image.Store(instanceStorageKey(inst.acc, inst.id), prog.image)
		}
		if err != nil {
			res.State = StateNonexistent
			res.Cause = CauseNormal
			if x, ok := err.(public.Error); ok {
				res.Error = x.PublicError()
			}
			return internalFailure(ctx, inst.acc, prog.hash, inst.function, inst.id, "", err), err
		}
	}

	if res.State == StateHalted || res.State == StateTerminated {
		res.Result = int32(exit)
	}

	res.Error = ""
	return nil, nil
}

func programFailure(ctx context.Context, acc *account, progHash, function string, instID string) Event {
	return &event.FailRequest{
		Ctx:      accountContext(ctx, acc),
		Failure:  event.FailProgramError,
		Module:   progHash,
		Function: function,
		Instance: instID,
	}
}

func internalFailure(ctx context.Context, acc *account, progHash, function string, instID, subsys string, err error) Event {
	if x, ok := err.(subsystem.Error); ok {
		subsys = x.Subsystem()
	}

	return &event.FailInternal{
		Ctx:       accountContext(ctx, acc),
		Module:    progHash,
		Function:  function,
		Instance:  instID,
		Subsystem: subsys,
	}
}
