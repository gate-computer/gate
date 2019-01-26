// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"
	"io"
	"sync"

	"github.com/tsavola/gate/image"
	"github.com/tsavola/gate/internal/error/resourcelimit"
	"github.com/tsavola/gate/internal/serverapi"
	"github.com/tsavola/gate/runtime/abi"
	"github.com/tsavola/gate/server/event"
	"github.com/tsavola/gate/server/internal/error/resourcenotfound"
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

	instanceFactory <-chan *Instance

	lock     sync.Mutex
	accounts map[[principalKeySize]byte]*account
	programs map[string]*program
}

func New(ctx context.Context, config *Config) *Server {
	s := new(Server)

	if config != nil {
		s.Config = *config
	}
	if s.InstanceStore == nil {
		s.InstanceStore = image.Memory
	}
	if s.ProgramStorage == nil {
		s.ProgramStorage = image.Memory
	}
	if s.PreforkProcs == 0 {
		s.PreforkProcs = DefaultPreforkProcs
	}
	if s.Monitor == nil {
		s.Monitor = defaultMonitor
	}
	if !s.Configured() {
		panic("incomplete server configuration")
	}

	s.Info = &Info{
		Runtime: RuntimeInfo{
			MaxABIVersion: abi.MaxVersion,
			MinABIVersion: abi.MinVersion,
		},
	}

	s.instanceFactory = makeInstanceFactory(ctx, s)
	s.accounts = make(map[[principalKeySize]byte]*account)
	s.programs = make(map[string]*program)

	return s
}

// UploadModule creates a new module reference.  Caller provides module
// content.
func (s *Server) UploadModule(ctx context.Context, pri *PrincipalKey, allegedHash string, content io.ReadCloser, contentLength int64,
) (err error) {
	closeContent := true

	defer func() {
		if closeContent {
			content.Close()
		}
	}()

	var pol progPolicy

	err = s.AccessPolicy.AuthorizeProgramContent(ctx, pri, &pol.res, &pol.prog)
	if err != nil {
		return
	}

	if contentLength > int64(pol.prog.MaxModuleSize) {
		err = resourcelimit.New("module size limit exceeded")
		return
	}

	acc := s.ensureAccount(pri)

	// TODO: check resource policy

	found, err := s.uploadKnownModule(ctx, acc, &pol, allegedHash, content)
	if err != nil {
		return
	}

	closeContent = false

	if found {
		err = content.Close()
		if err != nil {
			err = wrapContentError(err)
			return
		}
	} else {
		err = s.uploadUnknownModule(ctx, acc, &pol, allegedHash, content, int(contentLength))
		if err != nil {
			return
		}
	}
	return
}

func (s *Server) uploadKnownModule(ctx context.Context, acc *account, pol *progPolicy, allegedHash string, content io.Reader,
) (found bool, err error) {
	prog, found := s.getProgram(allegedHash)
	if !found {
		return
	}

	err = validateHashContent(allegedHash, content)
	if err != nil {
		return
	}

	if prog.TextSize > pol.prog.MaxTextSize {
		err = resourcelimit.New("program code size limit exceeded")
		return
	}

	if replaced := s.registerProgram(acc, prog); replaced {
		prog.Close()
	}

	s.Monitor(&event.ModuleUploadExist{
		Ctx:    accountContext(ctx, acc),
		Module: prog.hash,
	}, nil)
	return
}

func (s *Server) uploadUnknownModule(ctx context.Context, acc *account, pol *progPolicy, allegedHash string, content io.ReadCloser, contentSize int,
) (err error) {
	_, prog, err := compileProgram(ctx, nil, nil, &pol.prog, s.ProgramStorage, allegedHash, content, contentSize, "")
	if err != nil {
		return
	}

	found := s.registerProgram(acc, prog)
	if found {
		prog.Close()
	}

	if found {
		s.Monitor(&event.ModuleUploadExist{
			Ctx:      accountContext(ctx, acc),
			Module:   prog.hash,
			Compiled: true,
		}, nil)
	} else {
		s.Monitor(&event.ModuleUploadNew{
			Ctx:    accountContext(ctx, acc),
			Module: prog.hash,
		}, nil)
	}
	return
}

// CreateInstance instantiates a module reference.  Instance id is optional.
func (s *Server) CreateInstance(ctx context.Context, pri *PrincipalKey, progHash, function, instID string,
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

	prog, found := s.getAccountProgram(acc, progHash)
	if !found {
		err = resourcenotfound.ErrModule
		return
	}

	entryAddr, err := prog.getEntryAddr(function)
	if err != nil {
		return
	}

	// TODO: check resource policy

	inst, err = s.newInstance(ctx, acc, pol.inst.Services, instID)
	if err != nil {
		return
	}

	kill := true

	defer func() {
		if kill {
			inst.Close()
			inst = nil
		}
	}()

	inst.exe, err = prog.loadExecutable(ctx, inst.ref, &pol.inst, entryAddr)
	if err != nil {
		return
	}

	err = inst.process.Start(inst.exe, prog.routine)
	if err != nil {
		return
	}

	_, err = s.registerInstance(acc, prog, inst, function)
	if err != nil {
		return
	}

	kill = false

	s.Monitor(&event.InstanceCreateLocal{
		Ctx:      Context(ctx, pri),
		Instance: inst.id,
		Module:   prog.hash,
	}, nil)
	return
}

// UploadModuleInstance creates a new module reference and instantiates it.
// Caller provides module content.  Instance id is optional.
func (s *Server) UploadModuleInstance(ctx context.Context, pri *PrincipalKey, allegedHash string, content io.ReadCloser, contentLength int64, function, instID string,
) (inst *Instance, err error) {
	closeContent := true

	defer func() {
		if closeContent {
			content.Close()
		}
	}()

	var pol instProgPolicy

	err = s.AccessPolicy.AuthorizeInstanceProgramContent(ctx, pri, &pol.res, &pol.inst, &pol.prog)
	if err != nil {
		return
	}

	acc, err := s.checkInstanceIDAndEnsureAccount(pri, instID)
	if err != nil {
		return
	}

	closeContent = false

	return s.loadModuleInstance(ctx, acc, &pol, allegedHash, content, contentLength, function, instID)
}

// SourceModuleInstance creates a new module reference and instantiates it.
// Module content is read from a source.  Instance id is optional.
func (s *Server) SourceModuleInstance(ctx context.Context, pri *PrincipalKey, source Source, uri, function, instID string,
) (progHash string, inst *Instance, err error) {
	var pol instProgPolicy

	err = s.AccessPolicy.AuthorizeInstanceProgramSource(ctx, pri, &pol.res, &pol.inst, &pol.prog, source)
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

	inst, err = s.loadModuleInstance(ctx, acc, &pol, "", content, int64(size), function, instID)
	if err != nil {
		return
	}

	progHash = inst.prog.hash
	return
}

func (s *Server) loadModuleInstance(ctx context.Context, acc *account, pol *instProgPolicy, allegedHash string, content io.ReadCloser, contentLength int64, function, instID string,
) (inst *Instance, err error) {
	var closeContent = true

	defer func() {
		if closeContent {
			content.Close()
		}
	}()

	if contentLength > int64(pol.prog.MaxModuleSize) {
		err = resourcelimit.New("module size limit exceeded")
		return
	}

	// TODO: check resource policy

	inst, err = s.newInstance(ctx, acc, pol.inst.Services, instID)
	if err != nil {
		return
	}

	kill := true

	defer func() {
		if kill {
			inst.Close()
			inst = nil
		}
	}()

	found, err := s.loadKnownModuleInstance(ctx, acc, inst, pol, allegedHash, content, function)
	if err != nil {
		return
	}

	closeContent = false

	if found {
		err = content.Close()
		if err != nil {
			err = wrapContentError(err)
			return
		}
	} else {
		err = s.loadUnknownModuleInstance(ctx, acc, inst, pol, allegedHash, content, int(contentLength), function)
		if err != nil {
			return
		}
	}

	kill = false
	return
}

func (s *Server) loadKnownModuleInstance(ctx context.Context, acc *account, inst *Instance, pol *instProgPolicy, allegedHash string, content io.Reader, function string,
) (found bool, err error) {
	if allegedHash == "" {
		return
	}

	prog, found := s.getProgram(allegedHash)
	if !found {
		return
	}

	err = validateHashContent(prog.hash, content)
	if err != nil {
		return
	}

	if prog.TextSize > pol.prog.MaxTextSize {
		err = resourcelimit.New("program code size limit exceeded")
		return
	}

	entryAddr, err := prog.getEntryAddr(function)
	if err != nil {
		return
	}

	inst.exe, err = prog.loadExecutable(ctx, inst.ref, &pol.inst, entryAddr)
	if err != nil {
		return
	}

	err = inst.process.Start(inst.exe, prog.routine)
	if err != nil {
		return
	}

	_, err = s.registerInstance(acc, prog, inst, function)
	if err != nil {
		return
	}

	s.Monitor(&event.ModuleUploadExist{
		Ctx:    accountContext(ctx, acc),
		Module: prog.hash,
	}, nil)

	s.Monitor(&event.InstanceCreateLocal{
		Ctx:      accountContext(ctx, acc),
		Instance: inst.id,
		Module:   prog.hash,
	}, nil)
	return
}

func (s *Server) loadUnknownModuleInstance(ctx context.Context, acc *account, inst *Instance, pol *instProgPolicy, allegedHash string, content io.ReadCloser, contentSize int, function string,
) (err error) {
	var prog *program

	inst.exe, prog, err = compileProgram(ctx, inst.ref, &pol.inst, &pol.prog, s.ProgramStorage, allegedHash, content, contentSize, function)
	if err != nil {
		return
	}

	err = inst.process.Start(inst.exe, prog.routine)
	if err != nil {
		return
	}

	found, err := s.registerInstance(acc, prog, inst, function)
	if err != nil {
		return
	}

	if allegedHash != "" {
		if found {
			s.Monitor(&event.ModuleUploadExist{
				Ctx:      accountContext(ctx, acc),
				Module:   prog.hash,
				Compiled: true,
			}, nil)
		} else {
			s.Monitor(&event.ModuleUploadNew{
				Ctx:    accountContext(ctx, acc),
				Module: prog.hash,
			}, nil)
		}
	} else {
		if found {
			s.Monitor(&event.ModuleSourceExist{
				Ctx:    accountContext(ctx, acc),
				Module: prog.hash,
				// TODO: source URI
				Compiled: true,
			}, nil)
		} else {
			s.Monitor(&event.ModuleSourceNew{
				Ctx:    accountContext(ctx, acc),
				Module: prog.hash,
				// TODO: source URI
			}, nil)
		}
	}

	s.Monitor(&event.InstanceCreateStream{
		Ctx:      accountContext(ctx, acc),
		Instance: inst.id,
		Module:   prog.hash,
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

		acc, found := s.accounts[pri.key]
		if !found {
			return nil
		}

		refs := make(ModuleRefs, 0, len(acc.programRefs))
		for prog := range acc.programRefs {
			refs = append(refs, ModuleRef{Key: prog.hash})
		}

		return refs
	}()

	s.Monitor(&event.ModuleList{
		Ctx: Context(ctx, pri),
	}, nil)
	return
}

// ModuleContent for downloading.  The caller must call ModuleLoad.Close and
// Server.ModuleDownloaded when it's done downloading.
func (s *Server) ModuleContent(ctx context.Context, pri *PrincipalKey, hash string,
) (content image.ModuleLoad, err error) {
	err = s.AccessPolicy.Authorize(ctx, pri)
	if err != nil {
		return
	}

	prog, found := s.getPrincipalProgram(pri, hash)
	if !found {
		err = resourcenotfound.ErrModule
		return
	}

	return prog.module.Open(ctx)
}

func (s *Server) ModuleDownloaded(ctx context.Context, pri *PrincipalKey, hash string, err error) {
	s.Monitor(&event.ModuleDownload{
		Ctx:    Context(ctx, pri),
		Module: hash,
	}, err)
}

func (s *Server) UnrefModule(ctx context.Context, pri *PrincipalKey, hash string) (err error) {
	err = s.AccessPolicy.Authorize(ctx, pri)
	if err != nil {
		return
	}

	err = func() error {
		s.lock.Lock()
		defer s.lock.Unlock()

		acc, prog, found := s.getAccountAndPrincipalProgramWithCallerLock(pri, hash)
		if !found {
			return resourcenotfound.ErrModule
		}

		delete(acc.programRefs, prog)
		prog.refCount-- // Unreferenced by account.

		// TODO: LRU cache
		if prog.refCount == 0 {
			delete(s.programs, hash)
		}

		return nil
	}()

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

	inst, found := s.getInstance(pri, instID)
	if !found {
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

	inst, found := s.getInstance(pri, instID)
	if !found {
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

	inst, found := s.getInstance(pri, instID)
	if !found {
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

	inst, found := s.getInstance(pri, instID)
	if !found {
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

func (s *Server) InstanceModule(ctx context.Context, pri *PrincipalKey, instID string,
) (moduleKey string, err error) {
	err = s.AccessPolicy.Authorize(ctx, pri)
	if err != nil {
		return
	}

	// TODO: check module storage limits

	inst, found := s.getInstance(pri, instID)
	if !found {
		err = resourcenotfound.ErrInstance
		return
	}

	prog, err := inst.snapshotModule(ctx, s.ProgramStorage)
	if err != nil {
		return
	}

	if replaced := s.registerProgram(inst.account, prog); replaced {
		prog.Close()
	}

	s.Monitor(&event.InstanceSnapshot{
		Ctx:      Context(ctx, pri),
		Instance: inst.id,
		Module:   prog.hash,
	}, nil)

	moduleKey = prog.hash
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

		acc, found := s.accounts[pri.key]
		if !found {
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

func (s *Server) ensureAccount(pri *PrincipalKey) (acc *account) {
	s.lock.Lock()
	defer s.lock.Unlock()

	acc, found := s.accounts[pri.key]
	if !found {
		acc = newAccount(pri)
		s.accounts[pri.key] = acc
	}
	return
}

func (s *Server) getProgram(hash string) (prog *program, found bool) {
	s.lock.Lock()
	defer s.lock.Unlock()

	prog, found = s.programs[hash]
	return
}

func (s *Server) getAccountProgram(acc *account, hash string) (*program, bool) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if prog, exists := s.programs[hash]; exists {
		if _, referenced := acc.programRefs[prog]; referenced {
			return prog, true
		}
	}

	return nil, false
}

func (s *Server) getPrincipalProgram(pri *PrincipalKey, hash string) (prog *program, refFound bool) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if acc, ok := s.accounts[pri.key]; ok {
		if prog, exists := s.programs[hash]; exists {
			if _, referenced := acc.programRefs[prog]; referenced {
				return prog, true
			}
		}
	}

	return nil, false
}

func (s *Server) getAccountAndPrincipalProgramWithCallerLock(pri *PrincipalKey, hash string,
) (acc *account, prog *program, refFound bool) {
	if acc, ok := s.accounts[pri.key]; ok {
		if prog, exists := s.programs[hash]; exists {
			if _, referenced := acc.programRefs[prog]; referenced {
				return acc, prog, true
			}
		}
	}

	return acc, nil, false
}

func (s *Server) registerProgram(acc *account, prog *program) (progFound bool) {
	s.lock.Lock()
	defer s.lock.Unlock()

	existingProg, progFound := s.programs[prog.hash]
	if progFound {
		prog = existingProg
	} else {
		if prog.orig != nil {
			prog.orig.refCount++ // It wasn't referenced when program object was created.
		}
		s.programs[prog.hash] = prog
	}

	if _, referenced := acc.programRefs[prog]; !referenced {
		acc.programRefs[prog] = struct{}{}
		prog.refCount++ // Referenced by account.
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

	if pri == nil {
		return
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	acc, found := s.accounts[pri.key]
	if !found {
		acc = newAccount(pri)
		s.accounts[pri.key] = acc
	}

	if instID != "" {
		err = acc.checkUniqueInstanceID(instID)
		if err != nil {
			return
		}
	}

	return
}

func (s *Server) getInstance(pri *PrincipalKey, instID string) (inst *Instance, found bool) {
	s.lock.Lock()
	defer s.lock.Unlock()

	acc, accFound := s.accounts[pri.key]
	if !accFound {
		return
	}

	inst, found = acc.instances[instID]
	return
}

func (s *Server) newInstance(ctx context.Context, acc *account, servicePolicy func() InstanceServices, id string,
) (inst *Instance, err error) {
	if servicePolicy == nil {
		err = AccessForbidden("no service policy")
		return
	}

	services := servicePolicy()

	select {
	case inst = <-s.instanceFactory:
		if inst == nil {
			err = context.Canceled // TODO: exact error
			return
		}

	case <-ctx.Done():
		err = ctx.Err()
		return
	}

	inst.account = acc
	inst.services = services
	inst.id = id
	return
}

func (s *Server) registerInstance(acc *account, prog *program, inst *Instance, function string,
) (progFound bool, err error) {
	inst.function = function

	if inst.id == "" {
		inst.id = makeInstanceID()
	}

	// Instance lock doesn't need to be held because the instance hasn't been
	// shared via s.instances yet.
	inst.status.State = serverapi.Status_running

	s.lock.Lock()
	defer s.lock.Unlock()

	existingProg, progFound := s.programs[prog.hash]
	if progFound {
		prog = existingProg
	} else {
		s.programs[prog.hash] = prog
	}

	inst.prog = prog
	prog.refCount++ // Referenced by instance.

	if acc == nil {
		return
	}

	if _, referenced := acc.programRefs[prog]; !referenced {
		acc.programRefs[prog] = struct{}{}
		prog.refCount++ // Referenced by account.
	}

	err = acc.checkUniqueInstanceID(inst.id)
	if err != nil {
		return
	}

	acc.instances[inst.id] = inst
	return
}
