// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/tsavola/gate/image"
	"github.com/tsavola/gate/internal/error/resourcelimit"
	inprincipal "github.com/tsavola/gate/internal/principal"
	"github.com/tsavola/gate/principal"
	"github.com/tsavola/gate/runtime"
	"github.com/tsavola/gate/server/detail"
	"github.com/tsavola/gate/server/event"
	"github.com/tsavola/gate/server/internal/error/resourcenotfound"
	"github.com/tsavola/gate/snapshot"
)

type principalKeyArray [32]byte

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

	mu       serverMutex
	accounts map[principalKeyArray]*account
	programs map[string]*program
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
		Config:   config,
		accounts: make(map[principalKeyArray]*account),
		programs: make(map[string]*program),
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
		s.accounts[inprincipal.RawKey(s.XXX_Owner)] = owner
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
	var is []*Instance

	func() {
		lock := s.mu.Lock()
		defer s.mu.Unlock()

		as := s.accounts
		s.accounts = nil

		for _, acc := range as {
			for _, x := range acc.cleanup(lock) {
				is = append(is, x.inst)
				x.prog.unref(lock)
			}
		}

		ps := s.programs
		s.programs = nil

		for _, prog := range ps {
			prog.unref(lock)
		}
	}()

	for _, inst := range is {
		inst.suspend()
	}

	var aborted bool

	for _, inst := range is {
		if inst.Wait(ctx).State == StateRunning {
			aborted = true
		}
	}

	if aborted {
		err = ctx.Err()
	}
	return
}

// UploadModule creates a new module reference if refModule is true.  Caller
// provides module content which is compiled or validated in any case.
func (s *Server) UploadModule(ctx context.Context, pri *principal.Key, refModule bool, allegedHash string, content io.ReadCloser, contentLength int64,
) (err error) {
	defer func() {
		closeReader(&content)
	}()

	if pri == nil && refModule {
		panic("referencing module without principal")
	}

	var pol progPolicy

	err = s.AccessPolicy.AuthorizeProgram(ctx, pri, &pol.res, &pol.prog)
	if err != nil {
		return
	}

	if contentLength > int64(pol.prog.MaxModuleSize) {
		err = resourcelimit.New("module size limit exceeded")
		return
	}

	var acc *account
	if refModule {
		acc, err = s.ensureAccount(pri)
		if err != nil {
			return
		}
	}

	// TODO: check resource policy

	found, err := s.loadKnownModule(ctx, acc, &pol, allegedHash, &content, contentLength)
	if found || err != nil {
		return
	}

	_, err = s.loadUnknownModule(ctx, acc, &pol, allegedHash, content, int(contentLength))
	content = nil
	return
}

// SourceModule creates a new module reference if refModule is true.  Module
// content is read from a source - it is compiled or validated in any case.
func (s *Server) SourceModule(ctx context.Context, pri *principal.Key, refModule bool, source Source, uri string,
) (progHash string, err error) {
	if pri == nil && refModule {
		panic("referencing module without principal")
	}

	var pol progPolicy

	err = s.AccessPolicy.AuthorizeProgramSource(ctx, pri, &pol.res, &pol.prog, source)
	if err != nil {
		return
	}

	var acc *account
	if refModule {
		acc, err = s.ensureAccount(pri)
		if err != nil {
			return
		}
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

	return s.loadUnknownModule(ctx, acc, &pol, "", content, int(size))
}

// loadKnownModule might close the content reader and set it to nil.
func (s *Server) loadKnownModule(ctx context.Context, acc *account, pol *progPolicy, allegedHash string, content *io.ReadCloser, contentLength int64,
) (found bool, err error) {
	prog, err := s.refProgram(ctx, allegedHash, contentLength)
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

	if acc != nil {
		err = prog.ensureStorage()
		if err != nil {
			return
		}
	}

	_, err = s.registerProgramRef(acc, prog)
	if err != nil {
		return
	}
	prog = nil

	s.monitor(&event.ModuleUploadExist{
		Ctx:    accountContext(ctx, acc),
		Module: progHash,
	})
	return
}

// loadUnknownModule always closes the content reader.
func (s *Server) loadUnknownModule(ctx context.Context, acc *account, pol *progPolicy, allegedHash string, content io.ReadCloser, contentSize int,
) (progHash string, err error) {
	prog, _, err := buildProgram(s.ImageStorage, &pol.prog, nil, allegedHash, content, contentSize, "")
	if err != nil {
		return
	}
	defer func() {
		s.unrefProgram(&prog)
	}()
	progHash = prog.hash

	if acc != nil {
		err = prog.ensureStorage()
		if err != nil {
			return
		}
	}

	redundant, err := s.registerProgramRef(acc, prog)
	if err != nil {
		return
	}
	prog = nil

	if redundant {
		s.monitor(&event.ModuleUploadExist{
			Ctx:      accountContext(ctx, acc),
			Module:   progHash,
			Compiled: true,
		})
	} else {
		s.monitor(&event.ModuleUploadNew{
			Ctx:    accountContext(ctx, acc),
			Module: progHash,
		})
	}
	return
}

// CreateInstance instantiates a module reference.  Instance id is optional.
func (s *Server) CreateInstance(ctx context.Context, pri *principal.Key, progHash string, persistInst bool, function string, instID, debug string,
) (inst *Instance, err error) {
	var pol instPolicy

	err = s.AccessPolicy.AuthorizeInstance(ctx, pri, &pol.res, &pol.inst)
	if err != nil {
		return
	}

	acc, err := s.checkInstanceIDAndEnsureAccount(pri, instID)
	if err != nil {
		return
	}

	prog := s.refAccountProgram(acc, progHash)
	if prog == nil {
		err = resourcenotfound.ErrModule
		return
	}
	defer func() {
		s.unrefProgram(&prog)
	}()
	progHash = prog.hash // Canonical string.

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

	inst, prog, _, err = s.registerProgramRefInstance(ctx, acc, false, prog, instImage, &pol.inst, persistInst, function, instID, debug)
	if err != nil {
		return
	}
	instImage = nil

	err = s.runOrDeleteInstance(ctx, inst, prog)
	if err != nil {
		return
	}
	prog = nil

	s.monitor(&event.InstanceCreateLocal{
		Ctx:      Context(ctx, pri),
		Instance: inst.ID,
		Module:   progHash,
	})
	return
}

// UploadModuleInstance creates a new module reference if refModule is true.
// The module is instantiated in any case.  Caller provides module content.
// Instance id is optional.
func (s *Server) UploadModuleInstance(ctx context.Context, pri *principal.Key, refModule bool, allegedHash string, content io.ReadCloser, contentLength int64, persistInst bool, function, instID, debug string,
) (inst *Instance, err error) {
	defer func() {
		closeReader(&content)
	}()

	if pri == nil && refModule {
		panic("referencing module without principal")
	}

	var pol instProgPolicy

	err = s.AccessPolicy.AuthorizeProgramInstance(ctx, pri, &pol.res, &pol.prog, &pol.inst)
	if err != nil {
		return
	}

	acc, err := s.checkInstanceIDAndEnsureAccount(pri, instID)
	if err != nil {
		return
	}

	_, inst, err = s.loadModuleInstance(ctx, acc, refModule, &pol, allegedHash, content, contentLength, persistInst, function, instID, debug)
	content = nil
	return
}

// SourceModuleInstance creates a new module reference if refModule is true.
// The module is instantiated in any case.  Module content is read from a
// source.  Instance id is optional.
func (s *Server) SourceModuleInstance(ctx context.Context, pri *principal.Key, refModule bool, source Source, uri string, persistInst bool, function, instID, debug string,
) (progHash string, inst *Instance, err error) {
	if pri == nil && refModule {
		panic("referencing module without principal")
	}

	var pol instProgPolicy

	err = s.AccessPolicy.AuthorizeProgramInstanceSource(ctx, pri, &pol.res, &pol.prog, &pol.inst, source)
	if err != nil {
		return
	}

	acc, err := s.checkInstanceIDAndEnsureAccount(pri, instID)
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

	progHash, inst, err = s.loadModuleInstance(ctx, acc, refModule, &pol, "", content, int64(size), persistInst, function, instID, debug)
	return
}

func (s *Server) loadModuleInstance(ctx context.Context, acc *account, refModule bool, pol *instProgPolicy, allegedHash string, content io.ReadCloser, contentLength int64, persistInst bool, function, instID, debug string,
) (progHash string, inst *Instance, err error) {
	defer func() {
		closeReader(&content)
	}()

	if contentLength > int64(pol.prog.MaxModuleSize) {
		err = resourcelimit.New("module size limit exceeded")
		return
	}

	// TODO: check resource policy

	if allegedHash != "" {
		inst, err = s.loadKnownModuleInstance(ctx, acc, refModule, pol, allegedHash, &content, contentLength, persistInst, function, instID, debug)
		if err != nil {
			return
		}
		if inst != nil {
			progHash = allegedHash
			return
		}
	}

	progHash, inst, err = s.loadUnknownModuleInstance(ctx, acc, refModule, pol, allegedHash, content, int(contentLength), persistInst, function, instID, debug)
	content = nil
	return
}

// loadKnownModuleInstance might close the content reader and set it to nil.
func (s *Server) loadKnownModuleInstance(ctx context.Context, acc *account, refModule bool, pol *instProgPolicy, allegedHash string, content *io.ReadCloser, contentLength int64, persistInst bool, function, instID, debug string,
) (inst *Instance, err error) {
	prog, err := s.refProgram(ctx, allegedHash, contentLength)
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

	err = prog.ensureStorage()
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

	inst, prog, _, err = s.registerProgramRefInstance(ctx, acc, refModule, prog, instImage, &pol.inst, persistInst, function, instID, debug)
	if err != nil {
		return
	}
	instImage = nil

	s.monitor(&event.ModuleUploadExist{
		Ctx:    accountContext(ctx, acc),
		Module: progHash,
	})

	err = s.runOrDeleteInstance(ctx, inst, prog)
	if err != nil {
		return
	}
	prog = nil

	s.monitor(&event.InstanceCreateLocal{
		Ctx:      accountContext(ctx, acc),
		Instance: inst.ID,
		Module:   progHash,
	})
	return
}

// loadUnknownModuleInstance always closes the content reader.
func (s *Server) loadUnknownModuleInstance(ctx context.Context, acc *account, refModule bool, pol *instProgPolicy, allegedHash string, content io.ReadCloser, contentSize int, persistInst bool, function, instID, debug string,
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

	err = prog.ensureStorage()
	if err != nil {
		return
	}

	inst, prog, redundantProg, err := s.registerProgramRefInstance(ctx, acc, refModule, prog, instImage, &pol.inst, persistInst, function, instID, debug)
	if err != nil {
		return
	}
	instImage = nil

	if allegedHash != "" {
		if redundantProg {
			s.monitor(&event.ModuleUploadExist{
				Ctx:      accountContext(ctx, acc),
				Module:   progHash,
				Compiled: true,
			})
		} else {
			s.monitor(&event.ModuleUploadNew{
				Ctx:    accountContext(ctx, acc),
				Module: progHash,
			})
		}
	} else {
		if redundantProg {
			s.monitor(&event.ModuleSourceExist{
				Ctx:    accountContext(ctx, acc),
				Module: progHash,
				// TODO: source URI
				Compiled: true,
			})
		} else {
			s.monitor(&event.ModuleSourceNew{
				Ctx:    accountContext(ctx, acc),
				Module: progHash,
				// TODO: source URI
			})
		}
	}

	err = s.runOrDeleteInstance(ctx, inst, prog)
	if err != nil {
		return
	}
	prog = nil

	s.monitor(&event.InstanceCreateStream{
		Ctx:      accountContext(ctx, acc),
		Instance: inst.ID,
		Module:   progHash,
	})
	return
}

func (s *Server) ModuleRefs(ctx context.Context, pri *principal.Key) (refs ModuleRefs, err error) {
	err = s.AccessPolicy.Authorize(ctx, pri)
	if err != nil {
		return
	}

	s.monitor(&event.ModuleList{
		Ctx: Context(ctx, pri),
	})

	s.mu.Lock()
	defer s.mu.Unlock()

	acc := s.accounts[inprincipal.RawKey(pri)]
	if acc == nil {
		return
	}

	refs = make(ModuleRefs, 0, len(acc.programRefs))
	for prog := range acc.programRefs {
		refs = append(refs, ModuleRef{
			Id: prog.hash,
		})
	}

	return
}

// ModuleContent for downloading.
func (s *Server) ModuleContent(ctx context.Context, pri *principal.Key, hash string,
) (content io.ReadCloser, length int64, err error) {
	err = s.AccessPolicy.Authorize(ctx, pri)
	if err != nil {
		return
	}

	prog := s.refPrincipalProgram(pri, hash)
	if prog == nil {
		err = resourcenotfound.ErrModule
		return
	}

	length = prog.image.ModuleSize()
	content = &moduleContent{
		ctx:   Context(ctx, pri),
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

func (s *Server) UnrefModule(ctx context.Context, pri *principal.Key, hash string) (err error) {
	err = s.AccessPolicy.Authorize(ctx, pri)
	if err != nil {
		return
	}

	err = func() (err error) {
		lock := s.mu.Lock()
		defer s.mu.Unlock()

		acc, prog := s.getAccountAndPrincipalProgram(lock, pri, hash)
		if prog == nil {
			err = resourcenotfound.ErrModule
			return
		}

		acc.unrefProgram(lock, prog)
		if prog.refCount == 1 {
			if _, ok := s.programs[prog.hash]; !ok {
				panic(fmt.Sprintf("account program %q reference is unknown to server", prog.hash))
			}
			delete(s.programs, prog.hash)
			prog.unref(lock)
		}

		return
	}()
	if err != nil {
		return
	}

	s.monitor(&event.ModuleUnref{
		Ctx:    Context(ctx, pri),
		Module: hash,
	})
	return
}

func (s *Server) InstanceConnection(ctx context.Context, pri *principal.Key, instID string,
) (inst *Instance, connIO func(context.Context, io.Reader, io.Writer) error, err error) {
	err = s.AccessPolicy.Authorize(ctx, pri)
	if err != nil {
		return
	}

	inst, _ = s.getInstance(pri, instID)
	if inst == nil {
		err = resourcenotfound.ErrInstance
		return
	}

	conn := inst.connect(ctx)
	if conn == nil {
		s.monitor(&event.FailRequest{
			Ctx:      Context(ctx, pri),
			Failure:  event.FailInstanceNoConnect,
			Instance: inst.ID,
		})
		return
	}

	connIO = func(ctx context.Context, r io.Reader, w io.Writer) (err error) {
		s.monitor(&event.InstanceConnect{
			Ctx:      Context(ctx, pri),
			Instance: inst.ID,
		})

		err = conn(ctx, r, w)

		s.Monitor(&event.InstanceDisconnect{
			Ctx:      Context(ctx, pri),
			Instance: inst.ID,
		}, err)
		return
	}
	return
}

// InstanceStatus of an existing instance.
func (s *Server) InstanceStatus(ctx context.Context, pri *principal.Key, instID string,
) (status Status, err error) {
	err = s.AccessPolicy.Authorize(ctx, pri)
	if err != nil {
		return
	}

	inst, _ := s.getInstance(pri, instID)
	if inst == nil {
		err = resourcenotfound.ErrInstance
		return
	}

	s.monitor(&event.InstanceStatus{
		Ctx:      Context(ctx, pri),
		Instance: inst.ID,
	})

	status = inst.Status()
	return
}

func (s *Server) WaitInstance(ctx context.Context, pri *principal.Key, instID string,
) (status Status, err error) {
	err = s.AccessPolicy.Authorize(ctx, pri)
	if err != nil {
		return
	}

	inst, _ := s.getInstance(pri, instID)
	if inst == nil {
		err = resourcenotfound.ErrInstance
		return
	}

	s.monitor(&event.InstanceWait{
		Ctx:      Context(ctx, pri),
		Instance: inst.ID,
	})

	status = inst.Wait(ctx)
	return
}

func (s *Server) KillInstance(ctx context.Context, pri *principal.Key, instID string,
) (inst *Instance, err error) {
	err = s.AccessPolicy.Authorize(ctx, pri)
	if err != nil {
		return
	}

	inst, _ = s.getInstance(pri, instID)
	if inst == nil {
		err = resourcenotfound.ErrInstance
		return
	}

	s.monitor(&event.InstanceKill{
		Ctx:      Context(ctx, pri),
		Instance: inst.ID,
	})

	inst.Kill()
	return
}

func (s *Server) SuspendInstance(ctx context.Context, pri *principal.Key, instID string,
) (inst *Instance, err error) {
	err = s.AccessPolicy.Authorize(ctx, pri)
	if err != nil {
		return
	}

	inst, _ = s.getInstance(pri, instID)
	if inst == nil {
		err = resourcenotfound.ErrInstance
		return
	}

	s.monitor(&event.InstanceSuspend{
		Ctx:      Context(ctx, pri),
		Instance: inst.ID,
	})

	inst.suspend()
	return
}

func (s *Server) ResumeInstance(ctx context.Context, pri *principal.Key, function, instID, debug string,
) (inst *Instance, err error) {
	var pol instPolicy

	err = s.AccessPolicy.AuthorizeInstance(ctx, pri, &pol.res, &pol.inst)
	if err != nil {
		return
	}

	inst, prog := s.getInstance(pri, instID)
	if inst == nil {
		err = resourcenotfound.ErrInstance
		return
	}

	err = inst.checkResume(prog, function)
	if err != nil {
		return
	}

	proc, services, debugStatus, debugOutput, err := s.allocateInstanceResources(ctx, pri, &pol.inst, debug)
	if err != nil {
		return
	}
	defer func() {
		closeInstanceResources(&proc, &services, &debugOutput)
	}()

	err = inst.doResume(prog, function, proc, services, pol.inst.TimeResolution, debugStatus, debugOutput)
	if err != nil {
		return
	}
	proc = nil
	services = nil
	debugOutput = nil

	err = s.runOrDeleteInstance(ctx, inst, prog)
	if err != nil {
		return
	}
	prog = nil

	s.monitor(&event.InstanceResume{
		Ctx:      Context(ctx, pri),
		Instance: inst.ID,
		Function: function,
	})
	return
}

func (s *Server) DeleteInstance(ctx context.Context, pri *principal.Key, instID string,
) (err error) {
	err = s.AccessPolicy.Authorize(ctx, pri)
	if err != nil {
		return
	}

	inst, _ := s.getInstance(pri, instID)
	if inst == nil {
		err = resourcenotfound.ErrInstance
		return
	}

	err = inst.annihilate()
	if err != nil {
		return
	}

	s.monitor(&event.InstanceDelete{
		Ctx:      Context(ctx, pri),
		Instance: inst.ID,
	})

	s.deleteNonexistentInstance(inst)
	return
}

func (s *Server) InstanceModule(ctx context.Context, pri *principal.Key, instID string,
) (moduleKey string, err error) {
	// TODO: implement suspend-snapshot-resume at a lower level

	inst, _ := s.getInstance(pri, instID)
	if inst == nil {
		err = resourcenotfound.ErrInstance
		return
	}

	status := inst.Status()
	resume := false
	if status.State == StateRunning {
		inst.suspend()
		resume = inst.Wait(context.Background()).State == StateSuspended
	}

	moduleKey, err = s.snapshot(ctx, pri, instID)

	if resume {
		if _, e := s.ResumeInstance(ctx, pri, "", instID, status.Debug); err == nil {
			err = e
		}
	}

	return
}

func (s *Server) snapshot(ctx context.Context, pri *principal.Key, instID string,
) (moduleKey string, err error) {
	err = s.AccessPolicy.Authorize(ctx, pri)
	if err != nil {
		return
	}

	// TODO: check module storage limits

	inst, oldProg := s.getInstance(pri, instID)
	if inst == nil {
		err = resourcenotfound.ErrInstance
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

	_, err = s.registerProgramRef(inst.acc, newProg)
	if err != nil {
		return
	}
	newProg = nil

	s.monitor(&event.InstanceSnapshot{
		Ctx:      Context(ctx, pri),
		Instance: inst.ID,
		Module:   moduleKey,
	})
	return
}

func (s *Server) Instances(ctx context.Context, pri *principal.Key) (statuses Instances, err error) {
	err = s.AccessPolicy.Authorize(ctx, pri)
	if err != nil {
		return
	}

	s.monitor(&event.InstanceList{
		Ctx: Context(ctx, pri),
	})

	// Get instance references while holding server lock.
	is := func() (is []*Instance) {
		s.mu.Lock()
		defer s.mu.Unlock()

		if acc := s.accounts[inprincipal.RawKey(pri)]; acc != nil {
			is = make([]*Instance, 0, len(acc.instances))
			for _, x := range acc.instances {
				is = append(is, x.inst)
			}
		}

		return
	}()

	// Get instance statuses.  Each instance has its own lock.
	statuses = make(Instances, 0, len(is))
	for _, inst := range is {
		statuses = append(statuses, InstanceStatus{
			Instance:  inst.ID,
			Status:    inst.Status(),
			Transient: inst.transient,
		})
	}

	return
}

func (s *Server) ensureAccount(pri *principal.Key) (acc *account, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.accounts == nil {
		err = context.Canceled
		return
	}

	acc = s.accounts[inprincipal.RawKey(pri)]
	if acc == nil {
		acc = newAccount(pri)
		s.accounts[inprincipal.RawKey(pri)] = acc
	}

	return
}

func (s *Server) refProgram(ctx context.Context, hash string, length int64) (*program, error) {
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

func (s *Server) refAccountProgram(acc *account, hash string) *program {
	lock := s.mu.Lock()
	defer s.mu.Unlock()

	if prog := s.programs[hash]; prog != nil {
		if _, own := acc.programRefs[prog]; own {
			return prog.ref(lock)
		}
	}

	return nil
}

func (s *Server) refPrincipalProgram(pri *principal.Key, hash string) *program {
	lock := s.mu.Lock()
	defer s.mu.Unlock()

	if acc := s.accounts[inprincipal.RawKey(pri)]; acc != nil {
		if prog := s.programs[hash]; prog != nil {
			if _, own := acc.programRefs[prog]; own {
				return prog.ref(lock)
			}
		}
	}

	return nil
}

func (s *Server) getAccountAndPrincipalProgram(_ serverLock, pri *principal.Key, hash string,
) (*account, *program) {
	acc := s.accounts[inprincipal.RawKey(pri)]
	if acc != nil {
		if prog := s.programs[hash]; prog != nil {
			if _, own := acc.programRefs[prog]; own {
				return acc, prog
			}
		}
	}

	return acc, nil
}

// registerProgramRef with the server and an account.  Caller's program
// reference is stolen (except on error).
func (s *Server) registerProgramRef(acc *account, prog *program) (redundant bool, err error) {
	lock := s.mu.Lock()
	defer s.mu.Unlock()

	prog, redundant, err = s.mergeProgramRef(lock, prog)
	if err != nil {
		return
	}

	if acc != nil {
		acc.ensureRefProgram(lock, prog)
	}

	return
}

// checkInstanceIDAndEnsureAccount if pri is non-nil.
func (s *Server) checkInstanceIDAndEnsureAccount(pri *principal.Key, instID string,
) (acc *account, err error) {
	if instID != "" {
		err = validateInstanceID(instID)
		if err != nil {
			return
		}
	}

	if pri != nil {
		lock := s.mu.Lock()
		defer s.mu.Unlock()

		if s.accounts == nil {
			err = context.Canceled
			return
		}

		acc = s.accounts[inprincipal.RawKey(pri)]
		if acc == nil {
			acc = newAccount(pri)
			s.accounts[inprincipal.RawKey(pri)] = acc
		}

		if instID != "" {
			err = acc.checkUniqueInstanceID(lock, instID)
			if err != nil {
				return
			}
		}
	}

	return
}

// runOrDeleteInstance steals the program reference (except on error).
func (s *Server) runOrDeleteInstance(ctx context.Context, inst *Instance, prog *program) error {
	defer func() {
		s.unrefProgram(&prog)
	}()

	if err := inst.startOrAnnihilate(prog); err != nil {
		s.deleteNonexistentInstance(inst)
		return err
	}

	var pri *principal.Key
	if inst.acc != nil {
		pri = inst.acc.Key
	}

	go s.driveInstance(detachedContext(ctx, pri), inst, prog)
	prog = nil

	return nil
}

// driveInstance steals the program reference.
func (s *Server) driveInstance(ctx context.Context, inst *Instance, prog *program) {
	defer func() {
		s.unrefProgram(&prog)
	}()

	if event, err := inst.drive(ctx, prog); event != nil {
		s.Monitor(event, err)
	}

	if inst.transient {
		if inst.annihilate() == nil {
			s.deleteNonexistentInstance(inst)
		}
	}
}

// getInstance and a program reference which is valid until the instance
// deleted.
func (s *Server) getInstance(pri *principal.Key, instID string) (*Instance, *program) {
	s.mu.Lock()
	defer s.mu.Unlock()

	acc := s.accounts[inprincipal.RawKey(pri)]
	if acc == nil {
		return nil, nil
	}

	x := acc.instances[instID]
	return x.inst, x.prog
}

func (s *Server) allocateInstanceResources(ctx context.Context, pri *principal.Key, pol *InstancePolicy, debugOption string,
) (proc *runtime.Process, services InstanceServices, debugStatus string, debugOutput io.WriteCloser, err error) {
	defer func() {
		if err != nil {
			closeInstanceResources(&proc, &services, &debugOutput)
		}
	}()

	ctx = inprincipal.ContextWithIDFrom(ctx, pri)

	if debugOption != "" {
		if pol.Debug == nil {
			err = AccessForbidden("no debug policy")
			return
		}
		debugStatus, debugOutput, err = pol.Debug(ctx, debugOption)
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

// registerProgramRefInstance with server, and an account if refModule is true.
// Caller's instance image is stolen (except on error).  Caller's program
// reference is replaced with a reference to the canonical program object.
func (s *Server) registerProgramRefInstance(ctx context.Context, acc *account, refModule bool, prog *program, instImage *image.Instance, pol *InstancePolicy, persistInst bool, function, instID, debug string,
) (inst *Instance, canonicalProg *program, redundantProg bool, err error) {
	var pri *principal.Key
	if acc != nil {
		pri = acc.Key
	}

	proc, services, debugStatus, debugOutput, err := s.allocateInstanceResources(ctx, pri, pol, debug)
	if err != nil {
		return
	}
	defer func() {
		closeInstanceResources(&proc, &services, &debugOutput)
	}()

	if instID == "" {
		instID = makeInstanceID()
	}

	lock := s.mu.Lock()
	defer s.mu.Unlock()

	if acc != nil {
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
	if persistInst {
		clone := prog.buffers
		persistent = &clone
	}

	inst = newInstance(instID, acc, function, instImage, persistent, proc, services, pol.TimeResolution, debugStatus, debugOutput)
	proc = nil
	services = nil
	debugOutput = nil

	if acc != nil {
		if refModule {
			acc.ensureRefProgram(lock, prog)
		}
		acc.instances[instID] = accountInstance{inst, prog.ref(lock)}
	}

	canonicalProg = prog.ref(lock)
	return
}

func (s *Server) deleteNonexistentInstance(inst *Instance) {
	if inst.acc == nil {
		return
	}

	lock := s.mu.Lock()
	defer s.mu.Unlock()

	if x := inst.acc.instances[inst.ID]; x.inst == inst {
		delete(inst.acc.instances, inst.ID)
		x.prog.unref(lock)
	}
}

// mergeProgramRef returns a borrowed program reference which valid until the
// server mutex is unlocked.
func (s *Server) mergeProgramRef(lock serverLock, prog *program) (canonical *program, redundant bool, err error) {
	switch existing := s.programs[prog.hash]; existing {
	case nil:
		if s.programs == nil {
			return nil, false, context.Canceled
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
