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
	"github.com/tsavola/gate/internal/serverapi"
	"github.com/tsavola/gate/runtime"
	runtimeabi "github.com/tsavola/gate/runtime/abi"
	"github.com/tsavola/gate/server/event"
	"github.com/tsavola/gate/server/internal/error/failrequest"
	"github.com/tsavola/gate/server/internal/error/resourcenotfound"
	objectabi "github.com/tsavola/wag/object/abi"
)

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

type Info struct {
	Runtime RuntimeInfo `json:"runtime"`
}

type RuntimeInfo struct {
	MaxABIVersion int `json:"max_abi_version"`
	MinABIVersion int `json:"min_abi_version"`
}

type Server struct {
	Config
	Info *Info

	lock     sync.Mutex
	accounts map[[principalKeySize]byte]*account
	programs map[string]*program
}

func New(ctx context.Context, config *Config) *Server {
	s := new(Server)

	if config != nil {
		s.Config = *config
	}
	if s.ImageStorage == nil {
		s.ImageStorage = image.Memory
	}
	if s.Monitor == nil {
		s.Monitor = defaultMonitor
	}
	if !s.Configured() {
		panic("incomplete server configuration")
	}

	s.Info = &Info{
		Runtime: RuntimeInfo{
			MaxABIVersion: runtimeabi.MaxVersion,
			MinABIVersion: runtimeabi.MinVersion,
		},
	}

	s.accounts = make(map[[principalKeySize]byte]*account)
	s.programs = make(map[string]*program)

	return s
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

			if inst.status.State == serverapi.Status_running {
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
func (s *Server) UploadModule(ctx context.Context, pri *PrincipalKey, refModule bool, allegedHash string, content io.ReadCloser, contentLength int64,
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
func (s *Server) SourceModule(ctx context.Context, pri *PrincipalKey, refModule bool, source Source, uri string,
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

	if prog.image.Manifest().TextSize > uint32(pol.prog.MaxTextSize) {
		err = resourcelimit.New("program code size limit exceeded")
		return
	}

	s.registerProgramRef(acc, prog)

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
func (s *Server) CreateInstance(ctx context.Context, pri *PrincipalKey, progHash, function, instID, debug string,
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

	entryIndex, entryAddr, err := prog.resolveEntry(function)
	if err != nil {
		return
	}

	// TODO: check resource policy (text/stack/memory/max-memory size etc.)

	instImage, err := image.NewInstance(prog.image, pol.inst.StackSize, entryIndex, entryAddr)
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			instImage.Close()
		}
	}()

	inst, _, err = s.registerProgramRefInstance(ctx, acc, false, prog, instImage, &pol.inst, function, instID, debug)
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
func (s *Server) UploadModuleInstance(ctx context.Context, pri *PrincipalKey, refModule bool, allegedHash string, content io.ReadCloser, contentLength int64, function, instID, debug string,
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

	inst, err = s.loadModuleInstance(ctx, acc, refModule, &pol, allegedHash, content, contentLength, function, instID, debug)
	content = nil
	if err != nil {
		return
	}

	return
}

// SourceModuleInstance creates a new module reference if refModule is true.
// The module is instantiated in any case.  Module content is read from a
// source.  Instance id is optional.
func (s *Server) SourceModuleInstance(ctx context.Context, pri *PrincipalKey, refModule bool, source Source, uri, function, instID, debug string,
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

	inst, err = s.loadModuleInstance(ctx, acc, refModule, &pol, "", content, int64(size), function, instID, debug)
	if err != nil {
		return
	}

	progHash = inst.prog.key
	return
}

func (s *Server) loadModuleInstance(ctx context.Context, acc *account, refModule bool, pol *instProgPolicy, allegedHash string, content io.ReadCloser, contentLength int64, function, instID, debug string,
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

	inst, err = s.loadKnownModuleInstance(ctx, acc, refModule, pol, allegedHash, content, contentLength, function, instID, debug)
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
		inst, err = s.loadUnknownModuleInstance(ctx, acc, refModule, pol, allegedHash, content, int(contentLength), function, instID, debug)
		if err != nil {
			return
		}
	}

	return
}

func (s *Server) loadKnownModuleInstance(ctx context.Context, acc *account, refModule bool, pol *instProgPolicy, allegedHash string, content io.Reader, contentLength int64, function, instID, debug string,
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

	if prog.image.Manifest().TextSize > uint32(pol.prog.MaxTextSize) {
		err = resourcelimit.New("program code size limit exceeded")
		return
	}

	// TODO: check resource policy (stack/memory/max-memory size etc.)

	entryIndex, entryAddr, err := prog.resolveEntry(function)
	if err != nil {
		return
	}

	instImage, err := image.NewInstance(prog.image, pol.inst.StackSize, entryIndex, entryAddr)
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			instImage.Close()
		}
	}()

	inst, _, err = s.registerProgramRefInstance(ctx, acc, refModule, prog, instImage, &pol.inst, function, instID, debug)
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

func (s *Server) loadUnknownModuleInstance(ctx context.Context, acc *account, refModule bool, pol *instProgPolicy, allegedHash string, content io.ReadCloser, contentSize int, function, instID, debug string,
) (inst *Instance, err error) {
	prog, instImage, err := buildProgram(s.ImageStorage, &pol.prog, &pol.inst, allegedHash, content, contentSize, function)
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			instImage.Close()
			s.unrefProgram(prog)
		}
	}()

	inst, redundant, err := s.registerProgramRefInstance(ctx, acc, refModule, prog, instImage, &pol.inst, function, instID, debug)
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

func (s *Server) ModuleRefs(ctx context.Context, pri *PrincipalKey) (refs ModuleRefs, err error) {
	err = s.AccessPolicy.Authorize(ctx, pri)
	if err != nil {
		return
	}

	refs = func() ModuleRefs {
		s.lock.Lock()
		defer s.lock.Unlock()

		acc := s.accounts[pri.key]
		if acc == nil {
			return nil
		}

		refs := make(ModuleRefs, 0, len(acc.programRefs))
		for prog := range acc.programRefs {
			refs = append(refs, ModuleRef{
				Key:       prog.key,
				Suspended: prog.image.Manifest().InitRoutine == objectabi.TextAddrResume,
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
func (s *Server) ModuleContent(ctx context.Context, pri *PrincipalKey, hash string,
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

func (s *Server) UnrefModule(ctx context.Context, pri *PrincipalKey, hash string) (err error) {
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

func (s *Server) InstanceConnection(ctx context.Context, pri *PrincipalKey, instID string,
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
			Failure:  event.FailRequest_InstanceNoConnect,
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
func (s *Server) InstanceStatus(ctx context.Context, pri *PrincipalKey, instID string,
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

func (s *Server) WaitInstance(ctx context.Context, pri *PrincipalKey, instID string,
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

func (s *Server) SuspendInstance(ctx context.Context, pri *PrincipalKey, instID string,
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

	inst.process.Suspend()

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

func (s *Server) ResumeInstance(ctx context.Context, pri *PrincipalKey, instID, debug string,
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

		if inst.status.State != serverapi.Status_suspended {
			err = failrequest.Errorf(event.FailRequest_InstanceStatus, "instance is not suspended")
			return
		}

		err = inst.image.CheckMutation()
		if err != nil {
			return
		}

		proc, services, debugStatus, debugOutput, err := s.allocateInstanceResources(ctx, &pol.inst, debug)
		if err != nil {
			return
		}

		inst.renew(proc, services, debugStatus, debugOutput)
		return
	}()
	if err != nil {
		inst = nil
		return
	}

	s.Monitor(&event.InstanceResume{
		Ctx:      Context(ctx, pri),
		Instance: inst.id,
	}, nil)
	return
}

func (s *Server) InstanceModule(ctx context.Context, pri *PrincipalKey, instID string,
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

	newImage, err := func() (*image.Program, error) {
		inst.lock.Lock()
		defer inst.lock.Unlock()

		var suspended bool

		switch inst.status.State {
		case serverapi.Status_suspended:
			suspended = true

		case serverapi.Status_terminated:
			suspended = false

		default:
			return nil, failrequest.Errorf(event.FailRequest_InstanceStatus, "instance must be suspended or terminated")
		}

		return image.Snapshot(oldProg.image, inst.image, inst.buffers, suspended)
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

	s.registerProgramRef(inst.acc, newProgram(moduleKey, newImage))

	s.Monitor(&event.InstanceSnapshot{
		Ctx:      Context(ctx, pri),
		Instance: inst.id,
		Module:   moduleKey,
	}, nil)
	return
}

func (s *Server) Instances(ctx context.Context, pri *PrincipalKey) (is Instances, err error) {
	err = s.AccessPolicy.Authorize(ctx, pri)
	if err != nil {
		return
	}

	// Get instance references while holding server lock.
	list := func() (list []*Instance) {
		s.lock.Lock()
		defer s.lock.Unlock()

		acc := s.accounts[pri.key]
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
			Instance: i.ID(),
			Status:   i.Status(),
		})
	}

	s.Monitor(&event.InstanceList{
		Ctx: Context(ctx, pri),
	}, nil)
	return
}

func (s *Server) ensureAccount(pri *PrincipalKey) (acc *account, err error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.accounts == nil {
		err = context.Canceled
		return
	}

	acc = s.accounts[pri.key]
	if acc == nil {
		acc = newAccount(pri)
		s.accounts[pri.key] = acc
	}
	return
}

func (s *Server) refProgram(ctx context.Context, hash string, length int64) (prog *program, err error) {
	s.lock.Lock()
	prog = s.programs[hash]
	if prog != nil {
		if length == prog.image.ModuleSize() {
			prog.ref()
		} else {
			err = errModuleSizeMismatch
		}
	}
	s.lock.Unlock()
	if prog != nil || err != nil {
		return
	}

	progImage, err := s.ImageStorage.LoadProgram(hash)
	if err != nil {
		s.Monitor(&event.FailInternal{
			Ctx:    Context(ctx, nil),
			Module: hash,
		}, err)
		return
	}
	if progImage == nil {
		return
	}

	prog = newProgram(hash, progImage)

	s.lock.Lock()
	defer s.lock.Unlock()

	prog, _, err = s.mergeProgramRef(prog)
	if err != nil {
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

func (s *Server) refPrincipalProgram(pri *PrincipalKey, hash string) *program {
	s.lock.Lock()
	defer s.lock.Unlock()

	if acc := s.accounts[pri.key]; acc != nil {
		if prog := s.programs[hash]; prog != nil {
			if _, own := acc.programRefs[prog]; own {
				return prog.ref()
			}
		}
	}

	return nil
}

func (s *Server) getAccountAndPrincipalProgramWithCallerLock(pri *PrincipalKey, hash string,
) (*account, *program) {
	acc := s.accounts[pri.key]
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
func (s *Server) checkInstanceIDAndEnsureAccount(pri *PrincipalKey, instID string,
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

		acc = s.accounts[pri.key]
		if acc == nil {
			acc = newAccount(pri)
			s.accounts[pri.key] = acc
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

func (s *Server) getInstance(pri *PrincipalKey, instID string) *Instance {
	s.lock.Lock()
	defer s.lock.Unlock()

	acc := s.accounts[pri.key]
	if acc == nil {
		return nil
	}

	return acc.instances[instID]
}

func (s *Server) refInstanceProgram(pri *PrincipalKey, instID string) (*program, *Instance) {
	s.lock.Lock()
	defer s.lock.Unlock()

	acc := s.accounts[pri.key]
	if acc == nil {
		return nil, nil
	}

	inst := acc.instances[instID]
	if inst == nil {
		return nil, nil
	}

	return inst.prog.ref(), inst
}

func (s *Server) allocateInstanceResources(ctx context.Context, pol *InstancePolicy, debugOption string,
) (proc *runtime.Process, services InstanceServices, debugStatus string, debugOutput io.WriteCloser, err error) {
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
	services = pol.Services()
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
func (s *Server) registerProgramRefInstance(ctx context.Context, acc *account, refModule bool, prog *program, instImage *image.Instance, pol *InstancePolicy, function, instID, debug string,
) (inst *Instance, redundant bool, err error) {
	proc, services, debugStatus, debugOutput, err := s.allocateInstanceResources(ctx, pol, debug)
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

	inst = newInstance(acc, instID, prog.ref(), function, instImage, proc, services, debugStatus, debugOutput)

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
