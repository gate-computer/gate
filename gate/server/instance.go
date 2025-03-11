// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"fmt"
	"io"
	"path"
	"reflect"
	"strings"
	"time"

	"gate.computer/gate/image"
	"gate.computer/gate/principal"
	"gate.computer/gate/runtime"
	programscope "gate.computer/gate/scope/program"
	"gate.computer/gate/server/api"
	"gate.computer/gate/server/event"
	"gate.computer/gate/server/internal/error/failrequest"
	"gate.computer/gate/server/internal/error/notfound"
	"gate.computer/gate/snapshot"
	"gate.computer/gate/trap"
	"gate.computer/internal/dedup"
	"gate.computer/internal/error/subsystem"
	pb "gate.computer/internal/pb/server"
	internal "gate.computer/internal/principal"
	"gate.computer/wag/object"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/durationpb"
	"import.name/lock"

	. "import.name/type/context"
)

func makeInstanceID() string {
	return uuid.New().String()
}

func ValidateInstanceUUIDForm(s string) error {
	u, err := uuid.Parse(s)
	if err != nil {
		return failrequest.Error(event.FailInstanceIDInvalid, "instance id must be a UUID")
	}
	if u.Version() != 4 {
		return failrequest.Error(event.FailInstanceIDInvalid, "instance UUID version must be 4")
	}
	if u.Variant() != uuid.RFC4122 {
		return failrequest.Error(event.FailInstanceIDInvalid, "instance UUID variant must be RFC 4122")
	}
	if u.String() == s {
		return nil
	}
	if s[0] == '{' {
		return failrequest.Error(event.FailInstanceIDInvalid, "instance UUID must not be surrounded by brackets")
	}
	if strings.Contains(s, ":") {
		return failrequest.Error(event.FailInstanceIDInvalid, "instance UUID must not have namespace prefix")
	}
	return failrequest.Error(event.FailInstanceIDInvalid, "instance UUID must use lower-case hex encoding")
}

func instanceServingContext(ctx Context, id string) Context {
	ctx = principal.ContextWithInstanceUUID(ctx, uuid.Must(uuid.Parse(id)))
	ctx = programscope.ContextWithScope(ctx)
	return ctx
}

func instanceStorageKey(pri *internal.ID, instID string) string {
	return fmt.Sprintf("%s.%s", pri.String(), instID)
}

func mustParseInstanceStorageKey(key string) (*internal.ID, string) {
	i := strings.LastIndexByte(key, '.')
	if i < 0 {
		z.Panic(fmt.Errorf("invalid instance storage key: %q", key))
	}

	pri := must(internal.ParseID(key[:i]))
	instID := key[i+1:]
	z.Check(ValidateInstanceUUIDForm(instID))
	return pri, instID
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

type Instance struct {
	id  string
	acc *account

	mu           lock.TagMutex[instanceLock] // Guards the fields below.
	model        *pb.Instance
	altProgImage *image.Program
	altCallMap   *object.CallMap
	image        *image.Instance
	process      *runtime.Process
	services     InstanceServices
	debugLog     io.WriteCloser
	stopped      chan struct{}
}

// newInstance steals instance image, process, and services.
func newInstance(id string, acc *account, transient bool, image *image.Instance, buffers *snapshot.Buffers, proc *runtime.Process, services InstanceServices, timeResolution time.Duration, tags []string, debugLog io.WriteCloser) *Instance {
	return &Instance{
		id:  id,
		acc: acc,
		model: &pb.Instance{
			Transient:      transient,
			Status:         new(api.Status),
			Buffers:        buffers,
			TimeResolution: durationpb.New(timeResolution),
			Tags:           tags,
		},
		image:    image,
		process:  proc,
		services: services,
		debugLog: debugLog,
		stopped:  make(chan struct{}),
	}
}

func (inst *Instance) ID() string {
	return inst.id
}

func (inst *Instance) store(lock instanceLock, prog *program) error {
	return inst.image.Store(instanceStorageKey(inst.acc.ID, inst.id), prog.id, prog.image)
}

func (inst *Instance) startOrAnnihilate(prog *program) (drive bool, err error) {
	lock := inst.mu.Lock()
	defer inst.mu.Unlock()

	if inst.process == nil {
		if err := inst.store(lock, prog); err != nil {
			return false, err
		}

		trapID := inst.image.Trap()

		if inst.image.Final() {
			if trapID != trap.Exit {
				inst.model.Status.State = api.StateKilled
				if trapID != trap.Killed {
					inst.model.Status.Cause = api.Cause(trapID)
				}
			} else {
				inst.model.Status.State = api.StateTerminated
				inst.model.Status.Result = inst.image.Result()
			}
		} else {
			if trapID != trap.Exit {
				inst.model.Status.State, inst.model.Status.Cause = trapStatus(trapID)
			} else if inst.image.EntryAddr() == 0 {
				inst.model.Status.State = api.StateHalted
				inst.model.Status.Result = inst.image.Result()
			} else {
				inst.model.Status.State = api.StateSuspended
			}
		}

		inst.model.Exists = true
		close(inst.stopped)
		return false, nil
	}

	progImage := prog.image
	if inst.altProgImage != nil {
		progImage = inst.altProgImage
	}

	policy := runtime.ProcessPolicy{
		TimeResolution: inst.model.TimeResolution.AsDuration(),
		DebugLog:       inst.debugLog,
	}

	if err := inst.process.Start(progImage, inst.image, policy); err != nil {
		inst.stop(lock)
		inst.image.Close()
		inst.image = nil
		return false, err
	}

	inst.model.Status.State = api.StateRunning
	inst.model.Exists = true
	return true, nil
}

func (inst *Instance) stop(lock instanceLock) {
	close(inst.stopped)

	inst.process.Close()

	inst.services.Close()
	inst.services = nil

	if inst.debugLog != nil {
		inst.debugLog.Close()
		inst.debugLog = nil
	}
}

func (inst *Instance) Status() *api.Status {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	return cloneStatus(inst.model.Status)
}

// info may return nil.
func (inst *Instance) info(module string) *api.InstanceInfo {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	if !inst.model.Exists {
		return nil
	}

	return &api.InstanceInfo{
		Instance:  inst.id,
		Module:    module,
		Status:    cloneStatus(inst.model.Status),
		Transient: inst.model.Transient,
		Debugging: len(inst.image.Breakpoints()) > 0,
		Tags:      inst.model.Tags,
	}
}

func (inst *Instance) Wait(ctx Context) (status *api.Status) {
	var stopped <-chan struct{}
	lock.GuardTag(&inst.mu, func(instanceLock) {
		status = cloneStatus(inst.model.Status)
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

func (inst *Instance) Kill(ctx Context) error {
	inst.kill()
	return nil
}

func (inst *Instance) kill() {
	proc := inst.getProcess()
	if proc == nil {
		return
	}

	proc.Kill()
}

// Suspend the instance and make it non-transient.
func (inst *Instance) Suspend(ctx Context) error {
	inst.suspend(true)
	return nil
}

func (inst *Instance) suspend(setNonTransient bool) {
	proc := lock.GuardTagged(&inst.mu, func(instanceLock) *runtime.Process {
		if setNonTransient && inst.model.Status.State == api.StateRunning {
			inst.model.Transient = false
		}
		return inst.process
	})
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

func (inst *Instance) mustCheckResume(function string) {
	lock := inst.mu.Lock()
	defer inst.mu.Unlock()

	inst.mustCheckResumeWithLock(lock, function)
}

func (inst *Instance) mustCheckResumeWithLock(lock instanceLock, function string) {
	if !inst.model.Exists {
		z.Panic(notfound.ErrInstance)
	}

	switch inst.model.Status.State {
	case api.StateSuspended:
		if function != "" {
			z.Panic(failrequest.Error(event.FailInstanceStatus, "function specified for suspended instance"))
		}

	case api.StateHalted:
		if function == "" {
			z.Panic(failrequest.Error(event.FailInstanceStatus, "function must be specified when resuming halted instance"))
		}

	default:
		z.Panic(failrequest.Error(event.FailInstanceStatus, "instance must be suspended or halted"))
	}
}

// mustResume steals proc, services and debugLog.
func (inst *Instance) mustResume(function string, proc *runtime.Process, services InstanceServices, timeResolution time.Duration, debugLog io.WriteCloser) {
	var ok bool
	defer func() {
		if !ok {
			debugLog.Close()
		}
	}()

	lock := inst.mu.Lock()
	defer inst.mu.Unlock()

	// Check again in case of a race condition.
	inst.mustCheckResumeWithLock(lock, function)

	inst.model.Status = &api.Status{State: api.StateRunning}
	inst.process = proc
	inst.services = services
	inst.model.TimeResolution = durationpb.New(timeResolution)
	inst.debugLog = debugLog
	inst.stopped = make(chan struct{})

	ok = true
}

// Connect to a running instance.  Disconnection happens when context is
// canceled, the instance stops running, or the program closes the connection.
func (inst *Instance) Connect(ctx Context, r io.Reader, w io.WriteCloser) error {
	wrote := false
	defer func() {
		if !wrote {
			w.Close()
		}
	}()

	conn := inst.connect(ctx)
	if conn == nil {
		return nil
	}

	wrote = true
	return conn(ctx, r, w)
}

func (inst *Instance) connect(ctx Context) func(Context, io.Reader, io.WriteCloser) error {
	s := lock.GuardTagged(&inst.mu, func(instanceLock) InstanceServices {
		return inst.services
	})
	if s == nil {
		return nil
	}

	return s.Connect(ctx)
}

func (inst *Instance) mustSnapshot(prog *program) (*image.Program, *snapshot.Buffers) {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	if !inst.model.Exists {
		z.Panic(notfound.ErrInstance)
	}
	if inst.model.Status.State == api.StateRunning {
		z.Panic(failrequest.Error(event.FailInstanceStatus, "instance must not be running"))
	}

	buffers := inst.model.Buffers
	progImage := must(image.Snapshot(prog.image, inst.image, buffers, inst.model.Status.State == api.StateSuspended))

	return progImage, buffers
}

// mustAnnihilate fails unless instance is stopped.
func (inst *Instance) mustAnnihilate() {
	lock := inst.mu.Lock()
	defer inst.mu.Unlock()

	if !inst.model.Exists {
		z.Panic(notfound.ErrInstance)
	}
	if inst.model.Status.State == api.StateRunning {
		z.Panic(failrequest.Error(event.FailInstanceStatus, "instance must not be running"))
	}

	inst.annihilate(lock)
}

func (inst *Instance) annihilate(lock instanceLock) {
	inst.model.Exists = false
	inst.image.Unstore()
	inst.image.Close()
	inst.image = nil
}

func (inst *Instance) drive(ctx Context, prog *program, function string, config *Config) (nonexistent bool) {
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
		inst.model.Status = res
		inst.stop(lock)

		config.eventInstance(ctx, event.TypeInstanceStop, &event.Instance{
			Instance: inst.id,
			Status:   cloneStatus(inst.model.Status),
		}, nil)

		if inst.model.Transient {
			inst.annihilate(lock)
			nonexistent = true

			config.eventInstance(ctx, event.TypeInstanceDelete, &event.Instance{
				Instance: inst.id,
			}, nil)
		}
	}
	defer func() {
		if cleanupFunc != nil {
			lock := inst.mu.Lock()
			defer inst.mu.Unlock()
			cleanupFunc(lock)
		}
	}()

	var (
		result runtime.Result
		err    error
	)

	result, trapID, inst.model.Buffers, err = inst.process.Serve(instanceServingContext(ctx, inst.id), inst.services, inst.model.Buffers)
	if err != nil {
		res.Error = api.PublicErrorString(err, res.Error)
		if trapID == trap.ABIViolation {
			res.Cause = api.CauseABIViolation
			config.eventFail(ctx, event.TypeFailRequest, &event.Fail{
				Type:     event.FailProgramError,
				Module:   prog.id,
				Function: function,
				Instance: inst.id,
			}, err)
		} else {
			config.eventFail(ctx, event.TypeFailInternal, internalFail(prog.id, function, inst.id, "service io", err), err)
		}
		return
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
		res.Error = api.PublicErrorString(mutErr, res.Error)
		config.eventFail(ctx, event.TypeFailInternal, internalFail(prog.id, function, inst.id, "image state", mutErr), mutErr)
		return
	}

	if mutErr == nil && !inst.model.Transient {
		err = inst.store(lock, prog)
		if err != nil {
			res.Error = api.PublicErrorString(err, res.Error)
			config.eventFail(ctx, event.TypeFailInternal, internalFail(prog.id, function, inst.id, "image storage", err), err)
			return
		}
	}

	if trapID == trap.Exit {
		if inst.model.Transient || result.Terminated() {
			res.State = api.StateTerminated
		} else {
			res.State = api.StateHalted
		}
		res.Cause = api.CauseNormal
		res.Result = int32(result.Value())
	} else {
		res.State, res.Cause = trapStatus(trapID)
	}

	return
}

// update mutates the argument's contents to reflect actual modifications.
func (inst *Instance) update(update *api.InstanceUpdate) (modified bool) {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	if !inst.model.Exists {
		return
	}

	if update.Persist && inst.model.Transient {
		inst.model.Transient = false
		modified = true
	} else {
		update.Persist = false
	}

	if len(update.Tags) != 0 && !reflect.DeepEqual(inst.model.Tags, update.Tags) {
		inst.model.Tags = append([]string(nil), update.Tags...)
		modified = true
	} else {
		update.Tags = nil
	}

	return
}

func (inst *Instance) mustDebug(ctx Context, prog *program, req *api.DebugRequest) (*instanceRebuild, *api.DebugConfig, *api.DebugResponse) {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	if req.Op < api.DebugOpConfigGet || req.Op > api.DebugOpReadStack {
		z.Panic(failrequest.Error(event.FailUnsupported, "unsupported debug op"))
	}

	if req.Op != api.DebugOpConfigGet && inst.model.Status.State == api.StateRunning {
		z.Panic(failrequest.Error(event.FailInstanceStatus, "instance must be stopped"))
	}

	breaks := inst.image.Breakpoints()
	modified := false
	var data []byte

	switch req.Op {
	case api.DebugOpConfigSet:
		config := req.GetConfig()

		if len(config.Breakpoints) > snapshot.MaxBreakpoints {
			z.Panic(failrequest.Error(event.FailResourceLimit, "too many breakpoints"))
		}

		breaks = dedup.SortUint64(config.Breakpoints)
		if !reflect.DeepEqual(breaks, inst.image.Breakpoints()) {
			modified = true
		}

	case api.DebugOpConfigUnion:
		config := req.GetConfig()

		if len(breaks)+len(config.Breakpoints) > snapshot.MaxBreakpoints {
			z.Panic(failrequest.Error(event.FailResourceLimit, "too many breakpoints"))
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
		callMap := inst.altCallMap
		if inst.altProgImage == nil {
			callMap = &prog.image.Map
		}
		data = must(inst.image.ExportStack(callMap))
	}

	var (
		rebuild   *instanceRebuild
		newConfig *api.DebugConfig
	)

	if modified {
		if reflect.DeepEqual(breaks, prog.image.Breakpoints()) {
			if inst.altProgImage != nil {
				inst.altProgImage.Close()
				inst.altProgImage = nil
				inst.altCallMap = nil
			}

			inst.image.SetBreakpoints(prog.image.Breakpoints())
		} else {
			rebuild = &instanceRebuild{
				inst:       inst,
				origProgID: prog.id,
				oldConfig: &api.DebugConfig{
					Breakpoints: inst.image.Breakpoints(),
				},
			}
			newConfig = &api.DebugConfig{
				Breakpoints: breaks,
			}
		}
	}

	res := &api.DebugResponse{
		Module: path.Join(api.KnownModuleSource, prog.id),
		Status: cloneStatus(inst.model.Status),
		Config: &api.DebugConfig{
			Breakpoints: inst.image.Breakpoints(),
		},
		Data: data,
	}

	return rebuild, newConfig, res
}

type instanceRebuild struct {
	inst       *Instance
	origProgID string
	oldConfig  *api.DebugConfig
}

func (rebuild *instanceRebuild) apply(progImage *image.Program, newConfig *api.DebugConfig, callMap *object.CallMap) (res *api.DebugResponse, ok bool) {
	inst := rebuild.inst
	oldConfig := rebuild.oldConfig

	inst.mu.Lock()
	defer inst.mu.Unlock()

	if reflect.DeepEqual(inst.image.Breakpoints(), oldConfig.Breakpoints) {
		if inst.altProgImage != nil {
			inst.altProgImage.Close()
		}
		inst.altProgImage = progImage
		inst.altCallMap = callMap

		inst.image.SetBreakpoints(newConfig.Breakpoints)
		ok = true
	}

	res = &api.DebugResponse{
		Module: path.Join(api.KnownModuleSource, rebuild.origProgID),
		Status: cloneStatus(inst.model.Status),
		Config: &api.DebugConfig{
			Breakpoints: inst.image.Breakpoints(),
		},
	}
	return res, ok
}

func internalFail(module, function, instance, subsys string, err error) *event.Fail {
	if s := subsystem.Get(err); s != "" {
		subsys = s
	}

	return &event.Fail{
		Module:    module,
		Function:  function,
		Instance:  instance,
		Subsystem: subsys,
	}
}

func cloneStatus(s *api.Status) *api.Status {
	if s == nil {
		return nil
	}
	return &api.Status{
		State:  s.State,
		Cause:  s.Cause,
		Result: s.Result,
		Error:  s.Error,
	}
}
