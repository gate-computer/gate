// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"
	"io"
	"log"
	"sync"

	"gate.computer/gate/image"
	"gate.computer/gate/internal/error/public"
	"gate.computer/gate/internal/error/resourcelimit"
	"gate.computer/gate/internal/principal"
	"gate.computer/gate/runtime"
	"gate.computer/gate/server/detail"
	"gate.computer/gate/server/event"
	"gate.computer/gate/server/internal/error/resourcenotfound"
	api "gate.computer/gate/serverapi"
	"github.com/tsavola/wag/object/stack"
)

const ErrServerClosed = public.Err("server closed")

var errAnonymous = AccessUnauthorized("anonymous access not supported")

type rawPrincipalKey [32]byte

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

type serverLock struct{}
type serverMutex struct{ sync.Mutex }

func (m *serverMutex) Lock() serverLock {
	m.Mutex.Lock()
	return serverLock{}
}

type Server struct {
	Config

	mu        serverMutex
	programs  map[string]*program
	accounts  map[rawPrincipalKey]*account
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
		accounts:  make(map[rawPrincipalKey]*account),
		anonymous: make(map[*Instance]struct{}),
	}

	progList, err := s.ImageStorage.Programs()
	if err != nil {
		return nil, err
	}

	instList, err := s.ImageStorage.Instances()
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

	for _, hash := range progList {
		if err := s.loadProgramDuringInit(lock, owner, hash); err != nil {
			return nil, err
		}
	}

	for _, key := range instList {
		if err := s.loadInstanceDuringInit(lock, key); err != nil {
			return nil, err
		}
	}

	shutdown = nil
	return s, nil
}

func (s *Server) loadProgramDuringInit(lock serverLock, owner *account, hash string) error {
	image, err := s.ImageStorage.LoadProgram(hash)
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

	prog := newProgram(hash, image, buffers, true)
	image = nil

	if owner != nil {
		owner.ensureProgramRef(lock, prog)
	}

	s.programs[hash] = prog

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
	func() {
		lock := s.mu.Lock()
		defer s.mu.Unlock()

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
	}()

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

// UploadModule creates a new module reference if ref is true.  Caller provides
// module content which is compiled or validated in any case.
func (s *Server) UploadModule(ctx context.Context, ref bool, allegedHash string, content io.ReadCloser, contentLength int64,
) (progHash string, err error) {
	defer closeReader(&content)

	var pol progPolicy

	ctx, err = s.AccessPolicy.AuthorizeProgram(ctx, &pol.res, &pol.prog)
	if err != nil {
		return
	}

	if contentLength > int64(pol.prog.MaxModuleSize) {
		err = resourcelimit.New("module size limit exceeded")
		return
	}

	// TODO: check resource policy

	if allegedHash != "" {
		var found bool

		found, err = s.loadKnownModule(ctx, ref, &pol, allegedHash, &content, contentLength)
		if err != nil {
			return
		}
		if found {
			progHash = allegedHash
			return
		}
	}

	progHash, err = s.loadUnknownModule(ctx, ref, &pol, allegedHash, content, int(contentLength))
	content = nil
	return
}

// SourceModule creates a new module reference if ref is true.  Module content
// is read from a source - it is compiled or validated in any case.
func (s *Server) SourceModule(ctx context.Context, ref bool, source Source, uri string,
) (progHash string, err error) {
	var pol progPolicy

	ctx, err = s.AccessPolicy.AuthorizeProgramSource(ctx, &pol.res, &pol.prog, source)
	if err != nil {
		return
	}

	size, content, err := source.OpenURI(ctx, uri, pol.prog.MaxModuleSize)
	if err != nil {
		return
	}
	if content == nil {
		if size > 0 {
			err = resourcelimit.New("program size limit exceeded")
			return
		}
		err = resourcenotfound.ErrModule
		return
	}

	return s.loadUnknownModule(ctx, ref, &pol, "", content, int(size))
}

// loadKnownModule might close the content reader and set it to nil.
func (s *Server) loadKnownModule(ctx context.Context, ref bool, pol *progPolicy, allegedHash string, content *io.ReadCloser, contentLength int64,
) (found bool, err error) {
	prog, err := s.refProgram(allegedHash, contentLength)
	if prog == nil || err != nil {
		return
	}
	defer s.unrefProgram(&prog)
	progHash := prog.hash
	found = true

	err = validateHashContent(allegedHash, content)
	if err != nil {
		return
	}

	if prog.image.TextSize() > pol.prog.MaxTextSize {
		err = resourcelimit.New("program code size limit exceeded")
		return
	}

	_, err = s.registerProgramRef(ctx, prog, ref)
	if err != nil {
		return
	}
	prog = nil

	s.monitor(&event.ModuleUploadExist{
		Ctx:    ContextDetail(ctx),
		Module: progHash,
	})
	return
}

// loadUnknownModule always closes the content reader.
func (s *Server) loadUnknownModule(ctx context.Context, ref bool, pol *progPolicy, allegedHash string, content io.ReadCloser, contentSize int,
) (progHash string, err error) {
	prog, _, err := buildProgram(s.ImageStorage, &pol.prog, nil, allegedHash, content, contentSize, "")
	if err != nil {
		return
	}
	defer s.unrefProgram(&prog)
	progHash = prog.hash

	redundant, err := s.registerProgramRef(ctx, prog, ref)
	if err != nil {
		return
	}
	prog = nil

	if redundant {
		s.monitor(&event.ModuleUploadExist{
			Ctx:      ContextDetail(ctx),
			Module:   progHash,
			Compiled: true,
		})
	} else {
		s.monitor(&event.ModuleUploadNew{
			Ctx:    ContextDetail(ctx),
			Module: progHash,
		})
	}
	return
}

// CreateInstance instantiates a module reference.  Instance id and debug log
// are optional.  Debug log will be closed.
func (s *Server) CreateInstance(ctx context.Context, progHash, instID, function string, transient, suspend bool, debugLog io.WriteCloser,
) (inst *Instance, err error) {
	defer closeWriter(&debugLog)

	if suspend {
		if function != "" {
			err = public.Err("function cannot be specified for suspended instance")
			return
		}
		if debugLog != nil {
			err = public.Err("debug logging cannot be enabled when suspending instance")
			return
		}
	}

	var pol instPolicy

	ctx, err = s.AccessPolicy.AuthorizeInstance(ctx, &pol.res, &pol.inst)
	if err != nil {
		return
	}

	acc, err := s.checkAccountInstanceID(ctx, instID)
	if err != nil {
		return
	}
	if acc == nil {
		err = errAnonymous
		return
	}

	prog := func() *program {
		lock := s.mu.Lock()
		defer s.mu.Unlock()

		prog := s.programs[progHash]
		if prog == nil {
			return nil
		}

		return acc.refProgram(lock, prog)
	}()
	if prog == nil {
		err = resourcenotfound.ErrModule
		return
	}
	defer s.unrefProgram(&prog)

	funcIndex, err := prog.image.ResolveEntryFunc(function, false)
	if err != nil {
		return
	}

	// TODO: check resource policy (text/stack/memory/max-memory size etc.)

	instImage, err := image.NewInstance(prog.image, pol.inst.MaxMemorySize, pol.inst.StackSize, funcIndex)
	if err != nil {
		return
	}
	defer closeInstanceImage(&instImage)

	inst, prog, _, err = s.registerProgramRefInstance(ctx, acc, false, prog, instImage, &pol.inst, transient, instID, debugLog, suspend)
	if err != nil {
		return
	}
	instImage = nil
	debugLog = nil

	err = s.runOrDeleteInstance(ctx, inst, prog, function)
	if err != nil {
		return
	}
	prog = nil

	s.monitor(&event.InstanceCreateLocal{
		Ctx:      ContextDetail(ctx),
		Instance: inst.ID,
		Module:   progHash,
	})
	return
}

// UploadModuleInstance creates a new module reference if ref is true.  The
// module is instantiated in any case.  Caller provides module content.
// Instance id and debug log are optional.  Debug log will be closed.
func (s *Server) UploadModuleInstance(ctx context.Context, content io.ReadCloser, contentLength int64, allegedHash string, ref bool, instID, function string, transient, suspend bool, debugLog io.WriteCloser,
) (inst *Instance, err error) {
	defer closeReader(&content)
	defer closeWriter(&debugLog)

	var pol instProgPolicy

	ctx, err = s.AccessPolicy.AuthorizeProgramInstance(ctx, &pol.res, &pol.prog, &pol.inst)
	if err != nil {
		return
	}

	acc, err := s.checkAccountInstanceID(ctx, instID)
	if err != nil {
		return
	}

	_, inst, err = s.loadModuleInstance(ctx, acc, ref, &pol, allegedHash, content, contentLength, transient, function, instID, debugLog, suspend)
	content = nil
	debugLog = nil
	return
}

// SourceModuleInstance creates a new module reference if ref is true.  The
// module is instantiated in any case.  Module content is read from a source.
// Instance id and debug log are optional.  Debug log will be closed.
func (s *Server) SourceModuleInstance(ctx context.Context, source Source, uri string, ref bool, instID, function string, transient, suspend bool, debugLog io.WriteCloser,
) (progHash string, inst *Instance, err error) {
	defer closeWriter(&debugLog)

	var pol instProgPolicy

	ctx, err = s.AccessPolicy.AuthorizeProgramInstanceSource(ctx, &pol.res, &pol.prog, &pol.inst, source)
	if err != nil {
		return
	}

	acc, err := s.checkAccountInstanceID(ctx, instID)
	if err != nil {
		return
	}

	size, content, err := source.OpenURI(ctx, uri, pol.prog.MaxModuleSize)
	if err != nil {
		return
	}
	if content == nil {
		if size > 0 {
			err = resourcelimit.New("program size limit exceeded")
			return
		}
		err = resourcenotfound.ErrModule
		return
	}

	progHash, inst, err = s.loadModuleInstance(ctx, acc, ref, &pol, "", content, int64(size), transient, function, instID, debugLog, suspend)
	debugLog = nil
	return
}

func (s *Server) loadModuleInstance(ctx context.Context, acc *account, ref bool, pol *instProgPolicy, allegedHash string, content io.ReadCloser, contentLength int64, transient bool, function, instID string, debugLog io.WriteCloser, suspend bool,
) (progHash string, inst *Instance, err error) {
	defer closeReader(&content)
	defer closeWriter(&debugLog)

	if suspend {
		if function != "" {
			err = public.Err("function cannot be specified for suspended instance")
			return
		}
		if debugLog != nil {
			err = public.Err("debug logging cannot be enabled when suspending instance")
			return
		}
	}

	if contentLength > int64(pol.prog.MaxModuleSize) {
		err = resourcelimit.New("module size limit exceeded")
		return
	}

	// TODO: check resource policy

	if allegedHash != "" {
		inst, err = s.loadKnownModuleInstance(ctx, acc, ref, pol, allegedHash, &content, contentLength, transient, function, instID, debugLog, suspend)
		if err != nil {
			return
		}
		if inst != nil {
			debugLog = nil
			progHash = allegedHash
			return
		}
	}

	progHash, inst, err = s.loadUnknownModuleInstance(ctx, acc, ref, pol, allegedHash, content, int(contentLength), transient, function, instID, debugLog, suspend)
	content = nil
	debugLog = nil
	return
}

// loadKnownModuleInstance might close the content reader and set it to nil.
func (s *Server) loadKnownModuleInstance(ctx context.Context, acc *account, ref bool, pol *instProgPolicy, allegedHash string, content *io.ReadCloser, contentLength int64, transient bool, function, instID string, debugLog io.WriteCloser, suspend bool,
) (inst *Instance, err error) {
	defer closeWriter(&debugLog)

	prog, err := s.refProgram(allegedHash, contentLength)
	if prog == nil || err != nil {
		return
	}
	defer s.unrefProgram(&prog)
	progHash := prog.hash

	err = validateHashContent(prog.hash, content)
	if err != nil {
		return
	}

	if prog.image.TextSize() > pol.prog.MaxTextSize {
		err = resourcelimit.New("program code size limit exceeded")
		return
	}

	// TODO: check resource policy (stack/memory/max-memory size etc.)

	funcIndex, err := prog.image.ResolveEntryFunc(function, false)
	if err != nil {
		return
	}

	instImage, err := image.NewInstance(prog.image, pol.inst.MaxMemorySize, pol.inst.StackSize, funcIndex)
	if err != nil {
		return
	}
	defer closeInstanceImage(&instImage)

	inst, prog, _, err = s.registerProgramRefInstance(ctx, acc, ref, prog, instImage, &pol.inst, transient, instID, debugLog, suspend)
	if err != nil {
		return
	}
	instImage = nil
	debugLog = nil

	s.monitor(&event.ModuleUploadExist{
		Ctx:    ContextDetail(ctx),
		Module: progHash,
	})

	err = s.runOrDeleteInstance(ctx, inst, prog, function)
	if err != nil {
		return
	}
	prog = nil

	s.monitor(&event.InstanceCreateLocal{
		Ctx:      ContextDetail(ctx),
		Instance: inst.ID,
		Module:   progHash,
	})
	return
}

// loadUnknownModuleInstance always closes the content reader.
func (s *Server) loadUnknownModuleInstance(ctx context.Context, acc *account, ref bool, pol *instProgPolicy, allegedHash string, content io.ReadCloser, contentSize int, transient bool, function, instID string, debugLog io.WriteCloser, suspend bool,
) (progHash string, inst *Instance, err error) {
	defer closeWriter(&debugLog)

	prog, instImage, err := buildProgram(s.ImageStorage, &pol.prog, &pol.inst, allegedHash, content, contentSize, function)
	if err != nil {
		return
	}
	defer closeInstanceImage(&instImage)
	defer s.unrefProgram(&prog)
	progHash = prog.hash

	inst, prog, redundantProg, err := s.registerProgramRefInstance(ctx, acc, ref, prog, instImage, &pol.inst, transient, instID, debugLog, suspend)
	if err != nil {
		return
	}
	instImage = nil
	debugLog = nil

	if allegedHash != "" {
		if redundantProg {
			s.monitor(&event.ModuleUploadExist{
				Ctx:      ContextDetail(ctx),
				Module:   progHash,
				Compiled: true,
			})
		} else {
			s.monitor(&event.ModuleUploadNew{
				Ctx:    ContextDetail(ctx),
				Module: progHash,
			})
		}
	} else {
		if redundantProg {
			s.monitor(&event.ModuleSourceExist{
				Ctx:    ContextDetail(ctx),
				Module: progHash,
				// TODO: source URI
				Compiled: true,
			})
		} else {
			s.monitor(&event.ModuleSourceNew{
				Ctx:    ContextDetail(ctx),
				Module: progHash,
				// TODO: source URI
			})
		}
	}

	err = s.runOrDeleteInstance(ctx, inst, prog, function)
	if err != nil {
		return
	}
	prog = nil

	s.monitor(&event.InstanceCreateStream{
		Ctx:      ContextDetail(ctx),
		Instance: inst.ID,
		Module:   progHash,
	})
	return
}

func (s *Server) ModuleRefs(ctx context.Context) (refs api.ModuleRefs, err error) {
	ctx, err = s.AccessPolicy.Authorize(ctx)
	if err != nil {
		return
	}

	pri := principal.ContextID(ctx)
	if pri == nil {
		err = errAnonymous
		return
	}

	s.monitor(&event.ModuleList{
		Ctx: ContextDetail(ctx),
	})

	s.mu.Lock()
	defer s.mu.Unlock()

	acc := s.accounts[principal.Raw(pri)]
	if acc == nil {
		return
	}

	refs.Modules = make([]api.ModuleRef, 0, len(acc.programs))
	for prog := range acc.programs {
		refs.Modules = append(refs.Modules, api.ModuleRef{
			Id: prog.hash,
		})
	}

	return
}

// ModuleContent for downloading.
func (s *Server) ModuleContent(ctx context.Context, hash string,
) (content io.ReadCloser, length int64, err error) {
	ctx, err = s.AccessPolicy.Authorize(ctx)
	if err != nil {
		return
	}

	pri := principal.ContextID(ctx)
	if pri == nil {
		err = errAnonymous
		return
	}

	prog := func() *program {
		lock := s.mu.Lock()
		defer s.mu.Unlock()

		acc := s.accounts[principal.Raw(pri)]
		if acc == nil {
			return nil
		}

		prog := s.programs[hash]
		if prog == nil {
			return nil
		}

		return acc.refProgram(lock, prog)
	}()
	if prog == nil {
		err = resourcenotfound.ErrModule
		return
	}

	length = prog.image.ModuleSize()
	content = &moduleContent{
		ctx:   ContextDetail(ctx),
		r:     prog.image.NewModuleReader(),
		s:     s,
		prog:  prog,
		total: length,
	}
	return
}

type moduleContent struct {
	ctx   detail.Context
	r     io.Reader
	s     *Server
	prog  *program
	total int64
	read  int64
}

func (x *moduleContent) Read(b []byte) (n int, err error) {
	n, err = x.r.Read(b)
	x.read += int64(n)
	return
}

func (x *moduleContent) Close() (err error) {
	x.s.monitor(&event.ModuleDownload{
		Ctx:          x.ctx,
		Module:       x.prog.hash,
		ModuleLength: uint64(x.total),
		LengthRead:   uint64(x.read),
	})

	x.s.unrefProgram(&x.prog)
	return
}

func (s *Server) UnrefModule(ctx context.Context, hash string) (err error) {
	ctx, err = s.AccessPolicy.Authorize(ctx)
	if err != nil {
		return
	}

	pri := principal.ContextID(ctx)
	if pri == nil {
		err = errAnonymous
		return
	}

	found := func() bool {
		lock := s.mu.Lock()
		defer s.mu.Unlock()

		acc := s.accounts[principal.Raw(pri)]
		if acc == nil {
			return false
		}

		prog := s.programs[hash]
		if prog == nil {
			return false
		}

		return acc.unrefProgram(lock, prog)
	}()
	if !found {
		err = resourcenotfound.ErrModule
		return
	}

	s.monitor(&event.ModuleUnref{
		Ctx:    ContextDetail(ctx),
		Module: hash,
	})
	return
}

func (s *Server) InstanceConnection(ctx context.Context, instID string,
) (inst *Instance, connIO func(context.Context, io.Reader, io.Writer) error, err error) {
	ctx, err = s.AccessPolicy.Authorize(ctx)
	if err != nil {
		return
	}

	inst, err = s.getInstance(ctx, instID)
	if err != nil {
		return
	}

	conn := inst.connect(ctx)
	if conn == nil {
		s.monitor(&event.FailRequest{
			Ctx:      ContextDetail(ctx),
			Failure:  event.FailInstanceNoConnect,
			Instance: inst.ID,
		})
		return
	}

	connIO = func(ctx context.Context, r io.Reader, w io.Writer) (err error) {
		s.monitor(&event.InstanceConnect{
			Ctx:      ContextDetail(ctx),
			Instance: inst.ID,
		})

		err = conn(ctx, r, w)

		s.Monitor(&event.InstanceDisconnect{
			Ctx:      ContextDetail(ctx),
			Instance: inst.ID,
		}, err)
		return
	}
	return
}

// InstanceStatus of an existing instance.
func (s *Server) InstanceStatus(ctx context.Context, instID string,
) (status api.Status, err error) {
	ctx, err = s.AccessPolicy.Authorize(ctx)
	if err != nil {
		return
	}

	inst, err := s.getInstance(ctx, instID)
	if err != nil {
		return
	}

	s.monitor(&event.InstanceStatus{
		Ctx:      ContextDetail(ctx),
		Instance: inst.ID,
	})

	status = inst.Status()
	return
}

func (s *Server) WaitInstance(ctx context.Context, instID string,
) (status api.Status, err error) {
	ctx, err = s.AccessPolicy.Authorize(ctx)
	if err != nil {
		return
	}

	inst, err := s.getInstance(ctx, instID)
	if err != nil {
		return
	}

	s.monitor(&event.InstanceWait{
		Ctx:      ContextDetail(ctx),
		Instance: inst.ID,
	})

	status = inst.Wait(ctx)
	return
}

func (s *Server) KillInstance(ctx context.Context, instID string,
) (inst *Instance, err error) {
	ctx, err = s.AccessPolicy.Authorize(ctx)
	if err != nil {
		return
	}

	inst, err = s.getInstance(ctx, instID)
	if err != nil {
		return
	}

	s.monitor(&event.InstanceKill{
		Ctx:      ContextDetail(ctx),
		Instance: inst.ID,
	})

	inst.Kill()
	return
}

func (s *Server) SuspendInstance(ctx context.Context, instID string,
) (inst *Instance, err error) {
	ctx, err = s.AccessPolicy.Authorize(ctx)
	if err != nil {
		return
	}

	// Store the program in case the instance becomes non-transient.
	inst, prog, err := s.getInstanceRefProgram(ctx, instID)
	if err != nil {
		return
	}
	defer s.unrefProgram(&prog)

	err = prog.ensureStorage()
	if err != nil {
		return
	}

	s.monitor(&event.InstanceSuspend{
		Ctx:      ContextDetail(ctx),
		Instance: inst.ID,
	})

	inst.Suspend()
	return
}

// ResumeInstance.  Instance id and debug log are optional.  Debug log will be
// closed.
func (s *Server) ResumeInstance(ctx context.Context, instID, function string, debugLog io.WriteCloser,
) (inst *Instance, err error) {
	defer closeWriter(&debugLog)

	var pol instPolicy

	ctx, err = s.AccessPolicy.AuthorizeInstance(ctx, &pol.res, &pol.inst)
	if err != nil {
		return
	}

	inst, prog, err := s.getInstanceRefProgram(ctx, instID)
	if err != nil {
		return
	}
	defer s.unrefProgram(&prog)

	err = inst.checkResume(function)
	if err != nil {
		return
	}

	proc, services, err := s.allocateInstanceResources(ctx, &pol.inst)
	if err != nil {
		return
	}
	defer closeInstanceResources(&proc, &services)

	err = inst.doResume(function, proc, services, pol.inst.TimeResolution, debugLog)
	if err != nil {
		return
	}
	proc = nil
	services = nil
	debugLog = nil

	err = s.runOrDeleteInstance(ctx, inst, prog, function)
	if err != nil {
		return
	}
	prog = nil

	s.monitor(&event.InstanceResume{
		Ctx:      ContextDetail(ctx),
		Instance: inst.ID,
		Function: function,
	})
	return
}

func (s *Server) DeleteInstance(ctx context.Context, instID string,
) (err error) {
	ctx, err = s.AccessPolicy.Authorize(ctx)
	if err != nil {
		return
	}

	inst, err := s.getInstance(ctx, instID)
	if err != nil {
		return
	}

	err = inst.annihilate()
	if err != nil {
		return
	}

	s.monitor(&event.InstanceDelete{
		Ctx:      ContextDetail(ctx),
		Instance: inst.ID,
	})

	s.deleteNonexistentInstance(inst)
	return
}

func (s *Server) InstanceModule(ctx context.Context, instID string) (moduleKey string, err error) {
	// TODO: implement suspend-snapshot-resume at a lower level

	inst, err := s.getInstance(ctx, instID)
	if err != nil {
		return
	}

	status := inst.Status()
	resume := false
	if status.State == api.StateRunning {
		inst.suspend()
		resume = inst.Wait(context.Background()).State == api.StateSuspended
	}

	moduleKey, err = s.snapshot(ctx, instID)

	if resume {
		if _, e := s.ResumeInstance(ctx, instID, "", nil); err == nil {
			err = e
		}
	}

	return
}

func (s *Server) snapshot(ctx context.Context, instID string) (moduleKey string, err error) {
	ctx, err = s.AccessPolicy.Authorize(ctx)
	if err != nil {
		return
	}

	// TODO: check module storage limits

	inst, oldProg, err := s.getInstanceRefProgram(ctx, instID)
	if err != nil {
		return
	}
	defer s.unrefProgram(&oldProg)

	newImage, buffers, err := inst.snapshot(oldProg)
	if err != nil {
		return
	}
	defer closeProgramImage(&newImage)

	h := newHash()
	_, err = io.Copy(h, newImage.NewModuleReader())
	if err != nil {
		return
	}
	moduleKey = hashEncoding.EncodeToString(h.Sum(nil))

	err = newImage.Store(moduleKey)
	if err != nil {
		return
	}

	newProg := newProgram(moduleKey, newImage, buffers, true)
	newImage = nil
	defer s.unrefProgram(&newProg)

	_, err = s.registerProgramRef(ctx, newProg, true)
	if err != nil {
		return
	}
	newProg = nil

	s.monitor(&event.InstanceSnapshot{
		Ctx:      ContextDetail(ctx),
		Instance: inst.ID,
		Module:   moduleKey,
	})
	return
}

func (s *Server) DebugInstance(ctx context.Context, instID string, req api.DebugRequest,
) (res api.DebugResponse, err error) {
	var pol progPolicy

	ctx, err = s.AccessPolicy.AuthorizeProgram(ctx, &pol.res, &pol.prog)
	if err != nil {
		return
	}

	inst, defaultProg, err := s.getInstanceRefProgram(ctx, instID)
	if err != nil {
		return
	}
	defer s.unrefProgram(&defaultProg)

	rebuild, config, res, err := inst.debug(ctx, defaultProg, req)
	if err != nil {
		return
	}

	if rebuild != nil {
		var (
			progImage *image.Program
			textMap   stack.TextMap
			ok        bool
		)

		progImage, textMap, err = rebuildProgramImage(s.ImageStorage, &pol.prog, defaultProg.image.NewModuleReader(), config.DebugInfo, config.Breakpoints.Offsets)
		if err != nil {
			return
		}
		defer func() {
			if progImage != nil {
				progImage.Close()
			}
		}()

		res, ok = rebuild.apply(progImage, config, textMap)
		if !ok {
			err = public.Err("conflict") // TODO: http response code: conflict
			return
		}
		progImage = nil
	}

	s.monitor(&event.InstanceDebug{
		Ctx:      ContextDetail(ctx),
		Instance: inst.ID,
		Compiled: rebuild != nil,
	})
	return
}

func (s *Server) Instances(ctx context.Context) (statuses api.Instances, err error) {
	ctx, err = s.AccessPolicy.Authorize(ctx)
	if err != nil {
		return
	}

	pri := principal.ContextID(ctx)
	if pri == nil {
		err = errAnonymous
		return
	}

	s.monitor(&event.InstanceList{
		Ctx: ContextDetail(ctx),
	})

	// Get instance references while holding server lock.
	is := func() (is []*Instance) {
		s.mu.Lock()
		defer s.mu.Unlock()

		if acc := s.accounts[principal.Raw(pri)]; acc != nil {
			is = make([]*Instance, 0, len(acc.instances))
			for _, x := range acc.instances {
				is = append(is, x.inst)
			}
		}

		return
	}()

	// Get instance statuses.  Each instance has its own lock.
	statuses.Instances = make([]api.InstanceStatus, len(is))
	for i, inst := range is {
		statuses.Instances[i] = inst.instanceStatus()
	}

	return
}

// ensureAccount must not be called while the server is shutting down.
func (s *Server) ensureAccount(_ serverLock, pri *principal.ID) (acc *account) {
	acc = s.accounts[principal.Raw(pri)]
	if acc == nil {
		acc = newAccount(pri)
		s.accounts[principal.Raw(pri)] = acc
	}
	return
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

	lock := s.mu.Lock()
	defer s.mu.Unlock()

	prog.unref(lock)
}

// registerProgramRef with the server and an account.  Caller's program
// reference is stolen (except on error).
func (s *Server) registerProgramRef(ctx context.Context, prog *program, ref bool,
) (redundant bool, err error) {
	var pri *principal.ID

	if ref {
		pri = principal.ContextID(ctx)
		if pri == nil {
			err = errAnonymous
			return
		}

		err = prog.ensureStorage()
		if err != nil {
			return
		}
	}

	lock := s.mu.Lock()
	defer s.mu.Unlock()

	prog, redundant, err = s.mergeProgramRef(lock, prog)
	if err != nil {
		return
	}

	if ref {
		// mergeProgramRef checked for shutdown, so the ensure methods are safe
		// to call.
		s.ensureAccount(lock, pri).ensureProgramRef(lock, prog)
	}

	return
}

func (s *Server) checkAccountInstanceID(ctx context.Context, instID string) (acc *account, err error) {
	if instID != "" {
		err = validateInstanceID(instID)
		if err != nil {
			return
		}
	}

	pri := principal.ContextID(ctx)
	if pri == nil {
		return
	}

	lock := s.mu.Lock()
	defer s.mu.Unlock()

	if s.accounts == nil {
		err = ErrServerClosed
		return
	}

	acc = s.ensureAccount(lock, pri)

	if instID != "" {
		err = acc.checkUniqueInstanceID(lock, instID)
		if err != nil {
			return
		}
	}

	return
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

	if event, err := inst.drive(ctx, prog, function); event != nil {
		s.Monitor(event, err)
	}

	if inst.Transient() {
		if inst.annihilate() == nil {
			s.deleteNonexistentInstance(inst)
		}
	}
}

func (s *Server) getInstance(ctx context.Context, instID string) (inst *Instance, err error) {
	pri := principal.ContextID(ctx)
	if pri == nil {
		err = errAnonymous
		return
	}

	lock := s.mu.Lock()
	defer s.mu.Unlock()

	x, err := s.getInstanceBorrowProgram(lock, pri, instID)
	if err != nil {
		return
	}

	return x.inst, nil
}

func (s *Server) getInstanceRefProgram(ctx context.Context, instID string,
) (inst *Instance, prog *program, err error) {
	pri := principal.ContextID(ctx)
	if pri == nil {
		err = errAnonymous
		return
	}

	lock := s.mu.Lock()
	defer s.mu.Unlock()

	x, err := s.getInstanceBorrowProgram(lock, pri, instID)
	if err != nil {
		return
	}

	return x.inst, x.prog.ref(lock), nil
}

func (s *Server) getInstanceBorrowProgram(_ serverLock, pri *principal.ID, instID string,
) (x accountInstance, err error) {
	acc := s.accounts[principal.Raw(pri)]
	if acc == nil {
		err = resourcenotfound.ErrInstance
		return
	}

	x, found := acc.instances[instID]
	if !found {
		err = resourcenotfound.ErrInstance
		return
	}

	return
}

func (s *Server) allocateInstanceResources(ctx context.Context, pol *InstancePolicy,
) (proc *runtime.Process, services InstanceServices, err error) {
	defer func() {
		if err != nil {
			closeInstanceResources(&proc, &services)
		}
	}()

	if pol.Services == nil {
		err = AccessForbidden("no service policy")
		return
	}
	services = pol.Services(ctx)

	proc, err = s.ProcessFactory.NewProcess(ctx)
	return
}

// registerProgramRefInstance with server, and an account if ref is true.
// Caller's instance image is stolen (except on error).  Caller's program
// reference is replaced with a reference to the canonical program object.
func (s *Server) registerProgramRefInstance(ctx context.Context, acc *account, ref bool, prog *program, instImage *image.Instance, pol *InstancePolicy, transient bool, instID string, debugLog io.WriteCloser, suspend bool,
) (inst *Instance, canonicalProg *program, redundantProg bool, err error) {
	defer closeWriter(&debugLog)

	var (
		proc     *runtime.Process
		services InstanceServices
	)
	if !suspend && !instImage.Final() {
		proc, services, err = s.allocateInstanceResources(ctx, pol)
		if err != nil {
			return
		}
		defer closeInstanceResources(&proc, &services)
	}

	if ref || !transient {
		if acc == nil {
			err = errAnonymous
			return
		}

		err = prog.ensureStorage()
		if err != nil {
			return
		}
	}

	if instID == "" {
		instID = makeInstanceID()
	}

	lock := s.mu.Lock()
	defer s.mu.Unlock()

	if acc != nil {
		if s.accounts == nil {
			err = ErrServerClosed
			return
		}

		err = acc.checkUniqueInstanceID(lock, instID)
		if err != nil {
			return
		}
	}

	prog, redundantProg, err = s.mergeProgramRef(lock, prog)
	if err != nil {
		return
	}

	inst = newInstance(instID, acc, transient, instImage, prog.buffers, proc, services, pol.TimeResolution, debugLog)
	proc = nil
	services = nil
	debugLog = nil

	if acc != nil {
		if ref {
			// mergeProgramRef checked for shutdown, so ensureProgramRef is
			// safe to call.
			acc.ensureProgramRef(lock, prog)
		}
		acc.instances[instID] = accountInstance{inst, prog.ref(lock)}
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
// reference which valid until the server mutex is unlocked.
func (s *Server) mergeProgramRef(lock serverLock, prog *program,
) (canonical *program, redundant bool, err error) {
	switch existing := s.programs[prog.hash]; existing {
	case nil:
		if s.programs == nil {
			return nil, false, ErrServerClosed
		}
		s.programs[prog.hash] = prog // Pass reference to map.
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

func closeReader(p *io.ReadCloser) {
	if *p != nil {
		(*p).Close()
		*p = nil
	}
}

func closeWriter(p *io.WriteCloser) {
	if *p != nil {
		(*p).Close()
		*p = nil
	}
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
