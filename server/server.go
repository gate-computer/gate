// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"
	"errors"
	"io"
	"log"
	"strings"

	"gate.computer/gate/image"
	"gate.computer/gate/internal/error/resourcelimit"
	"gate.computer/gate/internal/principal"
	"gate.computer/gate/runtime"
	"gate.computer/gate/scope"
	"gate.computer/gate/server/api"
	"gate.computer/gate/server/event"
	"gate.computer/gate/server/internal"
	"gate.computer/gate/server/internal/error/failrequest"
	"gate.computer/gate/server/internal/error/notfound"
	"gate.computer/gate/server/internal/monitor"
	"gate.computer/wag/object"
	"import.name/pan"
)

var ErrServerClosed = errors.New("server closed")

var errAnonymous = Unauthenticated("anonymous access not supported")

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

type privateConfig struct {
	Config
}

type Server struct {
	privateConfig

	mu        serverMutex
	programs  map[string]*program
	accounts  map[principal.RawKey]*account
	anonymous map[*Instance]struct{}
}

func New(ctx context.Context, config *Config) (_ *Server, err error) {
	if internal.DontPanic() {
		defer func() { err = pan.Error(recover()) }()
	}

	s := &Server{
		programs:  make(map[string]*program),
		accounts:  make(map[principal.RawKey]*account),
		anonymous: make(map[*Instance]struct{}),
	}

	if config != nil {
		s.Config = *config
	}
	if s.ImageStorage == nil {
		s.ImageStorage = image.Memory
	}
	if s.Monitor == nil {
		s.Monitor = monitor.Default
	}
	if !s.Configured() {
		panic("incomplete server configuration")
	}

	progs, err := s.ImageStorage.Programs()
	_check(err)

	insts, err := s.ImageStorage.Instances()
	_check(err)

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
		s._loadProgramDuringInit(lock, owner, id)
	}

	for _, key := range insts {
		s._loadInstanceDuringInit(lock, key)
	}

	shutdown = nil
	return s, nil
}

func (s *Server) _loadProgramDuringInit(lock serverLock, owner *account, progID string) {
	image, err := s.ImageStorage.LoadProgram(progID)
	_check(err)
	if image == nil { // Race condition with human operator?
		return
	}
	defer closeProgramImage(&image)

	buffers, err := image.LoadBuffers()
	_check(err)

	prog := newProgram(progID, image, buffers, true)
	image = nil

	if owner != nil {
		owner.ensureProgramRef(lock, prog, nil)
	}

	s.programs[progID] = prog
}

func (s *Server) _loadInstanceDuringInit(lock serverLock, key string) {
	image, err := s.ImageStorage.LoadInstance(key)
	_check(err)
	if image == nil { // Race condition with human operator?
		return
	}
	defer closeInstanceImage(&image)

	pri, instID := _parseInstanceStorageKey(key)
	acc := s.ensureAccount(lock, pri)

	// TODO: restore instance
	log.Printf("TODO: load account %s instance %s (%s)", acc.ID, instID, image.Trap())
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
		inst.kill()
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

func (s *Server) Features() *api.Features {
	sources := make([]string, 0, len(s.ModuleSources))
	for s := range s.ModuleSources {
		sources = append(sources, s)
	}

	return &api.Features{
		Scope:         scope.Names(),
		ModuleSources: sources,
	}
}

func (s *Server) UploadModule(ctx context.Context, upload *api.ModuleUpload, know *api.ModuleOptions) (module string, err error) {
	if internal.DontPanic() {
		defer func() { err = pan.Error(recover()) }()
	}

	know = _prepareModuleOptions(know)

	policy := new(progPolicy)
	ctx = _context(s.AccessPolicy.AuthorizeProgram(ctx, &policy.res, &policy.prog))

	if upload.Length > int64(policy.prog.MaxModuleSize) {
		_check(resourcelimit.Error("module size limit exceeded"))
	}

	// TODO: check resource policy

	if upload.Hash != "" && s._loadKnownModule(ctx, policy, upload, know) {
		return upload.Hash, nil
	}

	return s._loadUnknownModule(ctx, policy, upload, know), nil
}

func (s *Server) SourceModule(ctx context.Context, uri string, know *api.ModuleOptions) (module string, err error) {
	if internal.DontPanic() {
		defer func() { err = pan.Error(recover()) }()
	}

	source, prefix := s._getSource(uri)
	know = _prepareModuleOptions(know)

	policy := new(progPolicy)
	ctx = _context(s.AccessPolicy.AuthorizeProgramSource(ctx, &policy.res, &policy.prog, prefix))

	stream, length, err := source.OpenURI(ctx, uri, policy.prog.MaxModuleSize)
	_check(err)
	if stream == nil {
		if length > 0 {
			_check(resourcelimit.Error("program size limit exceeded"))
		}
		_check(notfound.ErrModule)
	}

	upload := &api.ModuleUpload{
		Stream: stream,
		Length: length,
	}
	defer upload.Close()

	return s._loadUnknownModule(ctx, policy, upload, know), nil
}

func (s *Server) _loadKnownModule(ctx context.Context, policy *progPolicy, upload *api.ModuleUpload, know *api.ModuleOptions) bool {
	prog := s._refProgram(upload.Hash, upload.Length)
	if prog == nil {
		return false
	}
	defer s.unrefProgram(&prog)
	progID := prog.id

	_validateUpload(upload)

	if prog.image.TextSize() > policy.prog.MaxTextSize {
		_check(resourcelimit.Error("program code size limit exceeded"))
	}

	s._registerProgramRef(ctx, prog, know)
	prog = nil

	s.monitorModule(ctx, event.TypeModuleUploadExist, &event.Module{
		Module: progID,
	})

	return true
}

func (s *Server) _loadUnknownModule(ctx context.Context, policy *progPolicy, upload *api.ModuleUpload, know *api.ModuleOptions) string {
	prog, _ := _buildProgram(s.ImageStorage, &policy.prog, nil, upload, "")
	defer s.unrefProgram(&prog)
	progID := prog.id

	redundant := s._registerProgramRef(ctx, prog, know)
	prog = nil

	if redundant {
		s.monitorModule(ctx, event.TypeModuleUploadExist, &event.Module{
			Module:   progID,
			Compiled: true,
		})
	} else {
		s.monitorModule(ctx, event.TypeModuleUploadNew, &event.Module{
			Module: progID,
		})
	}

	return progID
}

func (s *Server) NewInstance(ctx context.Context, module string, launch *api.LaunchOptions) (_ api.Instance, err error) {
	if internal.DontPanic() {
		defer func() { err = pan.Error(recover()) }()
	}

	launch = _prepareLaunchOptions(launch)

	policy := new(instPolicy)
	ctx = _context(s.AccessPolicy.AuthorizeInstance(ctx, &policy.res, &policy.inst))

	acc := s._checkAccountInstanceID(ctx, launch.Instance)
	if acc == nil {
		_check(errAnonymous)
	}

	prog := s.mu.GuardProgram(func(lock serverLock) *program {
		prog := s.programs[module]
		if prog == nil {
			return nil
		}

		return acc.refProgram(lock, prog)
	})
	if prog == nil {
		_check(notfound.ErrModule)
	}
	defer s.unrefProgram(&prog)

	funcIndex, err := prog.image.ResolveEntryFunc(launch.Function, false)
	_check(err)

	// TODO: check resource policy (text/stack/memory/max-memory size etc.)

	instImage, err := image.NewInstance(prog.image, policy.inst.MaxMemorySize, policy.inst.StackSize, funcIndex)
	_check(err)
	defer closeInstanceImage(&instImage)

	ref := &api.ModuleOptions{}
	inst, prog, _ := s._registerProgramRefInstance(ctx, acc, prog, instImage, &policy.inst, ref, launch)
	instImage = nil

	s._runOrDeleteInstance(ctx, inst, prog, launch.Function)
	prog = nil

	s.monitorInstance(ctx, event.TypeInstanceCreateKnown, newInstanceCreateInfo(inst.id, module, launch))

	return inst, nil
}

func (s *Server) UploadModuleInstance(ctx context.Context, upload *api.ModuleUpload, know *api.ModuleOptions, launch *api.LaunchOptions) (_ string, _ api.Instance, err error) {
	if internal.DontPanic() {
		defer func() { err = pan.Error(recover()) }()
	}

	know = _prepareModuleOptions(know)
	launch = _prepareLaunchOptions(launch)

	policy := new(instProgPolicy)
	ctx = _context(s.AccessPolicy.AuthorizeProgramInstance(ctx, &policy.res, &policy.prog, &policy.inst))

	acc := s._checkAccountInstanceID(ctx, launch.Instance)
	module, inst := s._loadModuleInstance(ctx, acc, policy, upload, know, launch)
	return module, inst, nil
}

func (s *Server) SourceModuleInstance(ctx context.Context, uri string, know *api.ModuleOptions, launch *api.LaunchOptions) (module string, _ api.Instance, err error) {
	if internal.DontPanic() {
		defer func() { err = pan.Error(recover()) }()
	}

	source, prefix := s._getSource(uri)
	know = _prepareModuleOptions(know)
	launch = _prepareLaunchOptions(launch)

	policy := new(instProgPolicy)
	ctx = _context(s.AccessPolicy.AuthorizeProgramInstanceSource(ctx, &policy.res, &policy.prog, &policy.inst, prefix))

	acc := s._checkAccountInstanceID(ctx, launch.Instance)

	stream, length, err := source.OpenURI(ctx, uri, policy.prog.MaxModuleSize)
	_check(err)
	if stream == nil {
		if length > 0 {
			_check(resourcelimit.Error("program size limit exceeded"))
		}
		_check(notfound.ErrModule)
	}

	upload := &api.ModuleUpload{
		Stream: stream,
		Length: length,
	}
	defer upload.Close()

	module, inst := s._loadModuleInstance(ctx, acc, policy, upload, know, launch)
	return module, inst, nil
}

func (s *Server) _loadModuleInstance(ctx context.Context, acc *account, policy *instProgPolicy, upload *api.ModuleUpload, know *api.ModuleOptions, launch *api.LaunchOptions) (string, *Instance) {
	if upload.Length > int64(policy.prog.MaxModuleSize) {
		_check(resourcelimit.Error("module size limit exceeded"))
	}

	// TODO: check resource policy

	if upload.Hash != "" {
		inst := s._loadKnownModuleInstance(ctx, acc, policy, upload, know, launch)
		if inst != nil {
			return upload.Hash, inst
		}
	}

	return s._loadUnknownModuleInstance(ctx, acc, policy, upload, know, launch)
}

func (s *Server) _loadKnownModuleInstance(ctx context.Context, acc *account, policy *instProgPolicy, upload *api.ModuleUpload, know *api.ModuleOptions, launch *api.LaunchOptions) *Instance {
	prog := s._refProgram(upload.Hash, upload.Length)
	if prog == nil {
		return nil
	}
	defer s.unrefProgram(&prog)
	progID := prog.id

	_validateUpload(upload)

	if prog.image.TextSize() > policy.prog.MaxTextSize {
		_check(resourcelimit.Error("program code size limit exceeded"))
	}

	// TODO: check resource policy (stack/memory/max-memory size etc.)

	funcIndex, err := prog.image.ResolveEntryFunc(launch.Function, false)
	_check(err)

	instImage, err := image.NewInstance(prog.image, policy.inst.MaxMemorySize, policy.inst.StackSize, funcIndex)
	_check(err)
	defer closeInstanceImage(&instImage)

	inst, prog, _ := s._registerProgramRefInstance(ctx, acc, prog, instImage, &policy.inst, know, launch)
	instImage = nil

	s.monitorModule(ctx, event.TypeModuleUploadExist, &event.Module{
		Module: progID,
	})

	s._runOrDeleteInstance(ctx, inst, prog, launch.Function)
	prog = nil

	s.monitorInstance(ctx, event.TypeInstanceCreateKnown, newInstanceCreateInfo(inst.id, progID, launch))

	return inst
}

func (s *Server) _loadUnknownModuleInstance(ctx context.Context, acc *account, policy *instProgPolicy, upload *api.ModuleUpload, know *api.ModuleOptions, launch *api.LaunchOptions) (string, *Instance) {
	prog, instImage := _buildProgram(s.ImageStorage, &policy.prog, &policy.inst, upload, launch.Function)
	defer closeInstanceImage(&instImage)
	defer s.unrefProgram(&prog)
	progID := prog.id

	inst, prog, redundantProg := s._registerProgramRefInstance(ctx, acc, prog, instImage, &policy.inst, know, launch)
	instImage = nil

	if upload.Hash != "" {
		if redundantProg {
			s.monitorModule(ctx, event.TypeModuleUploadExist, &event.Module{
				Module:   progID,
				Compiled: true,
			})
		} else {
			s.monitorModule(ctx, event.TypeModuleUploadNew, &event.Module{
				Module: progID,
			})
		}
	} else {
		if redundantProg {
			s.monitorModule(ctx, event.TypeModuleSourceExist, &event.Module{
				Module: progID,
				// TODO: source URI
				Compiled: true,
			})
		} else {
			s.monitorModule(ctx, event.TypeModuleSourceNew, &event.Module{
				Module: progID,
				// TODO: source URI
			})
		}
	}

	s._runOrDeleteInstance(ctx, inst, prog, launch.Function)
	prog = nil

	s.monitorInstance(ctx, event.TypeInstanceCreateStream, newInstanceCreateInfo(inst.id, progID, launch))

	return progID, inst
}

func (s *Server) ModuleInfo(ctx context.Context, module string) (_ *api.ModuleInfo, err error) {
	if internal.DontPanic() {
		defer func() { err = pan.Error(recover()) }()
	}

	ctx = _context(s.AccessPolicy.Authorize(ctx))

	pri := principal.ContextID(ctx)
	if pri == nil {
		_check(errAnonymous)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.programs == nil {
		_check(ErrServerClosed)
	}
	prog := s.programs[module]
	if prog == nil {
		_check(notfound.ErrModule)
	}

	acc := s.accounts[principal.Raw(pri)]
	if acc == nil {
		_check(notfound.ErrModule)
	}

	x, found := acc.programs[prog]
	if !found {
		_check(notfound.ErrModule)
	}

	info := &api.ModuleInfo{
		Id:   prog.id,
		Tags: append([]string(nil), x.tags...),
	}

	s.monitorModule(ctx, event.TypeModuleInfo, &event.Module{
		Module: prog.id,
	})

	return info, nil
}

func (s *Server) Modules(ctx context.Context) (_ *api.Modules, err error) {
	if internal.DontPanic() {
		defer func() { err = pan.Error(recover()) }()
	}

	ctx = _context(s.AccessPolicy.Authorize(ctx))

	pri := principal.ContextID(ctx)
	if pri == nil {
		_check(errAnonymous)
	}

	s.monitor(ctx, event.TypeModuleList)

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
	if internal.DontPanic() {
		defer func() { err = pan.Error(recover()) }()
	}

	ctx = _context(s.AccessPolicy.Authorize(ctx))

	pri := principal.ContextID(ctx)
	if pri == nil {
		_check(errAnonymous)
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
		_check(notfound.ErrModule)
	}

	length = prog.image.ModuleSize()
	stream = &moduleContent{
		Reader: prog.image.NewModuleReader(),
		ctx:    ctx,
		s:      s,
		prog:   prog,
		length: length,
	}
	return stream, length, nil
}

type moduleContent struct {
	io.Reader
	ctx    context.Context
	s      *Server
	prog   *program
	length int64
}

func (x *moduleContent) Close() error {
	x.s.monitorModule(x.ctx, event.TypeModuleDownload, &event.Module{
		Module: x.prog.id,
		Length: x.length,
	})

	x.s.unrefProgram(&x.prog)
	return nil
}

func (s *Server) PinModule(ctx context.Context, module string, know *api.ModuleOptions) (err error) {
	if internal.DontPanic() {
		defer func() { err = pan.Error(recover()) }()
	}

	know = _prepareModuleOptions(know)
	if !know.GetPin() {
		panic("Server.PinModule called without ModuleOptions.Pin")
	}

	policy := new(progPolicy)
	ctx = _context(s.AccessPolicy.AuthorizeProgram(ctx, &policy.res, &policy.prog))

	pri := principal.ContextID(ctx)
	if pri == nil {
		_check(errAnonymous)
	}

	modified := s.mu.GuardBool(func(lock serverLock) bool {
		if s.programs == nil {
			_check(ErrServerClosed)
		}
		prog := s.programs[module]
		if prog == nil {
			_check(notfound.ErrModule)
		}

		acc := s.accounts[principal.Raw(pri)]
		if acc == nil {
			_check(notfound.ErrModule)
		}

		if _, found := acc.programs[prog]; !found {
			for _, x := range acc.instances {
				if x.prog == prog {
					goto do
				}
			}
			_check(notfound.ErrModule)
		}

	do:
		// TODO: check resource limits
		return acc.ensureProgramRef(lock, prog, know.Tags)
	})

	if modified {
		s.monitorModule(ctx, event.TypeModulePin, &event.Module{
			Module:   module,
			TagCount: int32(len(know.Tags)),
		})
	}

	return
}

func (s *Server) UnpinModule(ctx context.Context, module string) (err error) {
	if internal.DontPanic() {
		defer func() { err = pan.Error(recover()) }()
	}

	ctx = _context(s.AccessPolicy.Authorize(ctx))

	pri := principal.ContextID(ctx)
	if pri == nil {
		_check(errAnonymous)
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
		_check(notfound.ErrModule)
	}

	s.monitorModule(ctx, event.TypeModuleUnpin, &event.Module{
		Module: module,
	})

	return
}

func (s *Server) InstanceConnection(ctx context.Context, instance string) (
	_ api.Instance,
	_ func(context.Context, io.Reader, io.WriteCloser) error,
	err error,
) {
	if internal.DontPanic() {
		defer func() { err = pan.Error(recover()) }()
	}

	ctx = _context(s.AccessPolicy.Authorize(ctx))

	inst := s._getInstance(ctx, instance)
	conn := inst.connect(ctx)
	if conn == nil {
		s.monitorFail(ctx, event.TypeFailRequest, &event.Fail{
			Type:     event.FailInstanceNoConnect,
			Instance: inst.id,
		}, nil)
		return inst, nil, nil
	}

	iofunc := func(ctx context.Context, r io.Reader, w io.WriteCloser) error {
		s.monitorInstance(ctx, event.TypeInstanceConnect, &event.Instance{
			Instance: inst.id,
		})

		err := conn(ctx, r, w)
		// TODO: monitor error

		s.monitorInstance(ctx, event.TypeInstanceDisconnect, &event.Instance{
			Instance: inst.id,
		})

		return err
	}

	return inst, iofunc, nil
}

func (s *Server) InstanceInfo(ctx context.Context, instance string) (_ *api.InstanceInfo, err error) {
	if internal.DontPanic() {
		defer func() { err = pan.Error(recover()) }()
	}

	ctx = _context(s.AccessPolicy.Authorize(ctx))

	progID, inst := s._getInstanceProgramID(ctx, instance)
	info := inst.info(progID)
	if info == nil {
		_check(notfound.ErrInstance)
	}

	s.monitorInstance(ctx, event.TypeInstanceInfo, &event.Instance{
		Instance: inst.id,
	})

	return info, nil
}

func (s *Server) WaitInstance(ctx context.Context, instID string) (_ *api.Status, err error) {
	if internal.DontPanic() {
		defer func() { err = pan.Error(recover()) }()
	}

	ctx = _context(s.AccessPolicy.Authorize(ctx))

	inst := s._getInstance(ctx, instID)
	status := inst.Wait(ctx)

	s.monitorInstance(ctx, event.TypeInstanceWait, &event.Instance{
		Instance: inst.id,
	})

	return status, err
}

func (s *Server) KillInstance(ctx context.Context, instance string) (_ api.Instance, err error) {
	if internal.DontPanic() {
		defer func() { err = pan.Error(recover()) }()
	}

	ctx = _context(s.AccessPolicy.Authorize(ctx))

	inst := s._getInstance(ctx, instance)
	inst.kill()

	s.monitorInstance(ctx, event.TypeInstanceKill, &event.Instance{
		Instance: inst.id,
	})

	return inst, nil
}

func (s *Server) SuspendInstance(ctx context.Context, instance string) (_ api.Instance, err error) {
	if internal.DontPanic() {
		defer func() { err = pan.Error(recover()) }()
	}

	ctx = _context(s.AccessPolicy.Authorize(ctx))

	// Store the program in case the instance becomes non-transient.
	inst, prog := s._getInstanceRefProgram(ctx, instance)
	defer s.unrefProgram(&prog)

	prog._ensureStorage()
	inst.suspend_()

	s.monitorInstance(ctx, event.TypeInstanceSuspend, &event.Instance{
		Instance: inst.id,
	})

	return inst, nil
}

func (s *Server) ResumeInstance(ctx context.Context, instance string, resume *api.ResumeOptions) (_ api.Instance, err error) {
	if internal.DontPanic() {
		defer func() { err = pan.Error(recover()) }()
	}

	resume = prepareResumeOptions(resume)
	policy := new(instPolicy)

	ctx = _context(s.AccessPolicy.AuthorizeInstance(ctx, &policy.res, &policy.inst))

	inst, prog := s._getInstanceRefProgram(ctx, instance)
	defer s.unrefProgram(&prog)

	inst._checkResume(resume.Function)

	proc, services := s._allocateInstanceResources(ctx, &policy.inst)
	defer closeInstanceResources(&proc, &services)

	inst._doResume(resume.Function, proc, services, policy.inst.TimeResolution, s.openDebugLog(resume.Invoke))
	proc = nil
	services = nil

	s._runOrDeleteInstance(ctx, inst, prog, resume.Function)
	prog = nil

	s.monitorInstance(ctx, event.TypeInstanceResume, &event.Instance{
		Instance: inst.id,
		Function: resume.Function,
	})

	return inst, nil
}

func (s *Server) DeleteInstance(ctx context.Context, instance string) (err error) {
	if internal.DontPanic() {
		defer func() { err = pan.Error(recover()) }()
	}

	ctx = _context(s.AccessPolicy.Authorize(ctx))

	inst := s._getInstance(ctx, instance)
	inst._annihilate()
	s.deleteNonexistentInstance(inst)

	s.monitorInstance(ctx, event.TypeInstanceDelete, &event.Instance{
		Instance: inst.id,
	})

	return
}

func (s *Server) Snapshot(ctx context.Context, instance string, know *api.ModuleOptions) (module string, err error) {
	if internal.DontPanic() {
		defer func() { err = pan.Error(recover()) }()
	}

	know = _prepareModuleOptions(know)
	if !know.GetPin() {
		panic("Server.SnapshotInstance called without ModuleOptions.Pin")
	}

	inst := s._getInstance(ctx, instance)

	// TODO: implement suspend-snapshot-resume at a lower level

	if inst.Status(ctx).State == api.StateRunning {
		inst.suspend()
		if inst.Wait(context.Background()).State == api.StateSuspended {
			defer func() {
				_, e := s.ResumeInstance(ctx, instance, nil)
				if module != "" {
					_check(e)
				}
			}()
		}
	}

	module = s._snapshot(ctx, instance, know)
	return
}

func (s *Server) _snapshot(ctx context.Context, instance string, know *api.ModuleOptions) string {
	ctx = _context(s.AccessPolicy.Authorize(ctx))

	// TODO: check module storage limits

	inst, oldProg := s._getInstanceRefProgram(ctx, instance)
	defer s.unrefProgram(&oldProg)

	newImage, buffers := inst._snapshot(oldProg)
	defer closeProgramImage(&newImage)

	h := api.KnownModuleHash.New()
	_, err := io.Copy(h, newImage.NewModuleReader())
	_check(err)
	progID := api.EncodeKnownModule(h.Sum(nil))

	_check(newImage.Store(progID))

	newProg := newProgram(progID, newImage, buffers, true)
	newImage = nil
	defer s.unrefProgram(&newProg)

	s._registerProgramRef(ctx, newProg, know)
	newProg = nil

	s.monitorInstance(ctx, event.TypeInstanceSnapshot, &event.Instance{
		Instance: inst.id,
		Module:   progID,
	})

	return progID
}

func (s *Server) UpdateInstance(ctx context.Context, instance string, update *api.InstanceUpdate) (_ *api.InstanceInfo, err error) {
	if internal.DontPanic() {
		defer func() { err = pan.Error(recover()) }()
	}

	update = prepareInstanceUpdate(update)

	ctx = _context(s.AccessPolicy.Authorize(ctx))

	progID, inst := s._getInstanceProgramID(ctx, instance)
	if inst.update(update) {
		s.monitorInstance(ctx, event.TypeInstanceUpdate, &event.Instance{
			Instance: inst.id,
			Persist:  update.Persist,
			TagCount: int32(len(update.Tags)),
		})
	}

	info := inst.info(progID)
	if info == nil {
		_check(notfound.ErrInstance)
	}

	return info, nil
}

func (s *Server) DebugInstance(ctx context.Context, instance string, req *api.DebugRequest) (_ *api.DebugResponse, err error) {
	if internal.DontPanic() {
		defer func() { err = pan.Error(recover()) }()
	}

	policy := new(progPolicy)

	ctx = _context(s.AccessPolicy.AuthorizeProgram(ctx, &policy.res, &policy.prog))

	inst, defaultProg := s._getInstanceRefProgram(ctx, instance)
	defer s.unrefProgram(&defaultProg)

	rebuild, config, res := inst._debug(ctx, defaultProg, req)
	if rebuild != nil {
		var (
			progImage *image.Program
			callMap   *object.CallMap
			ok        bool
		)

		progImage, callMap = _rebuildProgramImage(s.ImageStorage, &policy.prog, defaultProg.image.NewModuleReader(), config.Breakpoints)
		defer func() {
			if progImage != nil {
				progImage.Close()
			}
		}()

		res, ok = rebuild.apply(progImage, config, callMap)
		if !ok {
			_check(failrequest.Error(event.FailInstanceDebugState, "conflict"))
		}
		progImage = nil
	}

	s.monitorInstance(ctx, event.TypeInstanceDebug, &event.Instance{
		Instance: inst.id,
		Compiled: rebuild != nil,
	})

	return res, nil
}

func (s *Server) Instances(ctx context.Context) (_ *api.Instances, err error) {
	if internal.DontPanic() {
		defer func() { err = pan.Error(recover()) }()
	}

	ctx = _context(s.AccessPolicy.Authorize(ctx))

	pri := principal.ContextID(ctx)
	if pri == nil {
		_check(errAnonymous)
	}

	s.monitor(ctx, event.TypeInstanceList)

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
		if info := x.inst.info(x.progID); info != nil {
			infos.Instances = append(infos.Instances, info)
		}
	}
	return infos, nil
}

// ensureAccount may return nil.  It must not be called while the server is
// shutting down.
func (s *Server) ensureAccount(_ serverLock, pri *principal.ID) *account {
	acc := s.accounts[principal.Raw(pri)]
	if acc == nil {
		acc = newAccount(pri)
		s.accounts[principal.Raw(pri)] = acc
	}
	return acc
}

func (s *Server) _refProgram(hash string, length int64) *program {
	lock := s.mu.Lock()
	defer s.mu.Unlock()

	prog := s.programs[hash]
	if prog == nil {
		return nil
	}

	if length != prog.image.ModuleSize() {
		_check(errModuleSizeMismatch)
	}

	return prog.ref(lock)
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
func (s *Server) _registerProgramRef(ctx context.Context, prog *program, know *api.ModuleOptions) (redundant bool) {
	var pri *principal.ID

	if know.Pin {
		pri = principal.ContextID(ctx)
		if pri == nil {
			_check(errAnonymous)
		}

		prog._ensureStorage()
	}

	lock := s.mu.Lock()
	defer s.mu.Unlock()

	prog, redundant = s._mergeProgramRef(lock, prog)

	if know.Pin {
		// mergeProgramRef checked for shutdown, so the ensure methods are safe
		// to call.
		if s.ensureAccount(lock, pri).ensureProgramRef(lock, prog, know.Tags) {
			// TODO: move outside of critical section
			s.monitorModule(ctx, event.TypeModulePin, &event.Module{
				Module:   prog.id,
				TagCount: int32(len(know.Tags)),
			})
		}
	}

	return
}

func (s *Server) _checkAccountInstanceID(ctx context.Context, instID string) *account {
	if instID != "" {
		_validateInstanceID(instID)
	}

	pri := principal.ContextID(ctx)
	if pri == nil {
		return nil
	}

	lock := s.mu.Lock()
	defer s.mu.Unlock()

	if s.accounts == nil {
		_check(ErrServerClosed)
	}

	acc := s.ensureAccount(lock, pri)

	if instID != "" {
		acc._checkUniqueInstanceID(lock, instID)
	}

	return acc
}

// runOrDeleteInstance steals the program reference (except on error).
func (s *Server) _runOrDeleteInstance(ctx context.Context, inst *Instance, prog *program, function string) {
	defer s.unrefProgram(&prog)

	drive, err := inst.startOrAnnihilate(prog)
	if err != nil {
		s.deleteNonexistentInstance(inst)
		_check(err)
	}

	if drive {
		go s.driveInstance(api.ContextWithAddress(ctx, ""), inst, prog, function)
		prog = nil
	}
}

// driveInstance steals the program reference.
func (s *Server) driveInstance(ctx context.Context, inst *Instance, prog *program, function string) {
	defer s.unrefProgram(&prog)

	if nonexistent := inst.drive(ctx, prog, function, &s.Config); nonexistent {
		s.deleteNonexistentInstance(inst)
	}
}

func (s *Server) _getInstance(ctx context.Context, instance string) *Instance {
	_, inst := s._getInstanceProgramID(ctx, instance)
	return inst
}

func (s *Server) _getInstanceProgramID(ctx context.Context, instance string) (string, *Instance) {
	pri := principal.ContextID(ctx)
	if pri == nil {
		_check(errAnonymous)
	}

	lock := s.mu.Lock()
	defer s.mu.Unlock()

	inst, prog := s._getInstanceBorrowProgram(lock, pri, instance)
	return prog.id, inst
}

func (s *Server) _getInstanceRefProgram(ctx context.Context, instance string) (*Instance, *program) {
	pri := principal.ContextID(ctx)
	if pri == nil {
		_check(errAnonymous)
	}

	lock := s.mu.Lock()
	defer s.mu.Unlock()

	inst, prog := s._getInstanceBorrowProgram(lock, pri, instance)
	return inst, prog.ref(lock)
}

func (s *Server) _getInstanceBorrowProgram(_ serverLock, pri *principal.ID, instance string) (*Instance, *program) {
	acc := s.accounts[principal.Raw(pri)]
	if acc == nil {
		_check(notfound.ErrInstance)
	}

	x, found := acc.instances[instance]
	if !found {
		_check(notfound.ErrInstance)
	}

	return x.inst, x.prog
}

func (s *Server) _allocateInstanceResources(ctx context.Context, policy *InstancePolicy) (*runtime.Process, InstanceServices) {
	if policy.Services == nil {
		_check(PermissionDenied("no service policy"))
	}

	services := policy.Services(ctx)
	defer func() {
		if services != nil {
			services.Close()
		}
	}()

	proc, err := s.ProcessFactory.NewProcess(ctx)
	_check(err)

	ss := services
	services = nil
	return proc, ss
}

// registerProgramRefInstance with server, and an account if ref is true.
// Caller's instance image is stolen (except on error).  Caller's program
// reference is replaced with a reference to the canonical program object.
func (s *Server) _registerProgramRefInstance(ctx context.Context, acc *account, prog *program, instImage *image.Instance, policy *InstancePolicy, know *api.ModuleOptions, launch *api.LaunchOptions) (inst *Instance, canonicalProg *program, redundantProg bool) {
	var (
		proc     *runtime.Process
		services InstanceServices
	)
	if !launch.Suspend && !instImage.Final() {
		proc, services = s._allocateInstanceResources(ctx, policy)
		defer closeInstanceResources(&proc, &services)
	}

	if know.Pin || !launch.Transient {
		if acc == nil {
			_check(errAnonymous)
		}
		prog._ensureStorage()
	}

	instance := launch.Instance
	if instance == "" {
		instance = makeInstanceID()
	}

	lock := s.mu.Lock()
	defer s.mu.Unlock()

	if acc != nil {
		if s.accounts == nil {
			_check(ErrServerClosed)
		}
		acc._checkUniqueInstanceID(lock, instance)
	}

	prog, redundantProg = s._mergeProgramRef(lock, prog)

	inst = newInstance(instance, acc, launch.Transient, instImage, prog.buffers, proc, services, policy.TimeResolution, launch.Tags, s.openDebugLog(launch.Invoke))
	proc = nil
	services = nil

	if acc != nil {
		if know.Pin {
			// mergeProgramRef checked for shutdown, so ensureProgramRef is
			// safe to call.
			if acc.ensureProgramRef(lock, prog, know.Tags) {
				// TODO: move outside of critical section
				s.monitorModule(ctx, event.TypeModulePin, &event.Module{
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
		if x := inst.acc.instances[inst.id]; x.inst == inst {
			delete(inst.acc.instances, inst.id)
			x.prog.unref(lock)
		}
	} else {
		delete(s.anonymous, inst)
	}
}

// mergeProgramRef steals the program reference and returns a borrowed program
// reference which is valid until the server mutex is unlocked.
func (s *Server) _mergeProgramRef(lock serverLock, prog *program) (canonical *program, redundant bool) {
	switch existing := s.programs[prog.id]; existing {
	case nil:
		if s.programs == nil {
			_check(ErrServerClosed)
		}
		s.programs[prog.id] = prog // Pass reference to map.
		return prog, false

	case prog:
		if prog.refCount < 2 {
			panic("unexpected program reference count")
		}
		prog.unref(lock) // Map has reference; safe to drop temporary reference.
		return prog, false

	default:
		prog.unref(lock)
		return existing, true
	}
}

func (s *Server) _getSource(uri string) (Source, string) {
	if strings.HasPrefix(uri, "/") {
		if i := strings.Index(uri[1:], "/"); i > 0 {
			prefix := uri[:1+i]
			if len(prefix)+1 < len(uri) {
				source := s.Config.ModuleSources[prefix]
				if source != nil {
					return source, prefix
				}
			}
		}
	}

	panic(pan.Wrap(notfound.ErrModule))
}

func _prepareModuleOptions(opt *api.ModuleOptions) *api.ModuleOptions {
	if opt == nil {
		return new(api.ModuleOptions)
	}
	return opt
}

func _prepareLaunchOptions(opt *api.LaunchOptions) *api.LaunchOptions {
	if opt == nil {
		return new(api.LaunchOptions)
	}
	if opt.Suspend && opt.Function != "" {
		_check(failrequest.Error(event.FailInstanceStatus, "function cannot be specified for suspended instance"))
	}
	return opt
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

func newInstanceCreateInfo(instance, module string, launch *api.LaunchOptions) *event.Instance {
	return &event.Instance{
		Instance:  instance,
		Module:    module,
		Transient: launch.Transient,
		Suspended: launch.Suspend,
		TagCount:  int32(len(launch.Tags)),
	}
}

func _check(err error) {
	pan.Check(err)
}

func _context(ctx context.Context, err error) context.Context {
	_check(err)
	return ctx
}
