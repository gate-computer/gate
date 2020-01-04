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
	"github.com/tsavola/gate/server/event"
	"github.com/tsavola/gate/server/internal/error/failrequest"
	"github.com/tsavola/gate/server/internal/error/notapplicable"
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

type Server struct {
	Config

	lock     sync.Mutex
	accounts map[principalKeyArray]*account
	programs map[string]*program
}

func New(config Config) (s *Server, err error) {
	if config.ImageStorage == nil {
		config.ImageStorage = image.Memory
	}
	if config.Monitor == nil {
		config.Monitor = defaultMonitor
	}
	if !config.Configured() {
		panic("incomplete server configuration")
	}

	s = &Server{
		Config:   config,
		accounts: make(map[principalKeyArray]*account),
		programs: make(map[string]*program),
	}
	defer func() {
		if err != nil {
			s.Shutdown(context.TODO())
		}
	}()

	err = s.initPrograms()
	if err != nil {
		return
	}

	return
}

func (s *Server) initPrograms() error {
	list, err := s.ImageStorage.Programs()
	if err != nil {
		return err
	}

	for _, hash := range list {
		if err := s.loadProgramDuringInit(hash); err != nil {
			return err
		}
	}

	return nil
}

func (s *Server) loadProgramDuringInit(hash string) error {
	image, err := s.ImageStorage.LoadProgram(hash)
	if err != nil {
		return err
	}
	if image == nil { // Race condition with human?
		return nil
	}
	defer func() {
		if image != nil {
			image.Close()
		}
	}()

	buffers, err := image.LoadBuffers()
	if err != nil {
		return err
	}

	prog := newProgram(hash, image, buffers, true)
	s.programs[hash] = prog
	image = nil

	if s.XXX_Owner != nil {
		acc, _ := s.ensureAccount(s.XXX_Owner)
		acc.ensureRefProgram(prog)
	}

	return nil
}

func (s *Server) Shutdown(ctx context.Context) (err error) {
	var is []*Instance

	func() {
		s.lock.Lock()
		defer s.lock.Unlock()

		ps := s.programs
		s.programs = nil

		for _, prog := range ps {
			prog.unref()
		}

		as := s.accounts
		s.accounts = nil

		for _, acc := range as {
			for _, inst := range acc.cleanup() {
				is = append(is, inst)
			}
		}
	}()

	for _, inst := range is {
		wait := func() <-chan struct{} {
			inst.lock.Lock()
			defer inst.lock.Unlock()

			if inst.status.State == StateRunning {
				return inst.stopped
			} else {
				return nil
			}
		}()

		if wait != nil {
			<-wait
		}

		inst.Kill(s)
	}

	return
}

// UploadModule creates a new module reference if refModule is true.  Caller
// provides module content which is compiled or validated in any case.
func (s *Server) UploadModule(ctx context.Context, pri *principal.Key, refModule bool, allegedHash string, content io.ReadCloser, contentLength int64,
) (err error) {
	defer func() {
		if content != nil {
			content.Close()
		}
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

	found, err := s.loadKnownModule(ctx, acc, &pol, allegedHash, content, contentLength)
	if err != nil {
		return
	}

	if found {
		err = content.Close()
		content = nil
		if err != nil {
			err = wrapContentError(err)
			return
		}
	} else {
		_, err = s.loadUnknownModule(ctx, acc, &pol, allegedHash, content, int(contentLength))
		if err != nil {
			return
		}
	}
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

	progHash, err = s.loadUnknownModule(ctx, acc, &pol, "", content, int(size))
	if err != nil {
		return
	}

	return
}

func (s *Server) loadKnownModule(ctx context.Context, acc *account, pol *progPolicy, allegedHash string, content io.Reader, contentLength int64,
) (found bool, err error) {
	prog, err := s.refProgram(ctx, allegedHash, contentLength)
	if err != nil || prog == nil {
		return
	}
	defer func() {
		if err != nil {
			s.unrefProgram(prog)
		}
	}()

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

	s.Monitor(&event.ModuleUploadExist{
		Ctx:    accountContext(ctx, acc),
		Module: prog.key,
	}, nil)
	return
}

func (s *Server) loadUnknownModule(ctx context.Context, acc *account, pol *progPolicy, allegedHash string, content io.ReadCloser, contentSize int,
) (progHash string, err error) {
	prog, _, err := buildProgram(s.ImageStorage, &pol.prog, nil, allegedHash, content, contentSize, "")
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			prog.unref()
		}
	}()

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

	if redundant {
		s.Monitor(&event.ModuleUploadExist{
			Ctx:      accountContext(ctx, acc),
			Module:   prog.key,
			Compiled: true,
		}, nil)
	} else {
		s.Monitor(&event.ModuleUploadNew{
			Ctx:    accountContext(ctx, acc),
			Module: prog.key,
		}, nil)
	}

	progHash = prog.key
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
		if err != nil {
			s.unrefProgram(prog)
		}
	}()

	entryIndex, err := prog.image.ResolveEntryFunc(function)
	if err != nil {
		return
	}

	// TODO: check resource policy (text/stack/memory/max-memory size etc.)

	instImage, err := image.NewInstance(prog.image, pol.inst.MaxMemorySize, pol.inst.StackSize, entryIndex)
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			instImage.Close()
		}
	}()

	inst, _, err = s.registerProgramRefInstance(ctx, acc, false, prog, instImage, &pol.inst, persistInst, function, instID, debug)
	if err != nil {
		return
	}

	s.Monitor(&event.InstanceCreateLocal{
		Ctx:      Context(ctx, pri),
		Instance: inst.id,
		Module:   prog.key,
	}, nil)
	return
}

// UploadModuleInstance creates a new module reference if refModule is true.
// The module is instantiated in any case.  Caller provides module content.
// Instance id is optional.
func (s *Server) UploadModuleInstance(ctx context.Context, pri *principal.Key, refModule bool, allegedHash string, content io.ReadCloser, contentLength int64, persistInst bool, function, instID, debug string,
) (inst *Instance, err error) {
	defer func() {
		if content != nil {
			content.Close()
		}
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

	inst, err = s.loadModuleInstance(ctx, acc, refModule, &pol, allegedHash, content, contentLength, persistInst, function, instID, debug)
	content = nil
	if err != nil {
		return
	}

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

	inst, err = s.loadModuleInstance(ctx, acc, refModule, &pol, "", content, int64(size), persistInst, function, instID, debug)
	if err != nil {
		return
	}

	progHash = inst.prog.key
	return
}

func (s *Server) loadModuleInstance(ctx context.Context, acc *account, refModule bool, pol *instProgPolicy, allegedHash string, content io.ReadCloser, contentLength int64, persistInst bool, function, instID, debug string,
) (inst *Instance, err error) {
	defer func() {
		if content != nil {
			content.Close()
		}
	}()

	if contentLength > int64(pol.prog.MaxModuleSize) {
		err = resourcelimit.New("module size limit exceeded")
		return
	}

	// TODO: check resource policy

	inst, err = s.loadKnownModuleInstance(ctx, acc, refModule, pol, allegedHash, content, contentLength, persistInst, function, instID, debug)
	if err != nil {
		return
	}

	if inst != nil {
		err = content.Close()
		content = nil
		if err != nil {
			err = wrapContentError(err)
			return
		}
	} else {
		inst, err = s.loadUnknownModuleInstance(ctx, acc, refModule, pol, allegedHash, content, int(contentLength), persistInst, function, instID, debug)
		if err != nil {
			return
		}
	}
	return
}

func (s *Server) loadKnownModuleInstance(ctx context.Context, acc *account, refModule bool, pol *instProgPolicy, allegedHash string, content io.Reader, contentLength int64, persistInst bool, function, instID, debug string,
) (inst *Instance, err error) {
	if allegedHash == "" {
		return
	}

	prog, err := s.refProgram(ctx, allegedHash, contentLength)
	if err != nil || prog == nil {
		return
	}
	defer func() {
		if err != nil {
			s.unrefProgram(prog)
		}
	}()

	err = validateHashContent(prog.key, content)
	if err != nil {
		return
	}

	if prog.image.TextSize() > pol.prog.MaxTextSize {
		err = resourcelimit.New("program code size limit exceeded")
		return
	}

	// TODO: check resource policy (stack/memory/max-memory size etc.)

	entryFunc, err := prog.image.ResolveEntryFunc(function)
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
		if err != nil {
			instImage.Close()
		}
	}()

	inst, _, err = s.registerProgramRefInstance(ctx, acc, refModule, prog, instImage, &pol.inst, persistInst, function, instID, debug)
	if err != nil {
		return
	}

	s.Monitor(&event.ModuleUploadExist{
		Ctx:    accountContext(ctx, acc),
		Module: prog.key,
	}, nil)

	s.Monitor(&event.InstanceCreateLocal{
		Ctx:      accountContext(ctx, acc),
		Instance: inst.id,
		Module:   prog.key,
	}, nil)
	return
}

func (s *Server) loadUnknownModuleInstance(ctx context.Context, acc *account, refModule bool, pol *instProgPolicy, allegedHash string, content io.ReadCloser, contentSize int, persistInst bool, function, instID, debug string,
) (inst *Instance, err error) {
	prog, instImage, err := buildProgram(s.ImageStorage, &pol.prog, &pol.inst, allegedHash, content, contentSize, function)
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			instImage.Close()
			prog.unref()
		}
	}()

	err = prog.ensureStorage()
	if err != nil {
		return
	}

	inst, redundant, err := s.registerProgramRefInstance(ctx, acc, refModule, prog, instImage, &pol.inst, persistInst, function, instID, debug)
	if err != nil {
		return
	}

	if allegedHash != "" {
		if redundant {
			s.Monitor(&event.ModuleUploadExist{
				Ctx:      accountContext(ctx, acc),
				Module:   prog.key,
				Compiled: true,
			}, nil)
		} else {
			s.Monitor(&event.ModuleUploadNew{
				Ctx:    accountContext(ctx, acc),
				Module: prog.key,
			}, nil)
		}
	} else {
		if redundant {
			s.Monitor(&event.ModuleSourceExist{
				Ctx:    accountContext(ctx, acc),
				Module: prog.key,
				// TODO: source URI
				Compiled: true,
			}, nil)
		} else {
			s.Monitor(&event.ModuleSourceNew{
				Ctx:    accountContext(ctx, acc),
				Module: prog.key,
				// TODO: source URI
			}, nil)
		}
	}

	s.Monitor(&event.InstanceCreateStream{
		Ctx:      accountContext(ctx, acc),
		Instance: inst.id,
		Module:   prog.key,
	}, nil)
	return
}

func (s *Server) ModuleRefs(ctx context.Context, pri *principal.Key) (refs ModuleRefs, err error) {
	err = s.AccessPolicy.Authorize(ctx, pri)
	if err != nil {
		return
	}

	refs = func() ModuleRefs {
		s.lock.Lock()
		defer s.lock.Unlock()

		acc := s.accounts[inprincipal.RawKey(pri)]
		if acc == nil {
			return nil
		}

		refs := make(ModuleRefs, 0, len(acc.programRefs))
		for prog := range acc.programRefs {
			refs = append(refs, ModuleRef{
				Key: prog.key,
			})
		}

		return refs
	}()

	s.Monitor(&event.ModuleList{
		Ctx: Context(ctx, pri),
	}, nil)
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

	content = moduleContent{
		Reader: prog.image.NewModuleReader(),
		done: func() {
			defer s.unrefProgram(prog)
			s.Monitor(&event.ModuleDownload{
				Ctx:    Context(ctx, pri),
				Module: prog.key,
			}, nil)
		},
	}
	length = prog.image.ModuleSize()
	return
}

type moduleContent struct {
	io.Reader
	done func()
}

func (mc moduleContent) Close() (err error) {
	mc.done()
	return
}

func (s *Server) UnrefModule(ctx context.Context, pri *principal.Key, hash string) (err error) {
	err = s.AccessPolicy.Authorize(ctx, pri)
	if err != nil {
		return
	}

	var (
		acc  *account
		prog *program
	)

	func() {
		s.lock.Lock()
		defer s.lock.Unlock()

		acc, prog = s.getAccountAndPrincipalProgramWithCallerLock(pri, hash)
		if prog == nil {
			err = resourcenotfound.ErrModule
			return
		}

		acc.unrefProgram(prog)
		if prog.refCount == 1 {
			if _, ok := s.programs[prog.key]; !ok {
				panic(fmt.Sprintf("account program %q reference is unknown to server", prog.key))
			}
			delete(s.programs, prog.key)
			prog.unref()
		}
	}()
	if err != nil {
		return
	}

	s.Monitor(&event.ModuleUnref{
		Ctx:    Context(ctx, pri),
		Module: hash,
	}, nil)
	return
}

func (s *Server) InstanceConnection(ctx context.Context, pri *principal.Key, instID string,
) (connIO func(context.Context, io.Reader, io.Writer) (Status, error), err error) {
	err = s.AccessPolicy.Authorize(ctx, pri)
	if err != nil {
		return
	}

	inst := s.getInstance(pri, instID)
	if inst == nil {
		err = resourcenotfound.ErrInstance
		return
	}

	conn := inst.services.Connect(ctx)
	if conn == nil {
		s.Monitor(&event.FailRequest{
			Ctx:      Context(ctx, pri),
			Failure:  event.FailInstanceNoConnect,
			Instance: inst.id,
		}, nil)
		return
	}

	connIO = func(ctx context.Context, r io.Reader, w io.Writer) (status Status, err error) {
		s.Monitor(&event.InstanceConnect{
			Ctx:      Context(ctx, pri),
			Instance: inst.id,
		}, nil)

		defer func() {
			s.Monitor(&event.InstanceDisconnect{
				Ctx:      Context(ctx, pri),
				Instance: inst.id,
			}, err)
		}()

		err = conn(ctx, r, w)
		if err != nil {
			return
		}

		status = inst.Status()
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

	inst := s.getInstance(pri, instID)
	if inst == nil {
		err = resourcenotfound.ErrInstance
		return
	}

	status = inst.Status()

	s.Monitor(&event.InstanceStatus{
		Ctx:      Context(ctx, pri),
		Instance: inst.id,
	}, nil)
	return
}

func (s *Server) WaitInstance(ctx context.Context, pri *principal.Key, instID string,
) (status Status, err error) {
	err = s.AccessPolicy.Authorize(ctx, pri)
	if err != nil {
		return
	}

	inst := s.getInstance(pri, instID)
	if inst == nil {
		err = resourcenotfound.ErrInstance
		return
	}

	select {
	case <-inst.stopped:
		// ok

	case <-ctx.Done():
		err = ctx.Err()
		return
	}

	status = inst.Status()

	s.Monitor(&event.InstanceWait{
		Ctx:      Context(ctx, pri),
		Instance: inst.id,
	}, nil)
	return
}

func (s *Server) SuspendInstance(ctx context.Context, pri *principal.Key, instID string,
) (status Status, err error) {
	err = s.AccessPolicy.Authorize(ctx, pri)
	if err != nil {
		return
	}

	inst := s.getInstance(pri, instID)
	if inst == nil {
		err = resourcenotfound.ErrInstance
		return
	}
	if inst.persistent == nil {
		err = notapplicable.ErrInstanceTransient
		return
	}

	inst.suspend(s)

	s.Monitor(&event.InstanceSuspend{
		Ctx:      Context(ctx, pri),
		Instance: inst.id,
	}, nil)

	select {
	case <-inst.stopped:
		status = inst.Status()

	case <-ctx.Done():
		err = ctx.Err()
	}
	return
}

func (s *Server) ResumeInstance(ctx context.Context, pri *principal.Key, function, instID, debug string,
) (inst *Instance, err error) {
	var pol instPolicy

	err = s.AccessPolicy.AuthorizeInstance(ctx, pri, &pol.res, &pol.inst)
	if err != nil {
		return
	}

	inst = s.getInstance(pri, instID)
	if inst == nil {
		err = resourcenotfound.ErrInstance
		return
	}

	err = func() (err error) {
		inst.lock.Lock()
		defer inst.lock.Unlock()

		entryIndex := -1

		switch inst.status.State {
		case StateSuspended:
			if function != "" {
				err = failrequest.Errorf(event.FailInstanceStatus, "function specified for suspended instance")
				return
			}

		case StateHalted:
			if function == "" {
				err = failrequest.Errorf(event.FailInstanceStatus, "no function specified for halted instance")
				return
			}

			entryIndex, err = inst.prog.image.ResolveEntryFunc(function)
			if err != nil {
				return
			}

		default:
			err = failrequest.Errorf(event.FailInstanceStatus, fmt.Sprintf("instance is %s", inst.status.State))
			return
		}

		err = inst.image.CheckMutation()
		if err != nil {
			return
		}

		proc, services, debugStatus, debugOutput, err := s.allocateInstanceResources(ctx, pri, &pol.inst, debug)
		if err != nil {
			return
		}

		inst.image.SetEntry(inst.prog.image, entryIndex)
		inst.renew(function, proc, pol.inst.TimeResolution, services, debugStatus, debugOutput)
		return
	}()
	if err != nil {
		inst = nil
		return
	}

	s.Monitor(&event.InstanceResume{
		Ctx:      Context(ctx, pri),
		Instance: inst.id,
		Function: function,
	}, nil)
	return
}

func (s *Server) DeleteInstance(ctx context.Context, pri *principal.Key, instID string,
) (status Status, err error) {
	err = s.AccessPolicy.Authorize(ctx, pri)
	if err != nil {
		return
	}

	inst := s.getInstance(pri, instID)
	if inst == nil {
		err = resourcenotfound.ErrInstance
		return
	}

	switch inst.Status().State {
	case StateSuspended, StateHalted, StateTerminated:
		// ok

	default:
		err = failrequest.Errorf(event.FailInstanceStatus, "instance must be suspended, halted or terminated")
		return
	}

	func() {
		s.lock.Lock()
		defer s.lock.Unlock()

		acc := s.accounts[inprincipal.RawKey(pri)]
		if acc == nil {
			return
		}

		if acc.instances[instID] == inst {
			delete(acc.instances, instID)
		}
	}()

	inst.Kill(s)

	s.Monitor(&event.InstanceDelete{
		Ctx:      Context(ctx, pri),
		Instance: inst.id,
	}, nil)
	return
}

func (s *Server) InstanceModule(ctx context.Context, pri *principal.Key, instID string,
) (moduleKey string, err error) {
	err = s.AccessPolicy.Authorize(ctx, pri)
	if err != nil {
		return
	}

	// TODO: check module storage limits

	oldProg, inst := s.refInstanceProgram(pri, instID)
	if oldProg == nil {
		err = resourcenotfound.ErrInstance
		return
	}
	defer s.unrefProgram(oldProg)

	newImage, buffers, err := func() (newImage *image.Program, buffers snapshot.Buffers, err error) {
		inst.lock.Lock()
		defer inst.lock.Unlock()

		var suspended bool

		switch inst.status.State {
		case StateSuspended:
			suspended = true

		case StateHalted, StateTerminated:
			suspended = false

		default:
			err = failrequest.Errorf(event.FailInstanceStatus, "instance must be suspended, halted or terminated")
			return
		}

		if inst.persistent == nil {
			err = resourcenotfound.ErrInstance
			return
		}

		buffers = *inst.persistent
		newImage, err = image.Snapshot(oldProg.image, inst.image, buffers, suspended)
		return
	}()
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			newImage.Close()
		}
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

	_, err = s.registerProgramRef(inst.acc, newProgram(moduleKey, newImage, buffers, true))
	if err != nil {
		return
	}

	s.Monitor(&event.InstanceSnapshot{
		Ctx:      Context(ctx, pri),
		Instance: inst.id,
		Module:   moduleKey,
	}, nil)
	return
}

func (s *Server) Instances(ctx context.Context, pri *principal.Key) (is Instances, err error) {
	err = s.AccessPolicy.Authorize(ctx, pri)
	if err != nil {
		return
	}

	// Get instance references while holding server lock.
	list := func() (list []*Instance) {
		s.lock.Lock()
		defer s.lock.Unlock()

		acc := s.accounts[inprincipal.RawKey(pri)]
		if acc == nil {
			return
		}

		list = make([]*Instance, 0, len(acc.instances))
		for _, i := range acc.instances {
			list = append(list, i)
		}
		return
	}()

	// Get instance statuses.  Each instance has its own lock.
	is = make(Instances, 0, len(list))
	for _, i := range list {
		is = append(is, InstanceStatus{
			Instance:  i.ID(),
			Status:    i.Status(),
			Transient: i.persistent == nil,
		})
	}

	s.Monitor(&event.InstanceList{
		Ctx: Context(ctx, pri),
	}, nil)
	return
}

func (s *Server) ensureAccount(pri *principal.Key) (acc *account, err error) {
	s.lock.Lock()
	defer s.lock.Unlock()

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

func (s *Server) refProgram(ctx context.Context, hash string, length int64) (prog *program, err error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	prog = s.programs[hash]
	if prog == nil {
		return
	}

	if length != prog.image.ModuleSize() {
		err = errModuleSizeMismatch
		return
	}

	prog.ref()
	return
}

func (s *Server) unrefProgram(prog *program) {
	s.lock.Lock()
	defer s.lock.Unlock()

	prog.unref()
}

func (s *Server) refAccountProgram(acc *account, hash string) *program {
	s.lock.Lock()
	defer s.lock.Unlock()

	if prog := s.programs[hash]; prog != nil {
		if _, own := acc.programRefs[prog]; own {
			return prog.ref()
		}
	}

	return nil
}

func (s *Server) refPrincipalProgram(pri *principal.Key, hash string) *program {
	s.lock.Lock()
	defer s.lock.Unlock()

	if acc := s.accounts[inprincipal.RawKey(pri)]; acc != nil {
		if prog := s.programs[hash]; prog != nil {
			if _, own := acc.programRefs[prog]; own {
				return prog.ref()
			}
		}
	}

	return nil
}

func (s *Server) getAccountAndPrincipalProgramWithCallerLock(pri *principal.Key, hash string,
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
// reference is stolen.
func (s *Server) registerProgramRef(acc *account, prog *program) (redundant bool, err error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	prog, redundant, err = s.mergeProgramRef(prog)
	if err != nil {
		return
	}

	if acc != nil {
		acc.ensureRefProgram(prog)
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
		s.lock.Lock()
		defer s.lock.Unlock()

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
			err = acc.checkUniqueInstanceID(instID)
			if err != nil {
				return
			}
		}
	}
	return
}

func (s *Server) getInstance(pri *principal.Key, instID string) *Instance {
	s.lock.Lock()
	defer s.lock.Unlock()

	acc := s.accounts[inprincipal.RawKey(pri)]
	if acc == nil {
		return nil
	}

	return acc.instances[instID]
}

func (s *Server) refInstanceProgram(pri *principal.Key, instID string) (*program, *Instance) {
	s.lock.Lock()
	defer s.lock.Unlock()

	acc := s.accounts[inprincipal.RawKey(pri)]
	if acc == nil {
		return nil, nil
	}

	inst := acc.instances[instID]
	if inst == nil {
		return nil, nil
	}

	return inst.prog.ref(), inst
}

func (s *Server) allocateInstanceResources(ctx context.Context, pri *principal.Key, pol *InstancePolicy, debugOption string,
) (proc *runtime.Process, services InstanceServices, debugStatus string, debugOutput io.WriteCloser, err error) {
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
		defer func() {
			if err != nil {
				debugOutput.Close()
			}
		}()
	}

	if pol.Services == nil {
		err = AccessForbidden("no service policy")
		return
	}
	services = pol.Services(ctx)
	defer func() {
		if err != nil {
			services.Close()
		}
	}()

	proc, err = s.ProcessFactory.NewProcess(ctx)
	if err != nil {
		return
	}

	return
}

// registerProgramRefInstance with server, and an account if refModule is true.
// Caller's program reference and instance image are stolen (except on error).
func (s *Server) registerProgramRefInstance(ctx context.Context, acc *account, refModule bool, prog *program, instImage *image.Instance, pol *InstancePolicy, persistInst bool, function, instID, debug string,
) (inst *Instance, redundant bool, err error) {
	var pri *principal.Key
	if acc != nil {
		pri = acc.Key
	}

	proc, services, debugStatus, debugOutput, err := s.allocateInstanceResources(ctx, pri, pol, debug)
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			debugOutput.Close()
			proc.Kill()
			services.Close()
		}
	}()

	if instID == "" {
		instID = makeInstanceID()
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	if acc != nil {
		err = acc.checkUniqueInstanceID(instID)
		if err != nil {
			return
		}
	}

	prog, redundant, err = s.mergeProgramRef(prog)
	if err != nil {
		return
	}

	inst = newInstance(acc, instID, prog.ref(), persistInst, function, instImage, proc, pol.TimeResolution, services, debugStatus, debugOutput)

	if acc != nil {
		if refModule {
			acc.ensureRefProgram(prog)
		}
		acc.instances[instID] = inst
	}

	return
}

// mergeProgramRef must be called with Server.lock held.  The returned program
// pointer is valid until the end of the critical section.
func (s *Server) mergeProgramRef(prog *program) (canonical *program, redundant bool, err error) {
	switch existing := s.programs[prog.key]; existing {
	case nil:
		if s.programs == nil {
			return nil, false, context.Canceled
		}
		s.programs[prog.key] = prog // Pass reference to map.
		return prog, false, nil

	case prog:
		if prog.refCount < 2 {
			panic("unexpected program reference count")
		}
		prog.unref() // Map has reference; safe to drop temporary reference.
		return prog, false, nil

	default:
		prog.unref()
		return existing, true, nil
	}
}
