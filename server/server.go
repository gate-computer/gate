// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"
	"io"
	"sync"

	"github.com/tsavola/gate/image"
	"github.com/tsavola/gate/internal/error/public"
	"github.com/tsavola/gate/internal/error/resourcelimit"
	inprincipal "github.com/tsavola/gate/internal/principal"
	"github.com/tsavola/gate/principal"
	"github.com/tsavola/gate/runtime"
	"github.com/tsavola/gate/server/detail"
	"github.com/tsavola/gate/server/event"
	"github.com/tsavola/gate/server/internal/error/resourcenotfound"
	api "github.com/tsavola/gate/serverapi"
	"github.com/tsavola/gate/snapshot"
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

func New(config Config) (*Server, error) {
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

	list, err := s.ImageStorage.Programs()
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
	if s.XXX_Owner != nil {
		owner = newAccount(s.XXX_Owner)
		s.accounts[inprincipal.Raw(s.XXX_Owner)] = owner
	}

	for _, hash := range list {
		if err := s.loadProgramDuringInit(lock, owner, hash); err != nil {
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
	if image == nil { // Race condition with human?
		return nil
	}
	defer func() {
		closeProgramImage(&image)
	}()

	buffers, err := image.LoadBuffers()
	if err != nil {
		return err
	}

	prog := newProgram(hash, image, buffers, true)
	image = nil

	s.programs[hash] = prog

	if owner != nil {
		owner.ensureRefProgram(lock, prog)
	}

	return nil
}

func (s *Server) Shutdown(ctx context.Context) (err error) {
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
			for _, x := range acc.cleanup(lock) {
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
		err = ctx.Err()
	}
	return
}

// UploadModule creates a new module reference if ref is true.  Caller provides
// module content which is compiled or validated in any case.
func (s *Server) UploadModule(ctx context.Context, ref bool, allegedHash string, content io.ReadCloser, contentLength int64,
) (progHash string, err error) {
	defer func() {
		closeReader(&content)
	}()

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
	defer func() {
		s.unrefProgram(&prog)
	}()
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
	defer func() {
		s.unrefProgram(&prog)
	}()
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

// CreateInstance instantiates a module reference.  Instance id is optional.
func (s *Server) CreateInstance(ctx context.Context, progHash string, transient bool, function string, instID, debug string, suspend bool,
) (inst *Instance, err error) {
	if suspend {
		if function != "" {
			err = public.Err("function cannot be specified for suspended instance")
			return
		}
		if debug != "" {
			err = public.Err("debug option is not available for suspended instance")
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

		_, own := acc.programRefs[prog]
		if !own {
			return nil
		}

		return prog.ref(lock)
	}()
	if prog == nil {
		err = resourcenotfound.ErrModule
		return
	}
	defer func() {
		s.unrefProgram(&prog)
	}()

	entryIndex, err := prog.image.ResolveEntryFunc(function, false)
	if err != nil {
		return
	}

	// TODO: check resource policy (text/stack/memory/max-memory size etc.)

	instImage, err := image.NewInstance(prog.image, pol.inst.MaxMemorySize, pol.inst.StackSize, entryIndex)
	if err != nil {
		return
	}
	defer func() {
		closeInstanceImage(&instImage)
	}()

	inst, prog, _, err = s.registerProgramRefInstance(ctx, acc, false, prog, instImage, &pol.inst, transient, instID, debug, suspend)
	if err != nil {
		return
	}
	instImage = nil

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
// Instance id is optional.
func (s *Server) UploadModuleInstance(ctx context.Context, ref bool, allegedHash string, content io.ReadCloser, contentLength int64, transient bool, function, instID, debug string, suspend bool,
) (inst *Instance, err error) {
	defer func() {
		closeReader(&content)
	}()

	var pol instProgPolicy

	ctx, err = s.AccessPolicy.AuthorizeProgramInstance(ctx, &pol.res, &pol.prog, &pol.inst)
	if err != nil {
		return
	}

	acc, err := s.checkAccountInstanceID(ctx, instID)
	if err != nil {
		return
	}

	_, inst, err = s.loadModuleInstance(ctx, acc, ref, &pol, allegedHash, content, contentLength, transient, function, instID, debug, suspend)
	content = nil
	return
}

// SourceModuleInstance creates a new module reference if ref is true.  The
// module is instantiated in any case.  Module content is read from a source.
// Instance id is optional.
func (s *Server) SourceModuleInstance(ctx context.Context, ref bool, source Source, uri string, transient bool, function, instID, debug string, suspend bool,
) (progHash string, inst *Instance, err error) {
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

	progHash, inst, err = s.loadModuleInstance(ctx, acc, ref, &pol, "", content, int64(size), transient, function, instID, debug, suspend)
	return
}

func (s *Server) loadModuleInstance(ctx context.Context, acc *account, ref bool, pol *instProgPolicy, allegedHash string, content io.ReadCloser, contentLength int64, transient bool, function, instID, debug string, suspend bool,
) (progHash string, inst *Instance, err error) {
	defer func() {
		closeReader(&content)
	}()

	if suspend {
		if function != "" {
			err = public.Err("function cannot be specified for suspended instance")
			return
		}
		if debug != "" {
			err = public.Err("debug option is not available for suspended instance")
			return
		}
	}

	if contentLength > int64(pol.prog.MaxModuleSize) {
		err = resourcelimit.New("module size limit exceeded")
		return
	}

	// TODO: check resource policy

	if allegedHash != "" {
		inst, err = s.loadKnownModuleInstance(ctx, acc, ref, pol, allegedHash, &content, contentLength, transient, function, instID, debug, suspend)
		if err != nil {
			return
		}
		if inst != nil {
			progHash = allegedHash
			return
		}
	}

	progHash, inst, err = s.loadUnknownModuleInstance(ctx, acc, ref, pol, allegedHash, content, int(contentLength), transient, function, instID, debug, suspend)
	content = nil
	return
}

// loadKnownModuleInstance might close the content reader and set it to nil.
func (s *Server) loadKnownModuleInstance(ctx context.Context, acc *account, ref bool, pol *instProgPolicy, allegedHash string, content *io.ReadCloser, contentLength int64, transient bool, function, instID, debug string, suspend bool,
) (inst *Instance, err error) {
	prog, err := s.refProgram(allegedHash, contentLength)
	if prog == nil || err != nil {
		return
	}
	defer func() {
		s.unrefProgram(&prog)
	}()
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

	entryFunc, err := prog.image.ResolveEntryFunc(function, false)
	if err != nil {
		return
	}

	instImage, err := image.NewInstance(prog.image, pol.inst.MaxMemorySize, pol.inst.StackSize, entryFunc)
	if err != nil {
		return
	}
	defer func() {
		closeInstanceImage(&instImage)
	}()

	inst, prog, _, err = s.registerProgramRefInstance(ctx, acc, ref, prog, instImage, &pol.inst, transient, instID, debug, suspend)
	if err != nil {
		return
	}
	instImage = nil

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
func (s *Server) loadUnknownModuleInstance(ctx context.Context, acc *account, ref bool, pol *instProgPolicy, allegedHash string, content io.ReadCloser, contentSize int, transient bool, function, instID, debug string, suspend bool,
) (progHash string, inst *Instance, err error) {
	prog, instImage, err := buildProgram(s.ImageStorage, &pol.prog, &pol.inst, allegedHash, content, contentSize, function)
	if err != nil {
		return
	}
	defer func() {
		closeInstanceImage(&instImage)
		s.unrefProgram(&prog)
	}()
	progHash = prog.hash

	inst, prog, redundantProg, err := s.registerProgramRefInstance(ctx, acc, ref, prog, instImage, &pol.inst, transient, instID, debug, suspend)
	if err != nil {
		return
	}
	instImage = nil

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

	acc := s.accounts[inprincipal.Raw(pri)]
	if acc == nil {
		return
	}

	refs.Modules = make([]api.ModuleRef, 0, len(acc.programRefs))
	for prog := range acc.programRefs {
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

		acc := s.accounts[inprincipal.Raw(pri)]
		if acc == nil {
			return nil
		}

		prog := s.programs[hash]
		if prog == nil {
			return nil
		}

		_, own := acc.programRefs[prog]
		if !own {
			return nil
		}

		return prog.ref(lock)
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

		acc := s.accounts[inprincipal.Raw(pri)]
		if acc == nil {
			return false
		}

		prog := s.programs[hash]
		if prog == nil {
			return false
		}

		_, own := acc.programRefs[prog]
		if !own {
			return false
		}

		acc.unrefProgram(lock, prog)
		return true
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

	inst, _, err = s.getInstance(ctx, instID)
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

	inst, _, err := s.getInstance(ctx, instID)
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

	inst, _, err := s.getInstance(ctx, instID)
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

	inst, _, err = s.getInstance(ctx, instID)
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
	inst, err = s.getInstanceEnsureProgramStorage(ctx, instID)
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

func (s *Server) ResumeInstance(ctx context.Context, function, instID, debug string,
) (inst *Instance, err error) {
	var pol instPolicy

	ctx, err = s.AccessPolicy.AuthorizeInstance(ctx, &pol.res, &pol.inst)
	if err != nil {
		return
	}

	inst, prog, err := s.getInstance(ctx, instID)
	if err != nil {
		return
	}

	err = inst.checkResume(prog, function)
	if err != nil {
		return
	}

	proc, services, debugStatus, debugLog, err := s.allocateInstanceResources(ctx, &pol.inst, debug)
	if err != nil {
		return
	}
	defer func() {
		closeInstanceResources(&proc, &services, &debugLog)
	}()

	err = inst.doResume(prog, function, proc, services, pol.inst.TimeResolution, debugStatus, debugLog)
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

	inst, _, err := s.getInstance(ctx, instID)
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

	inst, _, err := s.getInstance(ctx, instID)
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
		if _, e := s.ResumeInstance(ctx, "", instID, status.Debug); err == nil {
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

	inst, oldProg, err := s.getInstance(ctx, instID)
	if err != nil {
		return
	}

	newImage, buffers, err := inst.snapshot(oldProg)
	if err != nil {
		return
	}
	defer func() {
		closeProgramImage(&newImage)
	}()

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
	defer func() {
		s.unrefProgram(&newProg)
	}()

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

	inst, defaultProg, err := s.getInstance(ctx, instID)
	if err != nil {
		return
	}

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

		if acc := s.accounts[inprincipal.Raw(pri)]; acc != nil {
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
	acc = s.accounts[inprincipal.Raw(pri)]
	if acc == nil {
		acc = newAccount(pri)
		s.accounts[inprincipal.Raw(pri)] = acc
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
		// to be called.
		s.ensureAccount(lock, pri).ensureRefProgram(lock, prog)
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
	defer func() {
		s.unrefProgram(&prog)
	}()

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
	defer func() {
		s.unrefProgram(&prog)
	}()

	if event, err := inst.drive(ctx, prog, function); event != nil {
		s.Monitor(event, err)
	}

	if inst.Transient() {
		if inst.annihilate() == nil {
			s.deleteNonexistentInstance(inst)
		}
	}
}

// getInstance and a borrowed program reference which is valid until the
// instance is deleted.
func (s *Server) getInstance(ctx context.Context, instID string,
) (inst *Instance, prog *program, err error) {
	pri := principal.ContextID(ctx)
	if pri == nil {
		err = errAnonymous
		return
	}

	lock := s.mu.Lock()
	defer s.mu.Unlock()

	return s.getInstance_(lock, pri, instID)
}

func (s *Server) getInstanceEnsureProgramStorage(ctx context.Context, instID string,
) (inst *Instance, err error) {
	pri := principal.ContextID(ctx)
	if pri == nil {
		err = errAnonymous
		return
	}

	lock := s.mu.Lock()
	defer s.mu.Unlock()

	inst, prog, err := s.getInstance_(lock, pri, instID)
	if err != nil {
		return
	}

	err = prog.ensureStorage()
	return
}

func (s *Server) getInstance_(_ serverLock, pri *principal.ID, instID string,
) (inst *Instance, prog *program, err error) {
	acc := s.accounts[inprincipal.Raw(pri)]
	if acc == nil {
		err = resourcenotfound.ErrInstance
		return
	}

	x, found := acc.instances[instID]
	if !found {
		err = resourcenotfound.ErrInstance
		return
	}

	inst = x.inst
	prog = x.prog
	return
}

func (s *Server) allocateInstanceResources(ctx context.Context, pol *InstancePolicy, debugOption string,
) (proc *runtime.Process, services InstanceServices, debugStatus string, debugLog io.WriteCloser, err error) {
	defer func() {
		if err != nil {
			closeInstanceResources(&proc, &services, &debugLog)
		}
	}()

	if debugOption != "" {
		if pol.Debug == nil {
			err = AccessForbidden("no debug policy")
			return
		}
		debugStatus, debugLog, err = pol.Debug(ctx, debugOption)
		if err != nil {
			return
		}
	}

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
func (s *Server) registerProgramRefInstance(ctx context.Context, acc *account, ref bool, prog *program, instImage *image.Instance, pol *InstancePolicy, transient bool, instID, debug string, suspend bool,
) (inst *Instance, canonicalProg *program, redundantProg bool, err error) {
	var (
		proc        *runtime.Process
		services    InstanceServices
		debugStatus string
		debugLog    io.WriteCloser
	)
	if !suspend && !instImage.Final() {
		proc, services, debugStatus, debugLog, err = s.allocateInstanceResources(ctx, pol, debug)
		if err != nil {
			return
		}
		defer func() {
			closeInstanceResources(&proc, &services, &debugLog)
		}()
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

	var persistent *snapshot.Buffers
	if !transient {
		clone := prog.buffers
		persistent = &clone
	}

	inst = newInstance(instID, acc, instImage, persistent, proc, services, pol.TimeResolution, debugStatus, debugLog)
	proc = nil
	services = nil
	debugLog = nil

	if acc != nil {
		if ref {
			acc.ensureRefProgram(lock, prog)
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

// mergeProgramRef returns a borrowed program reference which valid until the
// server mutex is unlocked.
func (s *Server) mergeProgramRef(lock serverLock, prog *program) (canonical *program, redundant bool, err error) {
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

func closeInstanceResources(proc **runtime.Process, services *InstanceServices, w *io.WriteCloser) {
	if *proc != nil {
		(*proc).Close()
		*proc = nil
	}
	if *services != nil {
		(*services).Close()
		*services = nil
	}
	if *w != nil {
		(*w).Close()
		*w = nil
	}
}
