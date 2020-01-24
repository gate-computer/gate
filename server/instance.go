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
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/tsavola/gate/image"
	"github.com/tsavola/gate/internal/error/public"
	"github.com/tsavola/gate/internal/error/subsystem"
	"github.com/tsavola/gate/internal/manifest"
	"github.com/tsavola/gate/runtime"
	"github.com/tsavola/gate/server/event"
	"github.com/tsavola/gate/server/internal/error/failrequest"
	"github.com/tsavola/gate/server/internal/error/resourcenotfound"
	api "github.com/tsavola/gate/serverapi"
	"github.com/tsavola/gate/snapshot"
	"github.com/tsavola/gate/trap"
	"github.com/tsavola/wag/object/stack"
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
	return fmt.Sprintf("%s.%s", acc.ID.String(), instID)
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

type instanceLock struct{}
type instanceMutex struct{ sync.Mutex }

func (m *instanceMutex) Lock() instanceLock {
	m.Mutex.Lock()
	return instanceLock{}
}

type Instance struct {
	ID  string
	acc *account

	mu           instanceMutex // Guards the fields below.
	exists       bool
	transient    bool
	status       api.Status
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
func newInstance(id string, acc *account, image *image.Instance, persistent *snapshot.Buffers, proc *runtime.Process, services InstanceServices, timeReso time.Duration, debugStatus string, debugLog io.WriteCloser) *Instance {
	inst := &Instance{
		ID:        id,
		acc:       acc,
		transient: persistent == nil,
		status:    api.Status{Debug: debugStatus},
		image:     image,
		process:   proc,
		services:  services,
		timeReso:  timeReso,
		debugLog:  debugLog,
		stopped:   make(chan struct{}),
	}
	if persistent != nil {
		inst.buffers = *persistent
	}
	return inst
}

func (inst *Instance) startOrAnnihilate(prog *program) (drive bool, err error) {
	lock := inst.mu.Lock()
	defer inst.mu.Unlock()

	if inst.process == nil {
		if id := inst.image.Trap(); id == trap.Exit {
			if inst.image.Final() {
				inst.status.State = api.StateTerminated
			} else {
				inst.status.State = api.StateHalted
			}
			inst.status.Result = inst.image.Result()
		} else {
			if inst.image.Final() {
				inst.status.State = api.StateKilled
				if id != trap.Killed {
					inst.status.Cause = api.Cause(id)
				}
			} else {
				inst.status.State, inst.status.Cause = trapStatus(id)
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
		Debug:          inst.debugLog,
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

func (inst *Instance) Status() api.Status {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	return inst.status
}

func (inst *Instance) instanceStatus() api.InstanceStatus {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	return api.InstanceStatus{
		Instance:  inst.ID,
		Status:    inst.status,
		Transient: inst.transient,
		Debugging: inst.image.DebugInfo() || len(inst.image.Breakpoints()) > 0,
	}
}

func (inst *Instance) Wait(ctx context.Context) (status api.Status) {
	inst.mu.Lock()
	status = inst.status
	stopped := inst.stopped
	inst.mu.Unlock()

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
	inst.mu.Lock()
	proc := inst.process
	inst.mu.Unlock()

	if proc != nil {
		proc.Kill()
	}
}

func (inst *Instance) Suspend() {
	inst.mu.Lock()
	if inst.status.State == api.StateRunning {
		inst.transient = false
	}
	proc := inst.process
	inst.mu.Unlock()

	if proc != nil {
		proc.Suspend()
	}
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
		entryIndex, err = prog.image.ResolveEntryFunc(function, true)
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

// doResume steals proc, services and debugLog.
func (inst *Instance) doResume(prog *program, function string, proc *runtime.Process, services InstanceServices, timeReso time.Duration, debugStatus string, debugLog io.WriteCloser,
) (err error) {
	lock := inst.mu.Lock()
	defer inst.mu.Unlock()

	// Check again in case of a race condition.  (CheckMutation caches result.)
	entryIndex, err := inst.resumeCheck(lock, prog, function)
	if err != nil {
		return
	}

	inst.status = api.Status{
		State: api.StateRunning,
		Debug: debugStatus,
	}
	inst.image.SetEntry(prog.image, entryIndex)
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
	res := api.Status{
		State: api.StateKilled,
		Cause: api.CauseInternal,
		Debug: inst.status.Debug,
	}
	defer func() {
		lock := inst.mu.Lock()
		defer inst.mu.Unlock()

		if res.State >= api.StateTerminated {
			inst.image.SetFinal()
		}
		inst.image.SetTrap(trapID)
		inst.image.SetResult(res.Result)
		inst.status = res
		inst.stop(lock)
	}()

	result, trapID, err := inst.process.Serve(ctx, inst.services, &inst.buffers)
	if err != nil {
		res.Error = public.Error(err, res.Error)
		if trapID == trap.ABIViolation {
			res.Cause = api.CauseABIViolation
			return programFailure(ctx, prog.hash, function, inst.ID), err
		} else {
			return internalFailure(ctx, prog.hash, function, inst.ID, "service io", err), err
		}
	}

	if !inst.transient {
		err = prog.ensureStorage()
		if err == nil {
			err = inst.image.Store(instanceStorageKey(inst.acc, inst.ID), prog.image)
		}
		if err != nil {
			res.Error = public.Error(err, res.Error)
			return internalFailure(ctx, prog.hash, function, inst.ID, "", err), err
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

func (inst *Instance) debug(ctx context.Context, prog *program, req api.DebugRequest,
) (rebuild *instanceRebuild, newConfig api.DebugConfig, res api.DebugResponse, err error) {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	if req.Op < api.DebugOpConfigGet || req.Op > api.DebugOpReadStack {
		err = public.Err("unsupported debug op") // TODO: http response code: not implemented
		return
	}

	if req.Op != api.DebugOpConfigGet && inst.status.State != api.StateSuspended && inst.status.State != api.StateHalted {
		err = failrequest.Errorf(event.FailInstanceStatus, "instance must be suspended or halted")
		return
	}

	info := inst.image.DebugInfo()
	breaks := inst.image.Breakpoints()
	modified := false

	switch req.Op {
	case api.DebugOpConfigSet:
		if len(req.Config.Breakpoints.Offsets) > manifest.MaxBreakpoints {
			err = public.Err("too many breakpoints")
			return
		}

		info = req.Config.DebugInfo
		if info != inst.image.DebugInfo() {
			modified = true
		}

		breaks = manifest.SortDedupUint64(req.Config.Breakpoints.Offsets)
		if !reflect.DeepEqual(breaks, inst.image.Breakpoints()) {
			modified = true
		}

	case api.DebugOpConfigUnion:
		if len(breaks)+len(req.Config.Breakpoints.Offsets) > manifest.MaxBreakpoints {
			err = public.Err("too many breakpoints")
			return
		}

		if req.Config.DebugInfo {
			if !info {
				modified = true
			}
			info = true
		}

		breaks = append([]uint64{}, breaks...)
		for _, x := range req.Config.Breakpoints.Offsets {
			if i := searchUint64(breaks, x); i == len(breaks) || breaks[i] != x {
				breaks = append(breaks[:i], append([]uint64{x}, breaks[i:]...)...)
				modified = true
			}
		}

	case api.DebugOpConfigComplement:
		if req.Config.DebugInfo {
			if info {
				modified = true
			}
			info = false
		}

		breaks = append([]uint64{}, breaks...)
		for _, x := range req.Config.Breakpoints.Offsets {
			if i := searchUint64(breaks, x); i < len(breaks) && breaks[i] == x {
				breaks = append(breaks[:i], breaks[i+1:]...)
				modified = true
			}
		}

	case api.DebugOpReadGlobals:
		res.Module = path.Join(api.ModuleRefSource, prog.hash)

		panic("TODO")

	case api.DebugOpReadMemory:
		res.Module = path.Join(api.ModuleRefSource, prog.hash)

		panic("TODO")

	case api.DebugOpReadStack:
		res.Module = path.Join(api.ModuleRefSource, prog.hash)

		textMap := inst.altTextMap
		if inst.altProgImage == nil {
			textMap = &prog.image.Map
		}

		res.Data, err = inst.image.ExportStack(textMap)
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
				inst: inst,
				oldConfig: api.DebugConfig{
					DebugInfo:   inst.image.DebugInfo(),
					Breakpoints: manifest.Breakpoints{Offsets: inst.image.Breakpoints()},
				},
			}
			newConfig = api.DebugConfig{
				DebugInfo:   info,
				Breakpoints: manifest.Breakpoints{Offsets: breaks},
			}
		}
	}

	res.Status = inst.status
	res.Config = api.DebugConfig{
		DebugInfo:   inst.image.DebugInfo(),
		Breakpoints: manifest.Breakpoints{Offsets: inst.image.Breakpoints()},
	}

	return
}

type instanceRebuild struct {
	inst      *Instance
	oldConfig api.DebugConfig
}

func (rebuild *instanceRebuild) apply(progImage *image.Program, newConfig api.DebugConfig, textMap stack.TextMap,
) (res api.DebugResponse, ok bool) {
	inst := rebuild.inst
	oldConfig := rebuild.oldConfig

	inst.mu.Lock()
	defer inst.mu.Unlock()

	if inst.image.DebugInfo() == oldConfig.DebugInfo && reflect.DeepEqual(inst.image.Breakpoints(), oldConfig.Breakpoints.Offsets) {
		if inst.altProgImage != nil {
			inst.altProgImage.Close()
		}
		inst.altProgImage = progImage
		inst.altTextMap = textMap

		inst.image.SetDebugInfo(newConfig.DebugInfo)
		inst.image.SetBreakpoints(newConfig.Breakpoints.Offsets)
		ok = true
	}

	res = api.DebugResponse{
		Status: inst.status,
		Config: api.DebugConfig{
			DebugInfo:   inst.image.DebugInfo(),
			Breakpoints: manifest.Breakpoints{Offsets: inst.image.Breakpoints()},
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
