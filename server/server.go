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
	if s.ProgramStorage == nil {
		s.ProgramStorage = image.Memory
	}
	if s.InstanceStorage == nil {
		s.InstanceStorage = image.Memory
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
			MaxABIVersion: runtimeabi.MaxVersion,
			MinABIVersion: runtimeabi.MinVersion,
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
	prog, found := s.refProgram(ctx, allegedHash)
	if !found {
		return
	}
	defer func() {
		if err != nil {
			s.unrefProgram(prog)
		}
	}()

	err = validateHashContent(allegedHash, content)
	if err != nil {
		return
	}

	if prog.Manifest().TextSize > uint32(pol.prog.MaxTextSize) {
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

func (s *Server) uploadUnknownModule(ctx context.Context, acc *account, pol *progPolicy, allegedHash string, content io.ReadCloser, contentSize int,
) (err error) {
	prog, _, err := buildProgram(&pol.prog, s.ProgramStorage, nil, nil, allegedHash, content, contentSize, "")
	if err != nil {
		return
	}

	redundant := s.registerProgramRef(acc, prog)

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

	prog, found := s.refAccountProgram(acc, progHash)
	if !found {
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

	inst.image, err = image.NewInstance(s.InstanceStorage, prog.Program, pol.inst.StackSize, entryIndex, entryAddr)
	if err != nil {
		return
	}

	err = inst.process.Start(prog.Program, inst.image)
	if err != nil {
		return
	}

	_, err = s.registerProgramRefInstance(acc, prog, inst, function)
	if err != nil {
		return
	}

	kill = false

	s.Monitor(&event.InstanceCreateLocal{
		Ctx:      Context(ctx, pri),
		Instance: inst.id,
		Module:   prog.key,
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

	progHash = inst.prog.key
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

	prog, found := s.refProgram(ctx, allegedHash)
	if !found {
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

	if prog.Manifest().TextSize > uint32(pol.prog.MaxTextSize) {
		err = resourcelimit.New("program code size limit exceeded")
		return
	}

	// TODO: check resource policy (stack/memory/max-memory size etc.)

	entryIndex, entryAddr, err := prog.resolveEntry(function)
	if err != nil {
		return
	}

	inst.image, err = image.NewInstance(s.InstanceStorage, prog.Program, pol.inst.StackSize, entryIndex, entryAddr)
	if err != nil {
		return
	}

	err = inst.process.Start(prog.Program, inst.image)
	if err != nil {
		return
	}

	_, err = s.registerProgramRefInstance(acc, prog, inst, function)
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

func (s *Server) loadUnknownModuleInstance(ctx context.Context, acc *account, inst *Instance, pol *instProgPolicy, allegedHash string, content io.ReadCloser, contentSize int, function string,
) (err error) {
	var prog *program

	prog, inst.image, err = buildProgram(&pol.prog, s.ProgramStorage, &pol.inst, s.InstanceStorage, allegedHash, content, contentSize, function)
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			s.unrefProgram(prog)
		}
	}()

	err = inst.process.Start(prog.Program, inst.image)
	if err != nil {
		return
	}

	redundant, err := s.registerProgramRefInstance(acc, prog, inst, function)
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

		acc, found := s.accounts[pri.key]
		if !found {
			return nil
		}

		refs := make(ModuleRefs, 0, len(acc.programRefs))
		for prog := range acc.programRefs {
			refs = append(refs, ModuleRef{
				Key:       prog.key,
				Suspended: prog.Manifest().InitRoutine == objectabi.TextAddrResume,
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

	prog, found := s.refPrincipalProgram(pri, hash)
	if !found {
		err = resourcenotfound.ErrModule
		return
	}

	content = moduleContent{
		Reader: prog.NewModuleReader(),
		done: func() {
			defer s.unrefProgram(prog)
			s.Monitor(&event.ModuleDownload{
				Ctx:    Context(ctx, pri),
				Module: prog.key,
			}, nil)
		},
	}
	length = prog.ModuleSize()
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

	// TODO: LRU cache

	var (
		acc   *account
		prog  *program
		found bool
		final bool
	)

	err = func() error {
		s.lock.Lock()
		defer s.lock.Unlock()

		acc, prog, found = s.getAccountAndPrincipalProgramWithCallerLock(pri, hash)
		if !found {
			return resourcenotfound.ErrModule
		}

		final = acc.unrefProgram(prog)
		if final {
			delete(s.programs, hash)
		}

		return nil
	}()
	if err != nil {
		return
	}

	if final {
		prog.Close()
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

	prog, inst, found := s.refInstanceProgram(pri, instID)
	if !found {
		err = resourcenotfound.ErrInstance
		return
	}
	defer func() {
		if err != nil {
			s.unrefProgram(prog)
		}
	}()

	var suspended bool

	switch inst.Status().State {
	case serverapi.Status_suspended:
		suspended = true

	case serverapi.Status_terminated:
		suspended = false

	default:
		err = failrequest.Errorf(event.FailRequest_InstanceStatus, "instance must be suspended or terminated")
		return
	}

	newImage, err := image.Snapshot(s.ProgramStorage, prog.Program, inst.image, suspended)
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

func (s *Server) refProgram(ctx context.Context, hash string) (prog *program, found bool) {
	s.lock.Lock()
	prog, found = s.programs[hash]
	if found {
		prog.ref()
	}
	s.lock.Unlock()
	if found {
		return
	}

	progImage, err := s.ProgramStorage.LoadProgram(hash)
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
	prog, _ = s.mergeProgramRef(prog)
	prog.ref()
	s.lock.Unlock()
	found = true
	return
}

func (s *Server) unrefProgram(prog *program) {
	if func() bool {
		s.lock.Lock()
		defer s.lock.Unlock()

		return prog.unref()
	}() {
		prog.Close()
	}
}

func (s *Server) refAccountProgram(acc *account, hash string) (*program, bool) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if prog, exists := s.programs[hash]; exists {
		if _, referenced := acc.programRefs[prog]; referenced {
			return prog.ref(), true
		}
	}

	return nil, false
}

func (s *Server) refPrincipalProgram(pri *PrincipalKey, hash string) (prog *program, refFound bool) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if acc, ok := s.accounts[pri.key]; ok {
		if prog, exists := s.programs[hash]; exists {
			if _, referenced := acc.programRefs[prog]; referenced {
				return prog.ref(), true
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

// registerProgramRef with the server and an account.  Caller's program
// reference is stolen.
func (s *Server) registerProgramRef(acc *account, prog *program) (redundant bool) {
	s.lock.Lock()
	defer s.lock.Unlock()

	prog, redundant = s.mergeProgramRef(prog)
	acc.ensureRefProgram(prog)
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

func (s *Server) refInstanceProgram(pri *PrincipalKey, instID string,
) (prog *program, inst *Instance, found bool) {
	s.lock.Lock()
	defer s.lock.Unlock()

	acc, accFound := s.accounts[pri.key]
	if !accFound {
		return
	}

	inst, found = acc.instances[instID]
	if !found {
		return
	}

	prog = inst.prog.ref()
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

	inst.acc = acc
	inst.services = services
	inst.id = id
	return
}

// registerProgramRefInstance with server and an account (if any).  Caller's
// program reference is stolen (except on error).
func (s *Server) registerProgramRefInstance(acc *account, prog *program, inst *Instance, function string,
) (redundant bool, err error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if acc != nil {
		err = acc.checkUniqueInstanceID(inst.id)
		if err != nil {
			return
		}
	}

	prog, redundant = s.mergeProgramRef(prog)
	inst.refProgram(prog, function)

	if acc != nil {
		acc.ensureRefProgram(prog)
		acc.instances[inst.id] = inst
	}
	return
}

// mergeProgramRef must be called with Server.lock held.  The returned program
// pointer is valid until the end of the critical section.
func (s *Server) mergeProgramRef(prog *program) (canonical *program, redundant bool) {
	switch existing := s.programs[prog.key]; existing {
	case nil:
		s.programs[prog.key] = prog // Pass reference to map.
		return prog, false

	case prog:
		prog.unref() // Map has reference; safe to drop temporary reference.
		return prog, false

	default:
		if prog.unref() { // Drop reference to replaced object.
			prog.Close()
		}
		return existing, true
	}
}
