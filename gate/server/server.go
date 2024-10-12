// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"

	"gate.computer/gate/image"
	"gate.computer/gate/runtime"
	"gate.computer/gate/scope"
	"gate.computer/gate/server/api"
	"gate.computer/gate/server/event"
	"gate.computer/gate/server/internal"
	"gate.computer/gate/server/internal/error/failrequest"
	"gate.computer/gate/server/internal/error/notfound"
	"gate.computer/gate/server/tracelog"
	"gate.computer/internal/error/resourcelimit"
	"gate.computer/internal/principal"
	"gate.computer/wag/object"
	"import.name/lock"
	"import.name/pan"

	. "import.name/pan/mustcheck"
	. "import.name/type/context"
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

type serverLock struct{}

type Server struct {
	privateConfig

	mu        lock.TagMutex[serverLock]
	programs  map[string]*program
	accounts  map[principal.RawKey]*account
	anonymous map[*Instance]struct{}
}

func New(ctx Context, config *Config) (_ *Server, err error) {
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
	if s.StartSpan == nil {
		s.StartSpan = tracelog.SpanStarter(nil, "server: ")
	}
	if s.AddEvent == nil {
		s.AddEvent = tracelog.EventAdder(nil, "server: ", nil)
	}
	if !s.Configured() {
		panic("incomplete server configuration")
	}

	progs := Must(s.ImageStorage.Programs())
	insts := Must(s.ImageStorage.Instances())

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
		s.mustLoadProgramDuringInit(lock, owner, id)
	}
	for _, key := range insts {
		s.mustLoadInstanceDuringInit(lock, key)
	}

	shutdown = nil
	return s, nil
}

func (s *Server) mustLoadProgramDuringInit(lock serverLock, owner *account, progID string) {
	image := Must(s.ImageStorage.LoadProgram(progID))
	if image == nil { // Race condition with human operator?
		return
	}
	defer closeProgramImage(&image)

	prog := newProgram(progID, image, Must(image.LoadBuffers()), true)
	image = nil

	if owner != nil {
		owner.ensureProgramRef(lock, prog, nil)
	}

	s.programs[progID] = prog
}

func (s *Server) mustLoadInstanceDuringInit(lock serverLock, key string) {
	image := Must(s.ImageStorage.LoadInstance(key))
	if image == nil { // Race condition with human operator?
		return
	}
	defer closeInstanceImage(&image)

	pri, instID := mustParseInstanceStorageKey(key)
	acc := s.ensureAccount(lock, pri)

	// TODO: restore instance
	slog.Debug("server: instance loading not implemented", "principal", acc.ID, "instance", instID, "trap", image.Trap())
}

func (s *Server) Shutdown(ctx Context) error {
	var (
		accInsts  []*Instance
		anonInsts map[*Instance]struct{}
	)
	lock.GuardTag(&s.mu, func(lock serverLock) {
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
		inst.suspend(false)
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

func (s *Server) UploadModule(ctx Context, upload *api.ModuleUpload, know *api.ModuleOptions) (module string, err error) {
	if internal.DontPanic() {
		defer func() { err = pan.Error(recover()) }()
	}

	ctx, end := s.startOp(ctx, api.OpModuleUpload)
	defer end(ctx)

	know = mustPrepareModuleOptions(know)

	policy := new(progPolicy)
	ctx = Must(s.AccessPolicy.AuthorizeProgram(ctx, &policy.res, &policy.prog))

	if upload.Length > int64(policy.prog.MaxModuleSize) {
		pan.Panic(resourcelimit.Error("module size limit exceeded"))
	}

	// TODO: check resource policy

	if upload.Hash != "" && s.mustLoadKnownModule(ctx, policy, upload, know) {
		return upload.Hash, nil
	}

	return s.mustLoadUnknownModule(ctx, policy, upload, know), nil
}

func (s *Server) SourceModule(ctx Context, uri string, know *api.ModuleOptions) (module string, err error) {
	if internal.DontPanic() {
		defer func() { err = pan.Error(recover()) }()
	}

	ctx, end := s.startOp(ctx, api.OpModuleSource)
	defer end(ctx)

	source, prefix := s.mustGetSource(uri)
	know = mustPrepareModuleOptions(know)

	policy := new(progPolicy)
	ctx = Must(s.AccessPolicy.AuthorizeProgramSource(ctx, &policy.res, &policy.prog, prefix))

	uri = Must(source.CanonicalURI(uri))

	if m, err := s.SourceCache.GetSourceSHA256(ctx, uri); err == nil && m != "" {
		slog.DebugContext(ctx, "server: source module is known", "uri", uri, "module", m)
		// TODO
	}

	stream, length, err := source.OpenURI(ctx, uri, policy.prog.MaxModuleSize)
	Check(err)
	if stream == nil {
		if length > 0 {
			pan.Panic(resourcelimit.Error("program size limit exceeded"))
		}
		pan.Panic(notfound.ErrModule)
	}

	upload := &api.ModuleUpload{
		Stream: stream,
		Length: length,
	}
	defer upload.Close()

	module = s.mustLoadUnknownModule(ctx, policy, upload, know)

	if err := s.SourceCache.PutSourceSHA256(ctx, uri, module); err != nil {
		s.eventFail(ctx, event.TypeFailRequest, &event.Fail{
			Type:      event.FailInternal,
			Source:    uri,
			Module:    module,
			Subsystem: "source cache",
		}, err)
	}

	return
}

func (s *Server) mustLoadKnownModule(ctx Context, policy *progPolicy, upload *api.ModuleUpload, know *api.ModuleOptions) bool {
	prog := s.mustRefProgram(upload.Hash, upload.Length)
	if prog == nil {
		return false
	}
	defer s.unrefProgram(&prog)
	progID := prog.id

	mustValidateUpload(upload)

	if prog.image.TextSize() > policy.prog.MaxTextSize {
		pan.Panic(resourcelimit.Error("program code size limit exceeded"))
	}

	s.mustRegisterProgramRef(ctx, prog, know)
	prog = nil

	s.eventModule(ctx, event.TypeModuleUploadExist, &event.Module{
		Module: progID,
	})

	return true
}

func (s *Server) mustLoadUnknownModule(ctx Context, policy *progPolicy, upload *api.ModuleUpload, know *api.ModuleOptions) string {
	prog, _ := mustBuildProgram(s.ImageStorage, &policy.prog, nil, upload, "")
	defer s.unrefProgram(&prog)
	progID := prog.id

	redundant := s.mustRegisterProgramRef(ctx, prog, know)
	prog = nil

	if redundant {
		s.eventModule(ctx, event.TypeModuleUploadExist, &event.Module{
			Module:   progID,
			Compiled: true,
		})
	} else {
		s.eventModule(ctx, event.TypeModuleUploadNew, &event.Module{
			Module: progID,
		})
	}

	return progID
}

func (s *Server) NewInstance(ctx Context, module string, launch *api.LaunchOptions) (_ api.Instance, err error) {
	if internal.DontPanic() {
		defer func() { err = pan.Error(recover()) }()
	}

	ctx, end := s.startOp(ctx, api.OpLaunchExtant)
	defer end(ctx)

	launch = mustPrepareLaunchOptions(launch)

	policy := new(instPolicy)
	ctx = Must(s.AccessPolicy.AuthorizeInstance(ctx, &policy.res, &policy.inst))

	acc := s.mustCheckAccountInstanceID(ctx, launch.Instance)
	if acc == nil {
		pan.Panic(errAnonymous)
	}

	prog := lock.GuardTagged(&s.mu, func(lock serverLock) *program {
		prog := s.programs[module]
		if prog == nil {
			return nil
		}

		return acc.refProgram(lock, prog)
	})
	if prog == nil {
		pan.Panic(notfound.ErrModule)
	}
	defer s.unrefProgram(&prog)

	funcIndex := Must(prog.image.ResolveEntryFunc(launch.Function, false))

	// TODO: check resource policy (text/stack/memory/max-memory size etc.)

	instImage := Must(image.NewInstance(prog.image, policy.inst.MaxMemorySize, policy.inst.StackSize, funcIndex))
	defer closeInstanceImage(&instImage)

	ref := &api.ModuleOptions{}
	inst, prog, _ := s.mustRegisterProgramRefInstance(ctx, acc, prog, instImage, &policy.inst, ref, launch)
	instImage = nil

	s.mustRunOrDeleteInstance(ctx, inst, prog, launch.Function)
	prog = nil

	s.eventInstance(ctx, event.TypeInstanceCreateKnown, newInstanceCreateInfo(inst.id, module, launch), nil)

	return inst, nil
}

func (s *Server) UploadModuleInstance(ctx Context, upload *api.ModuleUpload, know *api.ModuleOptions, launch *api.LaunchOptions) (_ string, _ api.Instance, err error) {
	if internal.DontPanic() {
		defer func() { err = pan.Error(recover()) }()
	}

	ctx, end := s.startOp(ctx, api.OpLaunchUpload)
	defer end(ctx)

	know = mustPrepareModuleOptions(know)
	launch = mustPrepareLaunchOptions(launch)

	policy := new(instProgPolicy)
	ctx = Must(s.AccessPolicy.AuthorizeProgramInstance(ctx, &policy.res, &policy.prog, &policy.inst))

	acc := s.mustCheckAccountInstanceID(ctx, launch.Instance)
	module, inst := s.mustLoadModuleInstance(ctx, acc, policy, upload, know, launch)
	return module, inst, nil
}

func (s *Server) SourceModuleInstance(ctx Context, uri string, know *api.ModuleOptions, launch *api.LaunchOptions) (module string, _ api.Instance, err error) {
	if internal.DontPanic() {
		defer func() { err = pan.Error(recover()) }()
	}

	ctx, end := s.startOp(ctx, api.OpLaunchSource)
	defer end(ctx)

	source, prefix := s.mustGetSource(uri)
	know = mustPrepareModuleOptions(know)
	launch = mustPrepareLaunchOptions(launch)

	policy := new(instProgPolicy)
	ctx = Must(s.AccessPolicy.AuthorizeProgramInstanceSource(ctx, &policy.res, &policy.prog, &policy.inst, prefix))

	acc := s.mustCheckAccountInstanceID(ctx, launch.Instance)

	uri = Must(source.CanonicalURI(uri))

	if m, err := s.SourceCache.GetSourceSHA256(ctx, uri); err == nil && m != "" {
		slog.DebugContext(ctx, "server: source module is known", "uri", uri, "module", m)
		// TODO
	}

	stream, length, err := source.OpenURI(ctx, uri, policy.prog.MaxModuleSize)
	Check(err)
	if stream == nil {
		if length > 0 {
			pan.Panic(resourcelimit.Error("program size limit exceeded"))
		}
		pan.Panic(notfound.ErrModule)
	}

	upload := &api.ModuleUpload{
		Stream: stream,
		Length: length,
	}
	defer upload.Close()

	module, inst := s.mustLoadModuleInstance(ctx, acc, policy, upload, know, launch)

	if err := s.SourceCache.PutSourceSHA256(ctx, uri, module); err != nil {
		s.eventFail(ctx, event.TypeFailRequest, &event.Fail{
			Type:      event.FailInternal,
			Source:    uri,
			Module:    module,
			Subsystem: "source cache",
		}, err)
	}

	return module, inst, nil
}

func (s *Server) mustLoadModuleInstance(ctx Context, acc *account, policy *instProgPolicy, upload *api.ModuleUpload, know *api.ModuleOptions, launch *api.LaunchOptions) (string, *Instance) {
	if upload.Length > int64(policy.prog.MaxModuleSize) {
		pan.Panic(resourcelimit.Error("module size limit exceeded"))
	}

	// TODO: check resource policy

	if upload.Hash != "" {
		inst := s.mustLoadKnownModuleInstance(ctx, acc, policy, upload, know, launch)
		if inst != nil {
			return upload.Hash, inst
		}
	}

	return s.mustLoadUnknownModuleInstance(ctx, acc, policy, upload, know, launch)
}

func (s *Server) mustLoadKnownModuleInstance(ctx Context, acc *account, policy *instProgPolicy, upload *api.ModuleUpload, know *api.ModuleOptions, launch *api.LaunchOptions) *Instance {
	prog := s.mustRefProgram(upload.Hash, upload.Length)
	if prog == nil {
		return nil
	}
	defer s.unrefProgram(&prog)
	progID := prog.id

	mustValidateUpload(upload)

	if prog.image.TextSize() > policy.prog.MaxTextSize {
		pan.Panic(resourcelimit.Error("program code size limit exceeded"))
	}

	// TODO: check resource policy (stack/memory/max-memory size etc.)

	funcIndex := Must(prog.image.ResolveEntryFunc(launch.Function, false))

	instImage := Must(image.NewInstance(prog.image, policy.inst.MaxMemorySize, policy.inst.StackSize, funcIndex))
	defer closeInstanceImage(&instImage)

	inst, prog, _ := s.mustRegisterProgramRefInstance(ctx, acc, prog, instImage, &policy.inst, know, launch)
	instImage = nil

	s.eventModule(ctx, event.TypeModuleUploadExist, &event.Module{
		Module: progID,
	})

	s.mustRunOrDeleteInstance(ctx, inst, prog, launch.Function)
	prog = nil

	s.eventInstance(ctx, event.TypeInstanceCreateKnown, newInstanceCreateInfo(inst.id, progID, launch), nil)

	return inst
}

func (s *Server) mustLoadUnknownModuleInstance(ctx Context, acc *account, policy *instProgPolicy, upload *api.ModuleUpload, know *api.ModuleOptions, launch *api.LaunchOptions) (string, *Instance) {
	prog, instImage := mustBuildProgram(s.ImageStorage, &policy.prog, &policy.inst, upload, launch.Function)
	defer closeInstanceImage(&instImage)
	defer s.unrefProgram(&prog)
	progID := prog.id

	inst, prog, redundantProg := s.mustRegisterProgramRefInstance(ctx, acc, prog, instImage, &policy.inst, know, launch)
	instImage = nil

	if upload.Hash != "" {
		if redundantProg {
			s.eventModule(ctx, event.TypeModuleUploadExist, &event.Module{
				Module:   progID,
				Compiled: true,
			})
		} else {
			s.eventModule(ctx, event.TypeModuleUploadNew, &event.Module{
				Module: progID,
			})
		}
	} else {
		if redundantProg {
			s.eventModule(ctx, event.TypeModuleSourceExist, &event.Module{
				Module: progID,
				// TODO: source URI
				Compiled: true,
			})
		} else {
			s.eventModule(ctx, event.TypeModuleSourceNew, &event.Module{
				Module: progID,
				// TODO: source URI
			})
		}
	}

	s.mustRunOrDeleteInstance(ctx, inst, prog, launch.Function)
	prog = nil

	s.eventInstance(ctx, event.TypeInstanceCreateStream, newInstanceCreateInfo(inst.id, progID, launch), nil)

	return progID, inst
}

func (s *Server) ModuleInfo(ctx Context, module string) (_ *api.ModuleInfo, err error) {
	if internal.DontPanic() {
		defer func() { err = pan.Error(recover()) }()
	}

	ctx, end := s.startOp(ctx, api.OpModuleInfo)
	defer end(ctx)

	ctx = Must(s.AccessPolicy.Authorize(ctx))

	pri := principal.ContextID(ctx)
	if pri == nil {
		pan.Panic(errAnonymous)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.programs == nil {
		pan.Panic(ErrServerClosed)
	}
	prog := s.programs[module]
	if prog == nil {
		pan.Panic(notfound.ErrModule)
	}

	acc := s.accounts[principal.Raw(pri)]
	if acc == nil {
		pan.Panic(notfound.ErrModule)
	}

	x, found := acc.programs[prog]
	if !found {
		pan.Panic(notfound.ErrModule)
	}

	info := &api.ModuleInfo{
		Id:   prog.id,
		Tags: append([]string(nil), x.Tags...),
	}

	s.eventModule(ctx, event.TypeModuleInfo, &event.Module{
		Module: prog.id,
	})

	return info, nil
}

func (s *Server) Modules(ctx Context) (_ *api.Modules, err error) {
	if internal.DontPanic() {
		defer func() { err = pan.Error(recover()) }()
	}

	ctx, end := s.startOp(ctx, api.OpModuleList)
	defer end(ctx)

	ctx = Must(s.AccessPolicy.Authorize(ctx))

	pri := principal.ContextID(ctx)
	if pri == nil {
		pan.Panic(errAnonymous)
	}

	s.event(ctx, event.TypeModuleList)

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
			Tags: append([]string(nil), x.Tags...),
		})
	}
	return infos, nil
}

func (s *Server) ModuleContent(ctx Context, module string) (stream io.ReadCloser, length int64, err error) {
	if internal.DontPanic() {
		defer func() { err = pan.Error(recover()) }()
	}

	ctx, endSpan := s.startOp(ctx, api.OpModuleDownload)
	defer func() {
		if endSpan != nil {
			endSpan(ctx)
		}
	}()

	ctx = Must(s.AccessPolicy.Authorize(ctx))

	pri := principal.ContextID(ctx)
	if pri == nil {
		pan.Panic(errAnonymous)
	}

	prog := lock.GuardTagged(&s.mu, func(lock serverLock) *program {
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
		pan.Panic(notfound.ErrModule)
	}

	length = prog.image.ModuleSize()
	stream = &moduleContent{
		Reader:  prog.image.NewModuleReader(),
		ctx:     ctx,
		endSpan: endSpan,
		s:       s,
		prog:    prog,
		length:  length,
	}
	endSpan = nil
	return stream, length, nil
}

type moduleContent struct {
	io.Reader
	ctx     Context
	endSpan func(Context)
	s       *Server
	prog    *program
	length  int64
}

func (x *moduleContent) Close() error {
	defer x.endSpan(x.ctx)

	x.s.eventModule(x.ctx, event.TypeModuleDownload, &event.Module{
		Module: x.prog.id,
		Length: x.length,
	})

	x.s.unrefProgram(&x.prog)
	return nil
}

func (s *Server) PinModule(ctx Context, module string, know *api.ModuleOptions) (err error) {
	if internal.DontPanic() {
		defer func() { err = pan.Error(recover()) }()
	}

	ctx, end := s.startOp(ctx, api.OpModulePin)
	defer end(ctx)

	know = mustPrepareModuleOptions(know)
	if !know.GetPin() {
		panic("Server.PinModule called without ModuleOptions.Pin")
	}

	policy := new(progPolicy)
	ctx = Must(s.AccessPolicy.AuthorizeProgram(ctx, &policy.res, &policy.prog))

	pri := principal.ContextID(ctx)
	if pri == nil {
		pan.Panic(errAnonymous)
	}

	modified := lock.GuardTagged(&s.mu, func(lock serverLock) bool {
		if s.programs == nil {
			pan.Panic(ErrServerClosed)
		}
		prog := s.programs[module]
		if prog == nil {
			pan.Panic(notfound.ErrModule)
		}

		acc := s.accounts[principal.Raw(pri)]
		if acc == nil {
			pan.Panic(notfound.ErrModule)
		}

		_, found := acc.programs[prog]
		if !found {
			for _, x := range acc.instances {
				if x.prog == prog {
					goto do
				}
			}
			pan.Panic(notfound.ErrModule)
		}

	do:
		// TODO: check resource limits
		return acc.ensureProgramRef(lock, prog, know.Tags)
	})

	if modified {
		s.eventModule(ctx, event.TypeModulePin, &event.Module{
			Module:   module,
			TagCount: int32(len(know.Tags)),
		})
	}

	return
}

func (s *Server) UnpinModule(ctx Context, module string) (err error) {
	if internal.DontPanic() {
		defer func() { err = pan.Error(recover()) }()
	}

	ctx, end := s.startOp(ctx, api.OpModuleUnpin)
	defer end(ctx)

	ctx = Must(s.AccessPolicy.Authorize(ctx))

	pri := principal.ContextID(ctx)
	if pri == nil {
		pan.Panic(errAnonymous)
	}

	found := lock.GuardTagged(&s.mu, func(lock serverLock) bool {
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
		pan.Panic(notfound.ErrModule)
	}

	s.eventModule(ctx, event.TypeModuleUnpin, &event.Module{
		Module: module,
	})

	return
}

func (s *Server) InstanceConnection(ctx Context, instance string) (_ api.Instance, _ func(Context, io.Reader, io.WriteCloser) *api.Status, err error) {
	if internal.DontPanic() {
		defer func() { err = pan.Error(recover()) }()
	}

	ctx, end := s.startOp(ctx, api.OpInstanceConnect)
	defer end(ctx)

	ctx = Must(s.AccessPolicy.Authorize(ctx))

	inst := s.mustGetInstance(ctx, instance)
	conn := inst.connect(ctx)
	if conn == nil {
		s.eventFail(ctx, event.TypeFailRequest, &event.Fail{
			Type:     event.FailInstanceNoConnect,
			Instance: inst.id,
		}, nil)
		return inst, nil, nil
	}

	s.eventInstance(ctx, event.TypeInstanceConnect, &event.Instance{
		Instance: inst.id,
	}, nil)

	iofunc := func(ctx Context, r io.Reader, w io.WriteCloser) *api.Status {
		err := conn(ctx, r, w)

		s.eventInstance(ctx, event.TypeInstanceDisconnect, &event.Instance{
			Instance: inst.id,
		}, err)

		return inst.Status()
	}

	return inst, iofunc, nil
}

func (s *Server) InstanceInfo(ctx Context, instance string) (_ *api.InstanceInfo, err error) {
	if internal.DontPanic() {
		defer func() { err = pan.Error(recover()) }()
	}

	ctx, end := s.startOp(ctx, api.OpInstanceInfo)
	defer end(ctx)

	ctx = Must(s.AccessPolicy.Authorize(ctx))

	progID, inst := s.mustGetInstanceProgramID(ctx, instance)
	info := inst.info(progID)
	if info == nil {
		pan.Panic(notfound.ErrInstance)
	}

	s.eventInstance(ctx, event.TypeInstanceInfo, &event.Instance{
		Instance: inst.id,
	}, nil)

	return info, nil
}

func (s *Server) WaitInstance(ctx Context, instID string) (_ *api.Status, err error) {
	if internal.DontPanic() {
		defer func() { err = pan.Error(recover()) }()
	}

	ctx, end := s.startOp(ctx, api.OpInstanceWait)
	defer end(ctx)

	ctx = Must(s.AccessPolicy.Authorize(ctx))

	inst := s.mustGetInstance(ctx, instID)
	status := inst.Wait(ctx)

	s.eventInstance(ctx, event.TypeInstanceWait, &event.Instance{
		Instance: inst.id,
	}, nil)

	return status, err
}

func (s *Server) KillInstance(ctx Context, instance string) (_ api.Instance, err error) {
	if internal.DontPanic() {
		defer func() { err = pan.Error(recover()) }()
	}

	ctx, end := s.startOp(ctx, api.OpInstanceKill)
	defer end(ctx)

	ctx = Must(s.AccessPolicy.Authorize(ctx))

	inst := s.mustGetInstance(ctx, instance)
	inst.kill()

	s.eventInstance(ctx, event.TypeInstanceKill, &event.Instance{
		Instance: inst.id,
	}, nil)

	return inst, nil
}

func (s *Server) SuspendInstance(ctx Context, instance string) (_ api.Instance, err error) {
	if internal.DontPanic() {
		defer func() { err = pan.Error(recover()) }()
	}

	ctx, end := s.startOp(ctx, api.OpInstanceSuspend)
	defer end(ctx)

	ctx = Must(s.AccessPolicy.Authorize(ctx))

	// Store the program in case the instance becomes non-transient.
	inst, prog := s.mustGetInstanceRefProgram(ctx, instance)
	defer s.unrefProgram(&prog)

	prog.mustEnsureStorage()
	inst.suspend(true)

	s.eventInstance(ctx, event.TypeInstanceSuspend, &event.Instance{
		Instance: inst.id,
	}, nil)

	return inst, nil
}

func (s *Server) ResumeInstance(ctx Context, instance string, resume *api.ResumeOptions) (_ api.Instance, err error) {
	if internal.DontPanic() {
		defer func() { err = pan.Error(recover()) }()
	}

	ctx, end := s.startOp(ctx, api.OpInstanceResume)
	defer end(ctx)

	resume = prepareResumeOptions(resume)
	policy := new(instPolicy)

	ctx = Must(s.AccessPolicy.AuthorizeInstance(ctx, &policy.res, &policy.inst))

	inst, prog := s.mustGetInstanceRefProgram(ctx, instance)
	defer s.unrefProgram(&prog)

	inst.mustCheckResume(resume.Function)

	proc, services := s.mustAllocateInstanceResources(ctx, &policy.inst)
	defer closeInstanceResources(&proc, &services)

	inst.mustResume(resume.Function, proc, services, policy.inst.TimeResolution, s.openDebugLog(resume.Invoke))
	proc = nil
	services = nil

	s.mustRunOrDeleteInstance(ctx, inst, prog, resume.Function)
	prog = nil

	s.eventInstance(ctx, event.TypeInstanceResume, &event.Instance{
		Instance: inst.id,
		Function: resume.Function,
	}, nil)

	return inst, nil
}

func (s *Server) DeleteInstance(ctx Context, instance string) (err error) {
	if internal.DontPanic() {
		defer func() { err = pan.Error(recover()) }()
	}

	ctx, end := s.startOp(ctx, api.OpInstanceDelete)
	defer end(ctx)

	ctx = Must(s.AccessPolicy.Authorize(ctx))

	inst := s.mustGetInstance(ctx, instance)
	inst.mustAnnihilate()
	s.deleteNonexistentInstance(inst)

	s.eventInstance(ctx, event.TypeInstanceDelete, &event.Instance{
		Instance: inst.id,
	}, nil)

	return
}

func (s *Server) Snapshot(ctx Context, instance string, know *api.ModuleOptions) (module string, err error) {
	if internal.DontPanic() {
		defer func() { err = pan.Error(recover()) }()
	}

	ctx, end := s.startOp(ctx, api.OpInstanceSnapshot)
	defer end(ctx)

	know = mustPrepareModuleOptions(know)
	if !know.GetPin() {
		panic("Server.Snapshot called without ModuleOptions.Pin")
	}

	inst := s.mustGetInstance(ctx, instance)

	// TODO: implement suspend-snapshot-resume at a lower level

	if inst.Status().State == api.StateRunning {
		inst.suspend(false)
		if inst.Wait(context.Background()).State == api.StateSuspended {
			defer func() {
				_, e := s.ResumeInstance(ctx, instance, nil)
				if module != "" {
					Check(e)
				}
			}()
		}
	}

	module = s.mustSnapshot(ctx, instance, know)
	return
}

func (s *Server) mustSnapshot(ctx Context, instance string, know *api.ModuleOptions) string {
	ctx = Must(s.AccessPolicy.Authorize(ctx))

	// TODO: check module storage limits

	inst, oldProg := s.mustGetInstanceRefProgram(ctx, instance)
	defer s.unrefProgram(&oldProg)

	newImage, buffers := inst.mustSnapshot(oldProg)
	defer closeProgramImage(&newImage)

	h := api.KnownModuleHash.New()
	Must(io.Copy(h, newImage.NewModuleReader()))
	progID := api.EncodeKnownModule(h.Sum(nil))

	Check(newImage.Store(progID))

	newProg := newProgram(progID, newImage, buffers, true)
	newImage = nil
	defer s.unrefProgram(&newProg)

	s.mustRegisterProgramRef(ctx, newProg, know)
	newProg = nil

	s.eventInstance(ctx, event.TypeInstanceSnapshot, &event.Instance{
		Instance: inst.id,
		Module:   progID,
	}, nil)

	return progID
}

func (s *Server) UpdateInstance(ctx Context, instance string, update *api.InstanceUpdate) (_ *api.InstanceInfo, err error) {
	if internal.DontPanic() {
		defer func() { err = pan.Error(recover()) }()
	}

	ctx, end := s.startOp(ctx, api.OpInstanceUpdate)
	defer end(ctx)

	update = prepareInstanceUpdate(update)

	ctx = Must(s.AccessPolicy.Authorize(ctx))

	progID, inst := s.mustGetInstanceProgramID(ctx, instance)
	if inst.update(update) {
		s.eventInstance(ctx, event.TypeInstanceUpdate, &event.Instance{
			Instance: inst.id,
			Persist:  update.Persist,
			TagCount: int32(len(update.Tags)),
		}, nil)
	}

	info := inst.info(progID)
	if info == nil {
		pan.Panic(notfound.ErrInstance)
	}

	return info, nil
}

func (s *Server) DebugInstance(ctx Context, instance string, req *api.DebugRequest) (_ *api.DebugResponse, err error) {
	if internal.DontPanic() {
		defer func() { err = pan.Error(recover()) }()
	}

	ctx, end := s.startOp(ctx, api.OpInstanceDebug)
	defer end(ctx)

	policy := new(progPolicy)

	ctx = Must(s.AccessPolicy.AuthorizeProgram(ctx, &policy.res, &policy.prog))

	inst, defaultProg := s.mustGetInstanceRefProgram(ctx, instance)
	defer s.unrefProgram(&defaultProg)

	rebuild, config, res := inst.mustDebug(ctx, defaultProg, req)
	if rebuild != nil {
		var (
			progImage *image.Program
			callMap   *object.CallMap
			ok        bool
		)

		progImage, callMap = mustRebuildProgramImage(s.ImageStorage, &policy.prog, defaultProg.image.NewModuleReader(), config.Breakpoints)
		defer func() {
			if progImage != nil {
				progImage.Close()
			}
		}()

		res, ok = rebuild.apply(progImage, config, callMap)
		if !ok {
			pan.Panic(failrequest.Error(event.FailInstanceDebugState, "conflict"))
		}
		progImage = nil
	}

	s.eventInstance(ctx, event.TypeInstanceDebug, &event.Instance{
		Instance: inst.id,
		Compiled: rebuild != nil,
	}, nil)

	return res, nil
}

func (s *Server) Instances(ctx Context) (_ *api.Instances, err error) {
	if internal.DontPanic() {
		defer func() { err = pan.Error(recover()) }()
	}

	ctx, end := s.startOp(ctx, api.OpInstanceList)
	defer end(ctx)

	ctx = Must(s.AccessPolicy.Authorize(ctx))

	pri := principal.ContextID(ctx)
	if pri == nil {
		pan.Panic(errAnonymous)
	}

	s.event(ctx, event.TypeInstanceList)

	type instProgID struct {
		inst   *Instance
		progID string
	}

	// Get instance references while holding server lock.
	var insts []instProgID
	lock.GuardTag(&s.mu, func(serverLock) {
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
func (s *Server) ensureAccount(lock serverLock, pri *principal.ID) *account {
	acc := s.accounts[principal.Raw(pri)]
	if acc == nil {
		acc = newAccount(pri)
		s.accounts[principal.Raw(pri)] = acc
	}
	return acc
}

func (s *Server) mustRefProgram(hash string, length int64) *program {
	lock := s.mu.Lock()
	defer s.mu.Unlock()

	prog := s.programs[hash]
	if prog == nil {
		return nil
	}

	if length != prog.image.ModuleSize() {
		pan.Panic(errModuleSizeMismatch)
	}

	return prog.ref(lock)
}

func (s *Server) unrefProgram(p **program) {
	prog := *p
	*p = nil
	if prog == nil {
		return
	}

	lock.GuardTag(&s.mu, prog.unref)
}

// mustRegisterProgramRef with the server and an account.  Caller's program
// reference is stolen (except on error).
func (s *Server) mustRegisterProgramRef(ctx Context, prog *program, know *api.ModuleOptions) (redundant bool) {
	var pri *principal.ID

	if know.Pin {
		pri = principal.ContextID(ctx)
		if pri == nil {
			pan.Panic(errAnonymous)
		}

		prog.mustEnsureStorage()
	}

	lock := s.mu.Lock()
	defer s.mu.Unlock()

	prog, redundant = s.mustMergeProgramRef(lock, prog)

	if know.Pin {
		// mergeProgramRef checked for shutdown, so the ensure methods are safe
		// to call.
		if s.ensureAccount(lock, pri).ensureProgramRef(lock, prog, know.Tags) {
			// TODO: move outside of critical section
			s.eventModule(ctx, event.TypeModulePin, &event.Module{
				Module:   prog.id,
				TagCount: int32(len(know.Tags)),
			})
		}
	}

	return
}

func (s *Server) mustCheckAccountInstanceID(ctx Context, instID string) *account {
	if instID != "" {
		Check(ValidateInstanceUUIDForm(instID))
	}

	pri := principal.ContextID(ctx)
	if pri == nil {
		return nil
	}

	lock := s.mu.Lock()
	defer s.mu.Unlock()

	if s.accounts == nil {
		pan.Panic(ErrServerClosed)
	}

	acc := s.ensureAccount(lock, pri)

	if instID != "" {
		acc.mustCheckUniqueInstanceID(lock, instID)
	}

	return acc
}

// mustRunOrDeleteInstance steals the program reference (except on error).
func (s *Server) mustRunOrDeleteInstance(ctx Context, inst *Instance, prog *program, function string) {
	defer s.unrefProgram(&prog)

	drive, err := inst.startOrAnnihilate(prog)
	if err != nil {
		s.deleteNonexistentInstance(inst)
		pan.Panic(err)
	}

	if drive {
		go s.driveInstance(ctx, inst, prog, function)
		prog = nil
	}
}

// driveInstance steals the program reference.
func (s *Server) driveInstance(ctx Context, inst *Instance, prog *program, function string) {
	defer s.unrefProgram(&prog)

	if nonexistent := inst.drive(ctx, prog, function, &s.Config); nonexistent {
		s.deleteNonexistentInstance(inst)
	}
}

func (s *Server) mustGetInstance(ctx Context, instance string) *Instance {
	_, inst := s.mustGetInstanceProgramID(ctx, instance)
	return inst
}

func (s *Server) mustGetInstanceProgramID(ctx Context, instance string) (string, *Instance) {
	pri := principal.ContextID(ctx)
	if pri == nil {
		pan.Panic(errAnonymous)
	}

	lock := s.mu.Lock()
	defer s.mu.Unlock()

	inst, prog := s.mustGetInstanceBorrowProgram(lock, pri, instance)
	return prog.id, inst
}

func (s *Server) mustGetInstanceRefProgram(ctx Context, instance string) (*Instance, *program) {
	pri := principal.ContextID(ctx)
	if pri == nil {
		pan.Panic(errAnonymous)
	}

	lock := s.mu.Lock()
	defer s.mu.Unlock()

	inst, prog := s.mustGetInstanceBorrowProgram(lock, pri, instance)
	return inst, prog.ref(lock)
}

func (s *Server) mustGetInstanceBorrowProgram(lock serverLock, pri *principal.ID, instance string) (*Instance, *program) {
	acc := s.accounts[principal.Raw(pri)]
	if acc == nil {
		pan.Panic(notfound.ErrInstance)
	}

	x, found := acc.instances[instance]
	if !found {
		pan.Panic(notfound.ErrInstance)
	}

	return x.inst, x.prog
}

func (s *Server) mustAllocateInstanceResources(ctx Context, policy *InstancePolicy) (*runtime.Process, InstanceServices) {
	if policy.Services == nil {
		pan.Panic(PermissionDenied("no service policy"))
	}

	services := policy.Services(ctx)
	defer func() {
		if services != nil {
			services.Close()
		}
	}()

	proc := Must(s.ProcessFactory.NewProcess(ctx))

	ss := services
	services = nil
	return proc, ss
}

// mustRegisterProgramRefInstance with server, and an account if ref is true.
// Caller's instance image is stolen (except on error).  Caller's program
// reference is replaced with a reference to the canonical program object.
func (s *Server) mustRegisterProgramRefInstance(ctx Context, acc *account, prog *program, instImage *image.Instance, policy *InstancePolicy, know *api.ModuleOptions, launch *api.LaunchOptions) (inst *Instance, canonicalProg *program, redundantProg bool) {
	var (
		proc     *runtime.Process
		services InstanceServices
	)
	if !launch.Suspend && !instImage.Final() {
		proc, services = s.mustAllocateInstanceResources(ctx, policy)
		defer closeInstanceResources(&proc, &services)
	}

	if know.Pin || !launch.Transient {
		if acc == nil {
			pan.Panic(errAnonymous)
		}
		prog.mustEnsureStorage()
	}

	instance := launch.Instance
	if instance == "" {
		instance = makeInstanceID()
	}

	lock := s.mu.Lock()
	defer s.mu.Unlock()

	if acc != nil {
		if s.accounts == nil {
			pan.Panic(ErrServerClosed)
		}
		acc.mustCheckUniqueInstanceID(lock, instance)
	}

	prog, redundantProg = s.mustMergeProgramRef(lock, prog)

	inst = newInstance(instance, acc, launch.Transient, instImage, prog.buffers, proc, services, policy.TimeResolution, launch.Tags, s.openDebugLog(launch.Invoke))
	proc = nil
	services = nil

	if acc != nil {
		if know.Pin {
			// mergeProgramRef checked for shutdown, so ensureProgramRef is
			// safe to call.
			if acc.ensureProgramRef(lock, prog, know.Tags) {
				// TODO: move outside of critical section
				s.eventModule(ctx, event.TypeModulePin, &event.Module{
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

// mustMergeProgramRef steals the program reference and returns a borrowed
// program reference which is valid until the server mutex is unlocked.
func (s *Server) mustMergeProgramRef(lock serverLock, prog *program) (canonical *program, redundant bool) {
	switch existing := s.programs[prog.id]; existing {
	case nil:
		if s.programs == nil {
			pan.Panic(ErrServerClosed)
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

func (s *Server) mustGetSource(uri string) (Source, string) {
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

func mustPrepareModuleOptions(opt *api.ModuleOptions) *api.ModuleOptions {
	if opt == nil {
		return new(api.ModuleOptions)
	}
	return opt
}

func mustPrepareLaunchOptions(opt *api.LaunchOptions) *api.LaunchOptions {
	if opt == nil {
		return new(api.LaunchOptions)
	}
	if opt.Suspend && opt.Function != "" {
		pan.Panic(failrequest.Error(event.FailInstanceStatus, "function cannot be specified for suspended instance"))
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
