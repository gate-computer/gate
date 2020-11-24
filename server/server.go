// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"
	"io"
	"log"

	"gate.computer/gate/image"
	"gate.computer/gate/internal/error/public"
	"gate.computer/gate/internal/error/resourcelimit"
	"gate.computer/gate/internal/principal"
	"gate.computer/gate/runtime"
	"gate.computer/gate/server/api"
	"gate.computer/gate/server/detail"
	"gate.computer/gate/server/event"
	"gate.computer/gate/server/internal/error/resourcenotfound"
	"gate.computer/wag/object/stack"
)

const ErrServerClosed = public.Err("server closed")

var errAnonymous = AccessUnauthorized("anonymous access not supported")

type progPolicy struct {
	res  ResourcePolicy
	prog ProgramPolicy
}

type instPolicy struct {
	res  ResourcePolicy
	inst InstancePolicy
}

type instProgPolicy struct {
	res  ResourcePolicy
	prog ProgramPolicy
	inst InstancePolicy
}

type Server struct {
	Config

	mu        serverMutex
	programs  map[string]*program
	accounts  map[principal.RawKey]*account
	anonymous map[*Instance]struct{}
}

func New(ctx context.Context, config Config) (*Server, error) {
	if config.ImageStorage == nil {
		config.ImageStorage = image.Memory
	}
	if config.Monitor == nil {
		config.Monitor = defaultMonitor
	}
	if !config.Configured() {
		panic("incomplete server configuration")
	}

	s := &Server{
		Config:    config,
		programs:  make(map[string]*program),
		accounts:  make(map[principal.RawKey]*account),
		anonymous: make(map[*Instance]struct{}),
	}

	progs, err := s.ImageStorage.Programs()
	if err != nil {
		return nil, err
	}

	insts, err := s.ImageStorage.Instances()
	if err != nil {
		return nil, err
	}

	shutdown := s.Shutdown
	defer func() {
		if shutdown != nil {
			shutdown(context.Background())
		}
	}()

	lock := s.mu.Lock()
	defer s.mu.Unlock()

	var owner *account
	if id := principal.ContextID(ctx); id != nil {
		owner = newAccount(id)
		s.accounts[principal.Raw(id)] = owner
	}

	for _, id := range progs {
		if err := s.loadProgramDuringInit(lock, owner, id); err != nil {
			return nil, err
		}
	}

	for _, key := range insts {
		if err := s.loadInstanceDuringInit(lock, key); err != nil {
			return nil, err
		}
	}

	shutdown = nil
	return s, nil
}

func (s *Server) loadProgramDuringInit(lock serverLock, owner *account, progID string) error {
	image, err := s.ImageStorage.LoadProgram(progID)
	if err != nil {
		return err
	}
	if image == nil { // Race condition with human operator?
		return nil
	}
	defer closeProgramImage(&image)

	buffers, err := image.LoadBuffers()
	if err != nil {
		return err
	}

	prog := newProgram(progID, image, buffers, true)
	image = nil

	if owner != nil {
		owner.ensureProgramRef(lock, prog, nil)
	}

	s.programs[progID] = prog

	return nil
}

func (s *Server) loadInstanceDuringInit(lock serverLock, key string) error {
	image, err := s.ImageStorage.LoadInstance(key)
	if err != nil {
		return err
	}
	if image == nil { // Race condition with human operator?
		return nil
	}
	defer closeInstanceImage(&image)

	pri, instID, err := parseInstanceStorageKey(key)
	if err != nil {
		return err
	}

	acc := s.ensureAccount(lock, pri)

	// TODO: restore instance
	log.Printf("TODO: load account %s instance %s (%s)", acc.ID, instID, image.Trap())

	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	var (
		accInsts  []*Instance
		anonInsts map[*Instance]struct{}
	)
	s.mu.Guard(func(lock serverLock) {
		progs := s.programs
		s.programs = nil

		for _, prog := range progs {
			prog.unref(lock)
		}

		accs := s.accounts
		s.accounts = nil

		for _, acc := range accs {
			for _, x := range acc.shutdown(lock) {
				accInsts = append(accInsts, x.inst)
				x.prog.unref(lock)
			}
		}

		anonInsts = s.anonymous
		s.anonymous = nil
	})

	for _, inst := range accInsts {
		inst.suspend()
	}
	for inst := range anonInsts {
		inst.Kill()
	}

	var aborted bool

	for _, inst := range accInsts {
		if inst.Wait(ctx).State == api.StateRunning {
			aborted = true
		}
	}
	for inst := range anonInsts {
		inst.Wait(ctx)
	}

	if aborted {
		return ctx.Err()
	}
	return nil
}

func (s *Server) UploadModule(ctx context.Context, upload *ModuleUpload, know *api.ModuleOptions) (module string, err error) {
	know, err = prepareModuleOptions(know)
	if err != nil {
		return "", err
	}

	policy := new(progPolicy)

	ctx, err = s.AccessPolicy.AuthorizeProgram(ctx, &policy.res, &policy.prog)
	if err != nil {
		return "", err
	}

	if upload.Length > int64(policy.prog.MaxModuleSize) {
		return "", resourcelimit.New("module size limit exceeded")
	}

	// TODO: check resource policy

	if upload.Hash != "" {
		if found, err := s.loadKnownModule(ctx, policy, upload, know); err != nil {
			return "", err
		} else if found {
			return upload.Hash, nil
		}
	}

	return s.loadUnknownModule(ctx, policy, upload, know)
}

func (s *Server) SourceModule(ctx context.Context, source *ModuleSource, know *api.ModuleOptions) (module string, err error) {
	know, err = prepareModuleOptions(know)
	if err != nil {
		return "", err
	}

	policy := new(progPolicy)

	ctx, err = s.AccessPolicy.AuthorizeProgramSource(ctx, &policy.res, &policy.prog, source.Source)
	if err != nil {
		return "", err
	}

	stream, length, err := source.Source.OpenURI(ctx, source.URI, policy.prog.MaxModuleSize)
	if err != nil {
		return "", err
	}
	if stream == nil {
		if length > 0 {
			return "", resourcelimit.New("program size limit exceeded")
		}
		return "", resourcenotfound.ErrModule
	}

	upload := &ModuleUpload{
		Stream: stream,
		Length: length,
	}
	defer upload.Close()

	return s.loadUnknownModule(ctx, policy, upload, know)
}

func (s *Server) loadKnownModule(ctx context.Context, policy *progPolicy, upload *ModuleUpload, know *api.ModuleOptions) (bool, error) {
	prog, err := s.refProgram(upload.Hash, upload.Length)
	if prog == nil || err != nil {
		return false, err
	}
	defer s.unrefProgram(&prog)
	progID := prog.id

	if err := upload.validate(); err != nil {
		return true, err
	}

	if prog.image.TextSize() > policy.prog.MaxTextSize {
		return true, resourcelimit.New("program code size limit exceeded")
	}

	if _, err := s.registerProgramRef(ctx, prog, know); err != nil {
		return true, err
	}
	prog = nil

	s.monitor(&event.ModuleUploadExist{
		Ctx:    ContextDetail(ctx),
		Module: progID,
	})

	return true, nil
}

func (s *Server) loadUnknownModule(ctx context.Context, policy *progPolicy, upload *ModuleUpload, know *api.ModuleOptions) (string, error) {
	prog, _, err := buildProgram(s.ImageStorage, &policy.prog, nil, upload, "")
	if err != nil {
		return "", err
	}
	defer s.unrefProgram(&prog)
	progID := prog.id

	redundant, err := s.registerProgramRef(ctx, prog, know)
	if err != nil {
		return progID, err
	}
	prog = nil

	if redundant {
		s.monitor(&event.ModuleUploadExist{
			Ctx:      ContextDetail(ctx),
			Module:   progID,
			Compiled: true,
		})
	} else {
		s.monitor(&event.ModuleUploadNew{
			Ctx:    ContextDetail(ctx),
			Module: progID,
		})
	}

	return progID, nil
}

func (s *Server) NewInstance(ctx context.Context, module string, launch *api.LaunchOptions, invoke *InvokeOptions) (*Instance, error) {
	launch, err := prepareLaunchOptions(launch)
	if err != nil {
		return nil, err
	}

	policy := new(instPolicy)

	ctx, err = s.AccessPolicy.AuthorizeInstance(ctx, &policy.res, &policy.inst)
	if err != nil {
		return nil, err
	}

	acc, err := s.checkAccountInstanceID(ctx, launch.Instance)
	if err != nil {
		return nil, err
	}
	if acc == nil {
		return nil, errAnonymous
	}

	prog := s.mu.GuardProgram(func(lock serverLock) *program {
		prog := s.programs[module]
		if prog == nil {
			return nil
		}

		return acc.refProgram(lock, prog)
	})
	if prog == nil {
		return nil, resourcenotfound.ErrModule
	}
	defer s.unrefProgram(&prog)

	funcIndex, err := prog.image.ResolveEntryFunc(launch.Function, false)
	if err != nil {
		return nil, err
	}

	// TODO: check resource policy (text/stack/memory/max-memory size etc.)

	instImage, err := image.NewInstance(prog.image, policy.inst.MaxMemorySize, policy.inst.StackSize, funcIndex)
	if err != nil {
		return nil, err
	}
	defer closeInstanceImage(&instImage)

	ref := &api.ModuleOptions{}

	inst, prog, _, err := s.registerProgramRefInstance(ctx, acc, prog, instImage, &policy.inst, ref, launch, invoke)
	if err != nil {
		return nil, err
	}
	instImage = nil

	if err := s.runOrDeleteInstance(ctx, inst, prog, launch.Function); err != nil {
		return nil, err
	}
	prog = nil

	s.monitor(&event.InstanceCreateKnown{
		Ctx:    ContextDetail(ctx),
		Create: newInstanceCreateEvent(inst.ID, module, launch),
	})

	return inst, nil
}

func (s *Server) UploadModuleInstance(ctx context.Context, upload *ModuleUpload, know *api.ModuleOptions, launch *api.LaunchOptions, invoke *InvokeOptions) (*Instance, error) {
	know, err := prepareModuleOptions(know)
	if err != nil {
		return nil, err
	}
	launch, err = prepareLaunchOptions(launch)
	if err != nil {
		return nil, err
	}

	policy := new(instProgPolicy)

	ctx, err = s.AccessPolicy.AuthorizeProgramInstance(ctx, &policy.res, &policy.prog, &policy.inst)
	if err != nil {
		return nil, err
	}

	acc, err := s.checkAccountInstanceID(ctx, launch.Instance)
	if err != nil {
		return nil, err
	}

	_, inst, err := s.loadModuleInstance(ctx, acc, policy, upload, know, launch, invoke)
	return inst, err
}

func (s *Server) SourceModuleInstance(ctx context.Context, source *ModuleSource, know *api.ModuleOptions, launch *api.LaunchOptions, invoke *InvokeOptions) (module string, inst *Instance, err error) {
	know, err = prepareModuleOptions(know)
	if err != nil {
		return "", nil, err
	}
	launch, err = prepareLaunchOptions(launch)
	if err != nil {
		return "", nil, err
	}

	policy := new(instProgPolicy)

	ctx, err = s.AccessPolicy.AuthorizeProgramInstanceSource(ctx, &policy.res, &policy.prog, &policy.inst, source.Source)
	if err != nil {
		return "", nil, err
	}

	acc, err := s.checkAccountInstanceID(ctx, launch.Instance)
	if err != nil {
		return "", nil, err
	}

	stream, length, err := source.Source.OpenURI(ctx, source.URI, policy.prog.MaxModuleSize)
	if err != nil {
		return "", nil, err
	}
	if stream == nil {
		if length > 0 {
			return "", nil, resourcelimit.New("program size limit exceeded")
		}
		return "", nil, resourcenotfound.ErrModule
	}

	upload := &ModuleUpload{
		Stream: stream,
		Length: length,
	}
	defer upload.Close()

	return s.loadModuleInstance(ctx, acc, policy, upload, know, launch, invoke)
}

func (s *Server) loadModuleInstance(ctx context.Context, acc *account, policy *instProgPolicy, upload *ModuleUpload, know *api.ModuleOptions, launch *api.LaunchOptions, invoke *InvokeOptions) (string, *Instance, error) {
	launch, err := prepareLaunchOptions(launch)
	if err != nil {
		return "", nil, err
	}

	if upload.Length > int64(policy.prog.MaxModuleSize) {
		return "", nil, resourcelimit.New("module size limit exceeded")
	}

	// TODO: check resource policy

	if upload.Hash != "" {
		inst, err := s.loadKnownModuleInstance(ctx, acc, policy, upload, know, launch, invoke)
		if err != nil {
			return "", nil, err
		}
		if inst != nil {
			return upload.Hash, inst, nil
		}
	}

	return s.loadUnknownModuleInstance(ctx, acc, policy, upload, know, launch, invoke)
}

func (s *Server) loadKnownModuleInstance(ctx context.Context, acc *account, policy *instProgPolicy, upload *ModuleUpload, know *api.ModuleOptions, launch *api.LaunchOptions, invoke *InvokeOptions) (*Instance, error) {
	prog, err := s.refProgram(upload.Hash, upload.Length)
	if prog == nil || err != nil {
		return nil, err
	}
	defer s.unrefProgram(&prog)
	progID := prog.id

	if err := upload.validate(); err != nil {
		return nil, err
	}

	if prog.image.TextSize() > policy.prog.MaxTextSize {
		return nil, resourcelimit.New("program code size limit exceeded")
	}

	// TODO: check resource policy (stack/memory/max-memory size etc.)

	funcIndex, err := prog.image.ResolveEntryFunc(launch.Function, false)
	if err != nil {
		return nil, err
	}

	instImage, err := image.NewInstance(prog.image, policy.inst.MaxMemorySize, policy.inst.StackSize, funcIndex)
	if err != nil {
		return nil, err
	}
	defer closeInstanceImage(&instImage)

	inst, prog, _, err := s.registerProgramRefInstance(ctx, acc, prog, instImage, &policy.inst, know, launch, invoke)
	if err != nil {
		return nil, err
	}
	instImage = nil

	s.monitor(&event.ModuleUploadExist{
		Ctx:    ContextDetail(ctx),
		Module: progID,
	})

	if err := s.runOrDeleteInstance(ctx, inst, prog, launch.Function); err != nil {
		return nil, err
	}
	prog = nil

	s.monitor(&event.InstanceCreateKnown{
		Ctx:    ContextDetail(ctx),
		Create: newInstanceCreateEvent(inst.ID, progID, launch),
	})

	return inst, nil
}

func (s *Server) loadUnknownModuleInstance(ctx context.Context, acc *account, policy *instProgPolicy, upload *ModuleUpload, know *api.ModuleOptions, launch *api.LaunchOptions, invoke *InvokeOptions) (string, *Instance, error) {
	prog, instImage, err := buildProgram(s.ImageStorage, &policy.prog, &policy.inst, upload, launch.Function)
	if err != nil {
		return "", nil, err
	}
	defer closeInstanceImage(&instImage)
	defer s.unrefProgram(&prog)
	progID := prog.id

	inst, prog, redundantProg, err := s.registerProgramRefInstance(ctx, acc, prog, instImage, &policy.inst, know, launch, invoke)
	if err != nil {
		return "", nil, err
	}
	instImage = nil

	if upload.Hash != "" {
		if redundantProg {
			s.monitor(&event.ModuleUploadExist{
				Ctx:      ContextDetail(ctx),
				Module:   progID,
				Compiled: true,
			})
		} else {
			s.monitor(&event.ModuleUploadNew{
				Ctx:    ContextDetail(ctx),
				Module: progID,
			})
		}
	} else {
		if redundantProg {
			s.monitor(&event.ModuleSourceExist{
				Ctx:    ContextDetail(ctx),
				Module: progID,
				// TODO: source URI
				Compiled: true,
			})
		} else {
			s.monitor(&event.ModuleSourceNew{
				Ctx:    ContextDetail(ctx),
				Module: progID,
				// TODO: source URI
			})
		}
	}

	if err := s.runOrDeleteInstance(ctx, inst, prog, launch.Function); err != nil {
		return "", nil, err
	}
	prog = nil

	s.monitor(&event.InstanceCreateStream{
		Ctx:    ContextDetail(ctx),
		Create: newInstanceCreateEvent(inst.ID, progID, launch),
	})

	return progID, inst, nil
}

func (s *Server) ModuleInfo(ctx context.Context, module string) (*api.ModuleInfo, error) {
	ctx, err := s.AccessPolicy.Authorize(ctx)
	if err != nil {
		return nil, err
	}

	pri := principal.ContextID(ctx)
	if pri == nil {
		return nil, errAnonymous
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.programs == nil {
		return nil, ErrServerClosed
	}
	prog := s.programs[module]
	if prog == nil {
		return nil, resourcenotfound.ErrModule
	}

	acc := s.accounts[principal.Raw(pri)]
	if acc == nil {
		return nil, resourcenotfound.ErrModule
	}

	x, found := acc.programs[prog]
	if !found {
		return nil, resourcenotfound.ErrModule
	}

	info := &api.ModuleInfo{
		Id:   prog.id,
		Tags: append([]string(nil), x.tags...),
	}

	s.monitor(&event.ModuleInfo{
		Ctx:    ContextDetail(ctx),
		Module: prog.id,
	})

	return info, nil
}

func (s *Server) Modules(ctx context.Context) (*api.Modules, error) {
	ctx, err := s.AccessPolicy.Authorize(ctx)
	if err != nil {
		return nil, err
	}

	pri := principal.ContextID(ctx)
	if pri == nil {
		return nil, errAnonymous
	}

	s.monitor(&event.ModuleList{
		Ctx: ContextDetail(ctx),
	})

	s.mu.Lock()
	defer s.mu.Unlock()

	acc := s.accounts[principal.Raw(pri)]
	if acc == nil {
		return new(api.Modules), nil
	}

	infos := &api.Modules{
		Modules: make([]*api.ModuleInfo, 0, len(acc.programs)),
	}
	for prog, x := range acc.programs {
		infos.Modules = append(infos.Modules, &api.ModuleInfo{
			Id:   prog.id,
			Tags: append([]string(nil), x.tags...),
		})
	}
	return infos, nil
}

func (s *Server) ModuleContent(ctx context.Context, module string) (stream io.ReadCloser, length int64, err error) {
	ctx, err = s.AccessPolicy.Authorize(ctx)
	if err != nil {
		return nil, 0, err
	}

	pri := principal.ContextID(ctx)
	if pri == nil {
		return nil, 0, errAnonymous
	}

	prog := s.mu.GuardProgram(func(lock serverLock) *program {
		acc := s.accounts[principal.Raw(pri)]
		if acc == nil {
			return nil
		}

		prog := s.programs[module]
		if prog == nil {
			return nil
		}

		return acc.refProgram(lock, prog)
	})
	if prog == nil {
		return nil, 0, resourcenotfound.ErrModule
	}

	length = prog.image.ModuleSize()
	stream = &moduleContent{
		ctx:   ContextDetail(ctx),
		r:     prog.image.NewModuleReader(),
		s:     s,
		prog:  prog,
		total: length,
	}
	return stream, length, nil
}

type moduleContent struct {
	ctx   *detail.Context
	r     io.Reader
	s     *Server
	prog  *program
	total int64
	read  int64
}

func (x *moduleContent) Read(b []byte) (int, error) {
	n, err := x.r.Read(b)
	x.read += int64(n)
	return n, err
}

func (x *moduleContent) Close() error {
	x.s.monitor(&event.ModuleDownload{
		Ctx:          x.ctx,
		Module:       x.prog.id,
		ModuleLength: uint64(x.total),
		LengthRead:   uint64(x.read),
	})

	x.s.unrefProgram(&x.prog)
	return nil
}

func (s *Server) PinModule(ctx context.Context, module string, know *api.ModuleOptions) error {
	if !know.GetPin() {
		panic("Server.PinModule called without ModuleOptions.Pin")
	}
	know, err := prepareModuleOptions(know)
	if err != nil {
		return err
	}

	policy := new(progPolicy)

	ctx, err = s.AccessPolicy.AuthorizeProgram(ctx, &policy.res, &policy.prog)
	if err != nil {
		return err
	}

	pri := principal.ContextID(ctx)
	if pri == nil {
		return errAnonymous
	}

	var modified bool

	err = s.mu.GuardError(func(lock serverLock) error {
		if s.programs == nil {
			return ErrServerClosed
		}
		prog := s.programs[module]
		if prog == nil {
			return resourcenotfound.ErrModule
		}

		acc := s.accounts[principal.Raw(pri)]
		if acc == nil {
			return resourcenotfound.ErrModule
		}

		if _, found := acc.programs[prog]; !found {
			for _, x := range acc.instances {
				if x.prog == prog {
					goto do
				}
			}
			return resourcenotfound.ErrModule
		}

	do:
		// TODO: check resource limits
		modified = acc.ensureProgramRef(lock, prog, know.Tags)
		return nil
	})
	if err != nil {
		return err
	}

	if modified {
		s.monitor(&event.ModulePin{
			Ctx:      ContextDetail(ctx),
			Module:   module,
			TagCount: int32(len(know.Tags)),
		})
	}

	return nil
}

func (s *Server) UnpinModule(ctx context.Context, module string) error {
	ctx, err := s.AccessPolicy.Authorize(ctx)
	if err != nil {
		return err
	}

	pri := principal.ContextID(ctx)
	if pri == nil {
		return errAnonymous
	}

	found := s.mu.GuardBool(func(lock serverLock) bool {
		acc := s.accounts[principal.Raw(pri)]
		if acc == nil {
			return false
		}

		prog := s.programs[module]
		if prog == nil {
			return false
		}

		return acc.unrefProgram(lock, prog)
	})
	if !found {
		return resourcenotfound.ErrModule
	}

	s.monitor(&event.ModuleUnpin{
		Ctx:    ContextDetail(ctx),
		Module: module,
	})

	return nil
}

type IOFunc func(context.Context, io.Reader, io.Writer) error

func (s *Server) InstanceConnection(ctx context.Context, instance string) (*Instance, IOFunc, error) {
	ctx, err := s.AccessPolicy.Authorize(ctx)
	if err != nil {
		return nil, nil, err
	}

	inst, err := s.getInstance(ctx, instance)
	if err != nil {
		return nil, nil, err
	}

	conn := inst.connect(ctx)
	if conn == nil {
		s.monitor(&event.FailRequest{
			Ctx:      ContextDetail(ctx),
			Failure:  event.FailInstanceNoConnect,
			Instance: inst.ID,
		})
		return inst, nil, nil
	}

	ioFunc := func(ctx context.Context, r io.Reader, w io.Writer) error {
		s.monitor(&event.InstanceConnect{
			Ctx:      ContextDetail(ctx),
			Instance: inst.ID,
		})

		err := conn(ctx, r, w)

		s.Monitor(&event.InstanceDisconnect{
			Ctx:      ContextDetail(ctx),
			Instance: inst.ID,
		}, err)

		return err
	}

	return inst, ioFunc, nil
}

func (s *Server) InstanceInfo(ctx context.Context, instance string) (*api.InstanceInfo, error) {
	ctx, err := s.AccessPolicy.Authorize(ctx)
	if err != nil {
		return nil, err
	}

	progID, inst, err := s.getInstanceProgramID(ctx, instance)
	if err != nil {
		return nil, err
	}

	info, err := inst.info(progID)
	if err != nil {
		return nil, err
	}

	s.monitor(&event.InstanceInfo{
		Ctx:      ContextDetail(ctx),
		Instance: inst.ID,
	})

	return info, nil
}

func (s *Server) WaitInstance(ctx context.Context, instID string) (*api.Status, error) {
	ctx, err := s.AccessPolicy.Authorize(ctx)
	if err != nil {
		return nil, err
	}

	inst, err := s.getInstance(ctx, instID)
	if err != nil {
		return nil, err
	}

	s.monitor(&event.InstanceWait{
		Ctx:      ContextDetail(ctx),
		Instance: inst.ID,
	})

	return inst.Wait(ctx), err
}

func (s *Server) KillInstance(ctx context.Context, instance string) (*Instance, error) {
	ctx, err := s.AccessPolicy.Authorize(ctx)
	if err != nil {
		return nil, err
	}

	inst, err := s.getInstance(ctx, instance)
	if err != nil {
		return nil, err
	}

	s.monitor(&event.InstanceKill{
		Ctx:      ContextDetail(ctx),
		Instance: inst.ID,
	})

	inst.Kill()
	return inst, nil
}

func (s *Server) SuspendInstance(ctx context.Context, instance string) (*Instance, error) {
	ctx, err := s.AccessPolicy.Authorize(ctx)
	if err != nil {
		return nil, err
	}

	// Store the program in case the instance becomes non-transient.
	inst, prog, err := s.getInstanceRefProgram(ctx, instance)
	if err != nil {
		return nil, err
	}
	defer s.unrefProgram(&prog)

	if err := prog.ensureStorage(); err != nil {
		return nil, err
	}

	s.monitor(&event.InstanceSuspend{
		Ctx:      ContextDetail(ctx),
		Instance: inst.ID,
	})

	inst.Suspend()
	return inst, nil
}

func (s *Server) ResumeInstance(ctx context.Context, instance string, resume *api.ResumeOptions, invoke *InvokeOptions) (*Instance, error) {
	resume = prepareResumeOptions(resume)
	policy := new(instPolicy)

	ctx, err := s.AccessPolicy.AuthorizeInstance(ctx, &policy.res, &policy.inst)
	if err != nil {
		return nil, err
	}

	inst, prog, err := s.getInstanceRefProgram(ctx, instance)
	if err != nil {
		return nil, err
	}
	defer s.unrefProgram(&prog)

	if err := inst.checkResume(resume.Function); err != nil {
		return nil, err
	}

	proc, services, err := s.allocateInstanceResources(ctx, &policy.inst)
	if err != nil {
		return nil, err
	}
	defer closeInstanceResources(&proc, &services)

	if err := inst.doResume(resume.Function, proc, services, policy.inst.TimeResolution, invoke); err != nil {
		return nil, err
	}
	proc = nil
	services = nil

	if err := s.runOrDeleteInstance(ctx, inst, prog, resume.Function); err != nil {
		return nil, err
	}
	prog = nil

	s.monitor(&event.InstanceResume{
		Ctx:      ContextDetail(ctx),
		Instance: inst.ID,
		Function: resume.Function,
	})

	return inst, nil
}

func (s *Server) DeleteInstance(ctx context.Context, instance string) error {
	ctx, err := s.AccessPolicy.Authorize(ctx)
	if err != nil {
		return err
	}

	inst, err := s.getInstance(ctx, instance)
	if err != nil {
		return err
	}

	if err := inst.annihilate(); err != nil {
		return err
	}

	s.monitor(&event.InstanceDelete{
		Ctx:      ContextDetail(ctx),
		Instance: inst.ID,
	})

	s.deleteNonexistentInstance(inst)
	return nil
}

func (s *Server) SnapshotInstance(ctx context.Context, instance string, know *api.ModuleOptions) (module string, err error) {
	if !know.GetPin() {
		panic("Server.SnapshotInstance called without ModuleOptions.Pin")
	}
	know, err = prepareModuleOptions(know)
	if err != nil {
		return "", err
	}

	inst, err := s.getInstance(ctx, instance)
	if err != nil {
		return "", err
	}

	// TODO: implement suspend-snapshot-resume at a lower level

	resume := false
	if inst.Status().State == api.StateRunning {
		inst.suspend()
		s := inst.Wait(context.Background())
		resume = s.State == api.StateSuspended
	}

	module, err = s.snapshot(ctx, instance, know)
	if resume {
		if _, e := s.ResumeInstance(ctx, instance, nil, nil); err == nil {
			err = e
		}
	}
	return module, err
}

func (s *Server) snapshot(ctx context.Context, instance string, know *api.ModuleOptions) (string, error) {
	ctx, err := s.AccessPolicy.Authorize(ctx)
	if err != nil {
		return "", err
	}

	// TODO: check module storage limits

	inst, oldProg, err := s.getInstanceRefProgram(ctx, instance)
	if err != nil {
		return "", err
	}
	defer s.unrefProgram(&oldProg)

	newImage, buffers, err := inst.snapshot(oldProg)
	if err != nil {
		return "", err
	}
	defer closeProgramImage(&newImage)

	h := api.KnownModuleHash.New()
	if _, err := io.Copy(h, newImage.NewModuleReader()); err != nil {
		return "", err
	}
	progID := api.EncodeKnownModule(h.Sum(nil))

	if err := newImage.Store(progID); err != nil {
		return "", err
	}

	newProg := newProgram(progID, newImage, buffers, true)
	newImage = nil
	defer s.unrefProgram(&newProg)

	if _, err := s.registerProgramRef(ctx, newProg, know); err != nil {
		return "", err
	}
	newProg = nil

	s.monitor(&event.InstanceSnapshot{
		Ctx:      ContextDetail(ctx),
		Instance: inst.ID,
		Module:   progID,
	})

	return progID, nil
}

func (s *Server) UpdateInstance(ctx context.Context, instance string, update *api.InstanceUpdate) (*api.InstanceInfo, error) {
	update = prepareInstanceUpdate(update)

	ctx, err := s.AccessPolicy.Authorize(ctx)
	if err != nil {
		return nil, err
	}

	progID, inst, err := s.getInstanceProgramID(ctx, instance)
	if err != nil {
		return nil, err
	}

	if inst.update(update) {
		s.monitor(&event.InstanceUpdate{
			Ctx:      ContextDetail(ctx),
			Instance: inst.ID,
			Persist:  update.Persist,
			TagCount: int32(len(update.Tags)),
		})
	}

	return inst.info(progID)
}

func (s *Server) DebugInstance(ctx context.Context, instance string, req *api.DebugRequest) (*api.DebugResponse, error) {
	policy := new(progPolicy)

	ctx, err := s.AccessPolicy.AuthorizeProgram(ctx, &policy.res, &policy.prog)
	if err != nil {
		return nil, err
	}

	inst, defaultProg, err := s.getInstanceRefProgram(ctx, instance)
	if err != nil {
		return nil, err
	}
	defer s.unrefProgram(&defaultProg)

	rebuild, config, res, err := inst.debug(ctx, defaultProg, req)
	if err != nil {
		return nil, err
	}

	if rebuild != nil {
		var (
			progImage *image.Program
			textMap   stack.TextMap
			ok        bool
		)

		progImage, textMap, err = rebuildProgramImage(s.ImageStorage, &policy.prog, defaultProg.image.NewModuleReader(), config.DebugInfo, config.Breakpoints)
		if err != nil {
			return nil, err
		}
		defer func() {
			if progImage != nil {
				progImage.Close()
			}
		}()

		res, ok = rebuild.apply(progImage, config, textMap)
		if !ok {
			return nil, public.Err("conflict") // TODO: http response code: conflict
		}
		progImage = nil
	}

	s.monitor(&event.InstanceDebug{
		Ctx:      ContextDetail(ctx),
		Instance: inst.ID,
		Compiled: rebuild != nil,
	})

	return res, nil
}

func (s *Server) Instances(ctx context.Context) (*api.Instances, error) {
	ctx, err := s.AccessPolicy.Authorize(ctx)
	if err != nil {
		return nil, err
	}

	pri := principal.ContextID(ctx)
	if pri == nil {
		return nil, errAnonymous
	}

	s.monitor(&event.InstanceList{
		Ctx: ContextDetail(ctx),
	})

	type instProgID struct {
		inst   *Instance
		progID string
	}

	// Get instance references while holding server lock.
	var insts []instProgID
	s.mu.Guard(func(lock serverLock) {
		if acc := s.accounts[principal.Raw(pri)]; acc != nil {
			insts = make([]instProgID, 0, len(acc.instances))
			for _, x := range acc.instances {
				insts = append(insts, instProgID{x.inst, x.prog.id})
			}
		}
	})

	// Each instance has its own lock.
	infos := &api.Instances{
		Instances: make([]*api.InstanceInfo, 0, len(insts)),
	}
	for _, x := range insts {
		if info, err := x.inst.info(x.progID); err == nil {
			infos.Instances = append(infos.Instances, info)
		}
	}
	return infos, nil
}

// ensureAccount must not be called while the server is shutting down.
func (s *Server) ensureAccount(_ serverLock, pri *principal.ID) *account {
	acc := s.accounts[principal.Raw(pri)]
	if acc == nil {
		acc = newAccount(pri)
		s.accounts[principal.Raw(pri)] = acc
	}
	return acc
}

func (s *Server) refProgram(hash string, length int64) (*program, error) {
	lock := s.mu.Lock()
	defer s.mu.Unlock()

	prog := s.programs[hash]
	if prog == nil {
		return nil, nil
	}

	if length != prog.image.ModuleSize() {
		return nil, errModuleSizeMismatch
	}

	return prog.ref(lock), nil
}

func (s *Server) unrefProgram(p **program) {
	prog := *p
	*p = nil
	if prog == nil {
		return
	}

	s.mu.Guard(prog.unref)
}

// registerProgramRef with the server and an account.  Caller's program
// reference is stolen (except on error).
func (s *Server) registerProgramRef(ctx context.Context, prog *program, know *api.ModuleOptions) (redundant bool, err error) {
	var pri *principal.ID

	if know.Pin {
		pri = principal.ContextID(ctx)
		if pri == nil {
			return false, errAnonymous
		}

		if err := prog.ensureStorage(); err != nil {
			return false, err
		}
	}

	lock := s.mu.Lock()
	defer s.mu.Unlock()

	prog, redundant, err = s.mergeProgramRef(lock, prog)
	if err != nil {
		return false, err
	}

	if know.Pin {
		// mergeProgramRef checked for shutdown, so the ensure methods are safe
		// to call.
		if s.ensureAccount(lock, pri).ensureProgramRef(lock, prog, know.Tags) {
			// TODO: move outside of critical section
			s.monitor(&event.ModulePin{
				Ctx:      ContextDetail(ctx),
				Module:   prog.id,
				TagCount: int32(len(know.Tags)),
			})
		}
	}

	return redundant, nil
}

func (s *Server) checkAccountInstanceID(ctx context.Context, instID string) (*account, error) {
	if instID != "" {
		if err := validateInstanceID(instID); err != nil {
			return nil, err
		}
	}

	pri := principal.ContextID(ctx)
	if pri == nil {
		return nil, nil
	}

	lock := s.mu.Lock()
	defer s.mu.Unlock()

	if s.accounts == nil {
		return nil, ErrServerClosed
	}

	acc := s.ensureAccount(lock, pri)

	if instID != "" {
		if err := acc.checkUniqueInstanceID(lock, instID); err != nil {
			return nil, err
		}
	}

	return acc, nil
}

// runOrDeleteInstance steals the program reference (except on error).
func (s *Server) runOrDeleteInstance(ctx context.Context, inst *Instance, prog *program, function string) error {
	defer s.unrefProgram(&prog)

	drive, err := inst.startOrAnnihilate(prog)
	if err != nil {
		s.deleteNonexistentInstance(inst)
		return err
	}

	if drive {
		go s.driveInstance(detachedContext(ctx), inst, prog, function)
		prog = nil
	}

	return nil
}

// driveInstance steals the program reference.
func (s *Server) driveInstance(ctx context.Context, inst *Instance, prog *program, function string) {
	defer s.unrefProgram(&prog)

	if nonexistent := inst.drive(ctx, prog, function, s.Monitor); nonexistent {
		s.deleteNonexistentInstance(inst)
	}
}

func (s *Server) getInstance(ctx context.Context, instance string) (*Instance, error) {
	_, inst, err := s.getInstanceProgramID(ctx, instance)
	return inst, err
}

func (s *Server) getInstanceProgramID(ctx context.Context, instance string) (string, *Instance, error) {
	pri := principal.ContextID(ctx)
	if pri == nil {
		return "", nil, errAnonymous
	}

	lock := s.mu.Lock()
	defer s.mu.Unlock()

	inst, prog, err := s.getInstanceBorrowProgram(lock, pri, instance)
	if err != nil {
		return "", nil, err
	}

	return prog.id, inst, nil
}

func (s *Server) getInstanceRefProgram(ctx context.Context, instance string) (*Instance, *program, error) {
	pri := principal.ContextID(ctx)
	if pri == nil {
		return nil, nil, errAnonymous
	}

	lock := s.mu.Lock()
	defer s.mu.Unlock()

	inst, prog, err := s.getInstanceBorrowProgram(lock, pri, instance)
	if err != nil {
		return nil, nil, err
	}

	return inst, prog.ref(lock), nil
}

func (s *Server) getInstanceBorrowProgram(_ serverLock, pri *principal.ID, instance string) (*Instance, *program, error) {
	acc := s.accounts[principal.Raw(pri)]
	if acc == nil {
		return nil, nil, resourcenotfound.ErrInstance
	}

	x, found := acc.instances[instance]
	if !found {
		return nil, nil, resourcenotfound.ErrInstance
	}

	return x.inst, x.prog, nil
}

func (s *Server) allocateInstanceResources(ctx context.Context, policy *InstancePolicy) (*runtime.Process, InstanceServices, error) {
	if policy.Services == nil {
		return nil, nil, AccessForbidden("no service policy")
	}

	services := policy.Services(ctx)
	defer func() {
		if services != nil {
			services.Close()
		}
	}()

	proc, err := s.ProcessFactory.NewProcess(ctx)
	if err != nil {
		return nil, nil, err
	}

	ss := services
	services = nil
	return proc, ss, nil
}

// registerProgramRefInstance with server, and an account if ref is true.
// Caller's instance image is stolen (except on error).  Caller's program
// reference is replaced with a reference to the canonical program object.
func (s *Server) registerProgramRefInstance(ctx context.Context, acc *account, prog *program, instImage *image.Instance, policy *InstancePolicy, know *api.ModuleOptions, launch *api.LaunchOptions, invoke *InvokeOptions) (inst *Instance, canonicalProg *program, redundantProg bool, err error) {
	var (
		proc     *runtime.Process
		services InstanceServices
	)
	if !launch.Suspend && !instImage.Final() {
		proc, services, err = s.allocateInstanceResources(ctx, policy)
		if err != nil {
			return
		}
		defer closeInstanceResources(&proc, &services)
	}

	if know.Pin || !launch.Transient {
		if acc == nil {
			err = errAnonymous
			return
		}

		err = prog.ensureStorage()
		if err != nil {
			return
		}
	}

	instance := launch.Instance
	if instance == "" {
		instance = makeInstanceID()
	}

	lock := s.mu.Lock()
	defer s.mu.Unlock()

	if acc != nil {
		if s.accounts == nil {
			err = ErrServerClosed
			return
		}

		err = acc.checkUniqueInstanceID(lock, instance)
		if err != nil {
			return
		}
	}

	prog, redundantProg, err = s.mergeProgramRef(lock, prog)
	if err != nil {
		return
	}

	inst = newInstance(instance, acc, launch.Transient, instImage, prog.buffers, proc, services, policy.TimeResolution, launch.Tags, invoke)
	proc = nil
	services = nil

	if acc != nil {
		if know.Pin {
			// mergeProgramRef checked for shutdown, so ensureProgramRef is
			// safe to call.
			if acc.ensureProgramRef(lock, prog, know.Tags) {
				// TODO: move outside of critical section
				s.monitor(&event.ModulePin{
					Ctx:      ContextDetail(ctx),
					Module:   prog.id,
					TagCount: int32(len(know.Tags)),
				})
			}
		}
		acc.instances[instance] = accountInstance{inst, prog.ref(lock)}
	} else {
		s.anonymous[inst] = struct{}{}
	}

	canonicalProg = prog.ref(lock)
	return
}

func (s *Server) deleteNonexistentInstance(inst *Instance) {
	lock := s.mu.Lock()
	defer s.mu.Unlock()

	if inst.acc != nil {
		if x := inst.acc.instances[inst.ID]; x.inst == inst {
			delete(inst.acc.instances, inst.ID)
			x.prog.unref(lock)
		}
	} else {
		delete(s.anonymous, inst)
	}
}

// mergeProgramRef steals the program reference and returns a borrowed program
// reference which is valid until the server mutex is unlocked.
func (s *Server) mergeProgramRef(lock serverLock, prog *program) (canonical *program, redundant bool, err error) {
	switch existing := s.programs[prog.id]; existing {
	case nil:
		if s.programs == nil {
			return nil, false, ErrServerClosed
		}
		s.programs[prog.id] = prog // Pass reference to map.
		return prog, false, nil

	case prog:
		if prog.refCount < 2 {
			panic("unexpected program reference count")
		}
		prog.unref(lock) // Map has reference; safe to drop temporary reference.
		return prog, false, nil

	default:
		prog.unref(lock)
		return existing, true, nil
	}
}

func prepareModuleOptions(opt *api.ModuleOptions) (*api.ModuleOptions, error) {
	if opt == nil {
		return new(api.ModuleOptions), nil
	}
	return opt, nil
}

func prepareLaunchOptions(opt *api.LaunchOptions) (*api.LaunchOptions, error) {
	if opt == nil {
		return new(api.LaunchOptions), nil
	}
	if opt.Suspend && opt.Function != "" {
		return nil, public.Err("function cannot be specified for suspended instance")
	}
	return opt, nil
}

func prepareResumeOptions(opt *api.ResumeOptions) *api.ResumeOptions {
	if opt == nil {
		return new(api.ResumeOptions)
	}
	return opt
}

func prepareInstanceUpdate(opt *api.InstanceUpdate) *api.InstanceUpdate {
	if opt == nil {
		return new(api.InstanceUpdate)
	}
	return opt
}

func closeProgramImage(p **image.Program) {
	if *p != nil {
		(*p).Close()
		*p = nil
	}
}

func closeInstanceImage(p **image.Instance) {
	if *p != nil {
		(*p).Close()
		*p = nil
	}
}

func closeInstanceResources(proc **runtime.Process, services *InstanceServices) {
	if *proc != nil {
		(*proc).Close()
		*proc = nil
	}
	if *services != nil {
		(*services).Close()
		*services = nil
	}
}

func newInstanceCreateEvent(instance, module string, launch *api.LaunchOptions) *event.InstanceCreate {
	return &event.InstanceCreate{
		Instance:  instance,
		Module:    module,
		Transient: launch.Transient,
		Suspended: launch.Suspend,
		TagCount:  int32(len(launch.Tags)),
	}
}
