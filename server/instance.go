// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"
	"fmt"
	"io"
	"path"
	"reflect"
	"strings"
	"time"

	"gate.computer/gate/image"
	"gate.computer/gate/internal/error/public"
	"gate.computer/gate/internal/error/subsystem"
	"gate.computer/gate/internal/manifest"
	"gate.computer/gate/internal/principal"
	pprincipal "gate.computer/gate/principal"
	"gate.computer/gate/runtime"
	"gate.computer/gate/server/api"
	"gate.computer/gate/server/event"
	"gate.computer/gate/server/internal/error/failrequest"
	"gate.computer/gate/server/internal/error/resourcenotfound"
	"gate.computer/gate/snapshot"
	"gate.computer/gate/trap"
	"gate.computer/wag/object/stack"
	"github.com/google/uuid"
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

func contextWithInstanceID(ctx context.Context, id string) context.Context {
	return pprincipal.ContextWithInstanceUUID(ctx, uuid.Must(uuid.Parse(id)))
}

func instanceStorageKey(pri *principal.ID, instID string) string {
	return fmt.Sprintf("%s.%s", pri.String(), instID)
}

func parseInstanceStorageKey(key string) (pri *principal.ID, instID string, err error) {
	i := strings.LastIndexByte(key, '.')
	if i < 0 {
		err = fmt.Errorf("invalid instance storage key: %q", key)
		return
	}

	pri, err = principal.ParseID(key[:i])
	if err != nil {
		return
	}

	instID = key[i+1:]
	err = validateInstanceID(instID)
	return
}

// trapStatus converts non-exit trap id to non-final instance state and cause.
func trapStatus(id trap.ID) (api.State, api.Cause) {
	switch id {
	case trap.Suspended:
		return api.StateSuspended, api.CauseNormal

	case trap.CallStackExhausted, trap.ABIDeficiency, trap.Breakpoint:
		return api.StateSuspended, api.Cause(id)

	case trap.Killed:
		return api.StateKilled, api.CauseNormal

	default:
		return api.StateKilled, api.Cause(id)
	}
}

type Instance struct {
	ID  string
	acc *account

	mu           instanceMutex // Guards the fields below.
	exists       bool
	transient    bool
	status       *api.Status
	altProgImage *image.Program
	altTextMap   stack.TextMap
	image        *image.Instance
	buffers      snapshot.Buffers
	process      *runtime.Process
	services     InstanceServices
	timeReso     time.Duration
	debugLog     io.WriteCloser
	stopped      chan struct{}
}

// newInstance steals instance image, process, and services.
func newInstance(id string, acc *account, transient bool, image *image.Instance, buffers snapshot.Buffers, proc *runtime.Process, services InstanceServices, timeReso time.Duration, debugLog io.WriteCloser) *Instance {
	return &Instance{
		ID:        id,
		acc:       acc,
		transient: transient,
		status:    new(api.Status),
		image:     image,
		buffers:   buffers,
		process:   proc,
		services:  services,
		timeReso:  timeReso,
		debugLog:  debugLog,
		stopped:   make(chan struct{}),
	}
}

func (inst *Instance) store(_ instanceLock, prog *program) error {
	return inst.image.Store(instanceStorageKey(inst.acc.ID, inst.ID), prog.hash, prog.image)
}

func (inst *Instance) startOrAnnihilate(prog *program) (drive bool, err error) {
	lock := inst.mu.Lock()
	defer inst.mu.Unlock()

	if inst.process == nil {
		err = inst.store(lock, prog)
		if err != nil {
			return
		}

		trapID := inst.image.Trap()

		if inst.image.Final() {
			if trapID != trap.Exit {
				inst.status.State = api.StateKilled
				if trapID != trap.Killed {
					inst.status.Cause = api.Cause(trapID)
				}
			} else {
				inst.status.State = api.StateTerminated
				inst.status.Result = inst.image.Result()
			}
		} else {
			if trapID != trap.Exit {
				inst.status.State, inst.status.Cause = trapStatus(trapID)
			} else if inst.image.EntryAddr() == 0 {
				inst.status.State = api.StateHalted
				inst.status.Result = inst.image.Result()
			} else {
				inst.status.State = api.StateSuspended
			}
		}

		inst.exists = true
		close(inst.stopped)
		return
	}

	progImage := prog.image
	if inst.altProgImage != nil {
		progImage = inst.altProgImage
	}

	policy := runtime.ProcessPolicy{
		TimeResolution: inst.timeReso,
		DebugLog:       inst.debugLog,
	}

	err = inst.process.Start(progImage, inst.image, policy)
	if err != nil {
		inst.stop(lock)
		inst.image.Close()
		inst.image = nil
		return
	}

	inst.status.State = api.StateRunning
	inst.exists = true
	drive = true
	return
}

func (inst *Instance) stop(instanceLock) {
	close(inst.stopped)

	inst.process.Close()

	inst.services.Close()
	inst.services = nil

	if inst.debugLog != nil {
		inst.debugLog.Close()
		inst.debugLog = nil
	}
}

func (inst *Instance) Transient() bool {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	return inst.transient
}

func (inst *Instance) Status() *api.Status {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	return inst.status.Clone()
}

func (inst *Instance) instanceStatus() *api.InstanceStatus {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	return &api.InstanceStatus{
		Instance:  inst.ID,
		Status:    inst.status.Clone(),
		Transient: inst.transient,
		Debugging: inst.image.DebugInfo() || len(inst.image.Breakpoints()) > 0,
	}
}

func (inst *Instance) Wait(ctx context.Context) (status *api.Status) {
	var stopped <-chan struct{}
	inst.mu.Guard(func(lock instanceLock) {
		status = inst.status.Clone()
		stopped = inst.stopped
	})

	if status.State != api.StateRunning {
		return
	}

	select {
	case <-stopped:
	case <-ctx.Done():
	}

	return inst.Status()
}

func (inst *Instance) Kill() {
	proc := inst.getProcess()
	if proc == nil {
		return
	}

	proc.Kill()
}

// Suspend the instance and make it non-transient.
func (inst *Instance) Suspend() {
	var proc *runtime.Process
	inst.mu.Guard(func(lock instanceLock) {
		if inst.status.State == api.StateRunning {
			inst.transient = false
		}
		proc = inst.process
	})
	if proc == nil {
		return
	}

	proc.Suspend()
}

func (inst *Instance) suspend() {
	proc := inst.getProcess()
	if proc == nil {
		return
	}

	proc.Suspend()
}

func (inst *Instance) getProcess() *runtime.Process {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	return inst.process
}

func (inst *Instance) checkResume(function string) error {
	lock := inst.mu.Lock()
	defer inst.mu.Unlock()

	return inst.resumeCheck(lock, function)
}

func (inst *Instance) resumeCheck(_ instanceLock, function string) (err error) {
	if !inst.exists {
		err = resourcenotfound.ErrInstance
		return
	}

	switch inst.status.State {
	case api.StateSuspended:
		if function != "" {
			err = failrequest.Errorf(event.FailInstanceStatus, "function specified for suspended instance")
			return
		}

	case api.StateHalted:
		if function == "" {
			err = failrequest.Errorf(event.FailInstanceStatus, "function must be specified when resuming halted instance")
			return
		}

	default:
		err = failrequest.Errorf(event.FailInstanceStatus, "instance must be suspended or halted")
		return
	}

	return
}

// doResume steals proc, services and debugLog.
func (inst *Instance) doResume(function string, proc *runtime.Process, services InstanceServices, timeReso time.Duration, debugLog io.WriteCloser,
) (err error) {
	lock := inst.mu.Lock()
	defer inst.mu.Unlock()

	// Check again in case of a race condition.
	err = inst.resumeCheck(lock, function)
	if err != nil {
		return
	}

	inst.status = &api.Status{State: api.StateRunning}
	inst.process = proc
	inst.services = services
	inst.timeReso = timeReso
	inst.debugLog = debugLog
	inst.stopped = make(chan struct{})
	return
}

// Connect to a running instance.  Disconnection happens when context is
// canceled, the instance stops running, or the program closes the connection.
func (inst *Instance) Connect(ctx context.Context, r io.Reader, w io.Writer) error {
	conn := inst.connect(ctx)
	if conn == nil {
		return nil
	}

	return conn(ctx, r, w)
}

func (inst *Instance) connect(ctx context.Context) func(context.Context, io.Reader, io.Writer) error {
	var s InstanceServices
	inst.mu.Guard(func(lock instanceLock) {
		s = inst.services
	})
	if s == nil {
		return nil
	}

	return s.Connect(ctx)
}

func (inst *Instance) snapshot(prog *program,
) (progImage *image.Program, buffers snapshot.Buffers, err error) {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	if !inst.exists {
		err = resourcenotfound.ErrInstance
		return
	}
	if inst.status.State == api.StateRunning {
		err = failrequest.Errorf(event.FailInstanceStatus, "instance must not be running")
		return
	}

	buffers = inst.buffers
	progImage, err = image.Snapshot(prog.image, inst.image, buffers, inst.status.State == api.StateSuspended)
	return
}

// annihilate a stopped instance into nonexistence.
func (inst *Instance) annihilate() (err error) {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	if !inst.exists {
		err = resourcenotfound.ErrInstance
		return
	}
	if inst.status.State == api.StateRunning {
		err = failrequest.Errorf(event.FailInstanceStatus, "instance must not be running")
		return
	}

	inst.exists = false
	inst.image.Unstore()
	inst.image.Close()
	inst.image = nil
	return
}

func (inst *Instance) drive(ctx context.Context, prog *program, function string) (Event, error) {
	trapID := trap.InternalError
	res := &api.Status{
		State: api.StateKilled,
		Cause: api.CauseInternal,
	}

	cleanupFunc := func(lock instanceLock) {
		if res.State >= api.StateTerminated {
			inst.image.SetFinal()
		}
		inst.image.SetTrap(trapID)
		inst.image.SetResult(res.Result)
		inst.status = res
		inst.stop(lock)
	}
	defer func() {
		if cleanupFunc != nil {
			lock := inst.mu.Lock()
			defer inst.mu.Unlock()
			cleanupFunc(lock)
		}
	}()

	result, trapID, err := inst.process.Serve(contextWithInstanceID(ctx, inst.ID), inst.services, &inst.buffers)
	if err != nil {
		res.Error = public.Error(err, res.Error)
		if trapID == trap.ABIViolation {
			res.Cause = api.CauseABIViolation
			return programFailure(ctx, prog.hash, function, inst.ID), err
		} else {
			return internalFailure(ctx, prog.hash, function, inst.ID, "service io", err), err
		}
	}

	lock := inst.mu.Lock()
	defer func() {
		defer inst.mu.Unlock()
		f := cleanupFunc
		cleanupFunc = nil
		f(lock)
	}()

	mutErr := inst.image.CheckMutation()
	if mutErr != nil && trapID != trap.Killed {
		res.Error = public.Error(mutErr, res.Error)
		return internalFailure(ctx, prog.hash, function, inst.ID, "image state", mutErr), mutErr
	}

	if mutErr == nil && !inst.transient {
		err = inst.store(lock, prog)
		if err != nil {
			res.Error = public.Error(err, res.Error)
			return internalFailure(ctx, prog.hash, function, inst.ID, "image storage", err), err
		}
	}

	if trapID == trap.Exit {
		if inst.transient || result.Terminated() {
			res.State = api.StateTerminated
		} else {
			res.State = api.StateHalted
		}
		res.Cause = api.CauseNormal
		res.Result = int32(result.Value())
	} else {
		res.State, res.Cause = trapStatus(trapID)
	}

	return nil, nil
}

func (inst *Instance) debug(ctx context.Context, prog *program, req *api.DebugRequest,
) (rebuild *instanceRebuild, newConfig *api.DebugConfig, res *api.DebugResponse, err error) {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	if req.Op < api.DebugOpConfigGet || req.Op > api.DebugOpReadStack {
		err = public.Err("unsupported debug op") // TODO: http response code: not implemented
		return
	}

	if req.Op != api.DebugOpConfigGet && inst.status.State == api.StateRunning {
		err = failrequest.Errorf(event.FailInstanceStatus, "instance must be stopped")
		return
	}

	info := inst.image.DebugInfo()
	breaks := inst.image.Breakpoints()
	modified := false
	var data []byte

	switch req.Op {
	case api.DebugOpConfigSet:
		config := req.GetConfig()

		if len(config.Breakpoints) > manifest.MaxBreakpoints {
			err = public.Err("too many breakpoints")
			return
		}

		info = config.DebugInfo
		if info != inst.image.DebugInfo() {
			modified = true
		}

		breaks = manifest.SortDedupUint64(config.Breakpoints)
		if !reflect.DeepEqual(breaks, inst.image.Breakpoints()) {
			modified = true
		}

	case api.DebugOpConfigUnion:
		config := req.GetConfig()

		if len(breaks)+len(config.Breakpoints) > manifest.MaxBreakpoints {
			err = public.Err("too many breakpoints")
			return
		}

		if config.DebugInfo {
			if !info {
				modified = true
			}
			info = true
		}

		breaks = append([]uint64{}, breaks...)
		for _, x := range config.Breakpoints {
			if i := searchUint64(breaks, x); i == len(breaks) || breaks[i] != x {
				breaks = append(breaks[:i], append([]uint64{x}, breaks[i:]...)...)
				modified = true
			}
		}

	case api.DebugOpConfigComplement:
		config := req.GetConfig()

		if config.DebugInfo {
			if info {
				modified = true
			}
			info = false
		}

		breaks = append([]uint64{}, breaks...)
		for _, x := range config.Breakpoints {
			if i := searchUint64(breaks, x); i < len(breaks) && breaks[i] == x {
				breaks = append(breaks[:i], breaks[i+1:]...)
				modified = true
			}
		}

	case api.DebugOpReadGlobals:
		panic("TODO")

	case api.DebugOpReadMemory:
		panic("TODO")

	case api.DebugOpReadStack:
		textMap := inst.altTextMap
		if inst.altProgImage == nil {
			textMap = &prog.image.Map
		}

		data, err = inst.image.ExportStack(textMap)
		if err != nil {
			return
		}
	}

	if modified {
		if info == prog.image.DebugInfo() && reflect.DeepEqual(breaks, prog.image.Breakpoints()) {
			if inst.altProgImage != nil {
				inst.altProgImage.Close()
				inst.altProgImage = nil
				inst.altTextMap = nil
			}

			inst.image.SetDebugInfo(info)
			inst.image.SetBreakpoints(prog.image.Breakpoints())
		} else {
			rebuild = &instanceRebuild{
				inst:         inst,
				origProgHash: prog.hash,
				oldConfig: &api.DebugConfig{
					DebugInfo:   inst.image.DebugInfo(),
					Breakpoints: inst.image.Breakpoints(),
				},
			}
			newConfig = &api.DebugConfig{
				DebugInfo:   info,
				Breakpoints: breaks,
			}
		}
	}

	res = &api.DebugResponse{
		Module: path.Join(api.ModuleRefSource, prog.hash),
		Status: inst.status.Clone(),
		Config: &api.DebugConfig{
			DebugInfo:   inst.image.DebugInfo(),
			Breakpoints: inst.image.Breakpoints(),
		},
		Data: data,
	}
	return
}

type instanceRebuild struct {
	inst         *Instance
	origProgHash string
	oldConfig    *api.DebugConfig
}

func (rebuild *instanceRebuild) apply(progImage *image.Program, newConfig *api.DebugConfig, textMap stack.TextMap,
) (res *api.DebugResponse, ok bool) {
	inst := rebuild.inst
	oldConfig := rebuild.oldConfig

	inst.mu.Lock()
	defer inst.mu.Unlock()

	if inst.image.DebugInfo() == oldConfig.DebugInfo && reflect.DeepEqual(inst.image.Breakpoints(), oldConfig.Breakpoints) {
		if inst.altProgImage != nil {
			inst.altProgImage.Close()
		}
		inst.altProgImage = progImage
		inst.altTextMap = textMap

		inst.image.SetDebugInfo(newConfig.DebugInfo)
		inst.image.SetBreakpoints(newConfig.Breakpoints)
		ok = true
	}

	res = &api.DebugResponse{
		Module: path.Join(api.ModuleRefSource, rebuild.origProgHash),
		Status: inst.status.Clone(),
		Config: &api.DebugConfig{
			DebugInfo:   inst.image.DebugInfo(),
			Breakpoints: inst.image.Breakpoints(),
		},
	}
	return
}

func programFailure(ctx context.Context, progHash, function string, instID string) Event {
	return &event.FailRequest{
		Ctx:      ContextDetail(ctx),
		Failure:  event.FailProgramError,
		Module:   progHash,
		Function: function,
		Instance: instID,
	}
}

func internalFailure(ctx context.Context, progHash, function string, instID, subsys string, err error) Event {
	if x, ok := err.(subsystem.Error); ok {
		subsys = x.Subsystem()
	}

	return &event.FailInternal{
		Ctx:       ContextDetail(ctx),
		Module:    progHash,
		Function:  function,
		Instance:  instID,
		Subsystem: subsys,
	}
}
