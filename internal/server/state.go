// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"io"
	"log"
	"sync"

	"github.com/tsavola/gate/internal/publicerror"
	"github.com/tsavola/gate/run"
	"github.com/tsavola/gate/server/detail"
	"github.com/tsavola/gate/server/event"
	"github.com/tsavola/wag"
	"github.com/tsavola/wag/trap"
	"github.com/tsavola/wag/wasm"
)

const (
	maxStackSize = 0x40000000 // crazy but valid
)

var (
	newHash  = sha512.New384
	encoding = base64.RawURLEncoding
)

func defaultMonitorError(p *detail.Position, err error) {
	log.Printf("%v: %v", p, err)
}

func defaultMonitorEvent(Event, error) {
}

type State struct {
	Config

	instanceFactory <-chan *Instance

	lock           sync.Mutex
	programsByHash map[string]*program
	programs       map[string]*program
	instances      map[string]*Instance
}

func (s *State) Init(ctx context.Context, conf Config) {
	if conf.MonitorError == nil {
		conf.MonitorError = defaultMonitorError
	}
	if conf.MonitorEvent == nil {
		conf.MonitorEvent = defaultMonitorEvent
	}

	if conf.MaxProgramSize <= 0 {
		conf.MaxProgramSize = DefaultMaxProgramSize
	}

	if conf.MemorySizeLimit > 0 {
		conf.MemorySizeLimit = (conf.MemorySizeLimit + wasm.Page - 1) &^ (wasm.Page - 1)
	} else {
		conf.MemorySizeLimit = DefaultMemorySizeLimit
	}

	if conf.StackSize > maxStackSize {
		conf.StackSize = maxStackSize
	} else if conf.StackSize <= 0 {
		conf.StackSize = DefaultStackSize
	}

	if conf.PreforkProcs <= 0 {
		conf.PreforkProcs = DefaultPreforkProcs
	}

	s.Config = conf
	s.instanceFactory = makeInstanceFactory(ctx, s)
	s.programsByHash = make(map[string]*program)
	s.programs = make(map[string]*program)
	s.instances = make(map[string]*Instance)
}

func (s *State) newInstance(ctx context.Context) (inst *Instance, err error) {
	select {
	case inst = <-s.instanceFactory:
		if inst == nil {
			err = publicerror.Shutdown("instance factory", context.Canceled)
		}

	case <-ctx.Done():
		err = publicerror.Shutdown("instance factory", ctx.Err())
	}
	return
}

func (s *State) mustBeUniqueProgramId(progId string) {
	if _, exists := s.programs[progId]; exists {
		panic("duplicate program id")
	}
}

func (s *State) mustBeUniqueInstanceId(instId string) {
	if _, exists := s.instances[instId]; exists {
		panic("duplicate instance id")
	}
}

func (s *State) getProgram(progId string) (prog *program, found bool) {
	s.lock.Lock()
	defer s.lock.Unlock()

	prog, found = s.programs[progId]
	return
}

func (s *State) setProgramForOwner(progId string, prog *program) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.mustBeUniqueProgramId(progId)

	prog.ownerCount++
	s.programs[progId] = prog
}

func (s *State) getProgramForInstance(progId string) (prog *program, found bool) {
	s.lock.Lock()
	defer s.lock.Unlock()

	prog, found = s.programs[progId]
	if found {
		prog.instanceCount++
	}
	return
}

func (s *State) unrefProgramForInstance(prog *program) {
	s.lock.Lock()
	defer s.lock.Unlock()

	prog.instanceCount--
}

func (s *State) getProgramByHash(progHash string) (prog *program, found bool) {
	s.lock.Lock()
	defer s.lock.Unlock()

	prog, found = s.programsByHash[progHash]
	return
}

func (s *State) getInstance(instId string) (inst *Instance, found bool) {
	s.lock.Lock()
	defer s.lock.Unlock()

	inst, found = s.instances[instId]
	return
}

func (s *State) setInstance(instId string, inst *Instance) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.mustBeUniqueInstanceId(instId)

	s.instances[instId] = inst
}

func (s *State) Upload(ctx context.Context, wasm io.ReadCloser, clientHash string,
) (progId string, valid bool, err error) {
	progId, valid, found, err := s.uploadKnown(wasm, clientHash)
	if err != nil {
		return
	}

	if !found {
		var loaded bool

		progId, valid, loaded, err = s.uploadUnknown(wasm, clientHash)
		if err != nil {
			return
		}

		if loaded {
			s.MonitorEvent(&event.ProgramLoad{
				Context:     Context(ctx),
				ProgramHash: clientHash,
			}, nil)
		}
	}

	s.MonitorEvent(&event.ProgramCreate{
		Context:     Context(ctx),
		ProgramHash: clientHash,
		ProgramId:   progId,
	}, nil)
	return
}

func (s *State) uploadKnown(wasm io.ReadCloser, clientHash string,
) (progId string, valid, found bool, err error) {
	prog, found := s.getProgramByHash(clientHash)
	if !found {
		return
	}

	valid, err = validateReadHash(prog.hash, wasm)
	if err != nil {
		return
	}
	if !valid {
		return
	}

	progId = makeId()
	s.setProgramForOwner(progId, prog)
	return
}

func (s *State) uploadUnknown(wasm io.ReadCloser, clientHash string,
) (progId string, valid, loaded bool, err error) {
	prog, valid, err := loadProgram(wasm, clientHash, s.Runtime)
	if err != nil {
		return
	}
	if !valid {
		return
	}

	progId = makeId()

	s.lock.Lock()
	defer s.lock.Unlock()

	s.mustBeUniqueProgramId(progId)

	if existing, found := s.programsByHash[prog.hash]; found {
		// Some other connection uploaded same program before we finished
		prog = existing
	} else {
		s.programsByHash[prog.hash] = prog
		loaded = true
	}
	prog.ownerCount++
	s.programs[progId] = prog
	return
}

func (s *State) Check(ctx context.Context, clientHash, progId string) (valid, found bool) {
	prog, found := s.getProgram(progId)
	if !found {
		return
	}

	valid = validateStringHash(clientHash, prog.hash)
	if !valid {
		return
	}

	s.MonitorEvent(&event.ProgramCheck{
		Context:   Context(ctx),
		ProgramId: progId,
	}, nil)
	return
}

func (s *State) UploadAndInstantiate(ctx context.Context, wasm io.ReadCloser, clientHash string, originPipe *Pipe,
) (inst *Instance, instId, progId string, valid bool, err error) {
	var loaded bool

	inst, err = s.newInstance(ctx)
	if err != nil {
		return
	}

	killInst := true
	defer func() {
		if killInst {
			inst.kill(s)
		}
	}()

	instId, prog, progId, valid, found, err := s.uploadAndInstantiateKnown(wasm, clientHash, inst)
	if err != nil {
		return
	}

	if !found {
		instId, prog, progId, valid, loaded, err = s.uploadAndInstantiateUnknown(wasm, clientHash, inst)
		if err != nil {
			return
		}
	}

	removeProgAndInst := true
	defer func() {
		if removeProgAndInst {
			s.lock.Lock()
			defer s.lock.Unlock()
			delete(s.instances, instId)
			delete(s.programs, progId)
			prog.instanceCount--
			prog.ownerCount--
		}
	}()

	err = inst.populate(&prog.module, originPipe, s)
	if err != nil {
		return
	}

	removeProgAndInst = false
	killInst = false

	if loaded {
		s.MonitorEvent(&event.ProgramLoad{
			Context:     Context(ctx),
			ProgramHash: clientHash,
		}, nil)
	}

	s.MonitorEvent(&event.ProgramCreate{
		Context:     Context(ctx),
		ProgramHash: clientHash,
		ProgramId:   progId,
	}, nil)

	s.MonitorEvent(&event.InstanceCreate{
		Context:    Context(ctx),
		ProgramId:  progId,
		InstanceId: instId,
	}, nil)
	return
}

func (s *State) uploadAndInstantiateKnown(wasm io.ReadCloser, clientHash string, inst *Instance,
) (instId string, prog *program, progId string, valid, found bool, err error) {
	prog, found = s.getProgramByHash(clientHash)
	if !found {
		return
	}

	valid, err = validateReadHash(prog.hash, wasm)
	if err != nil {
		return
	}
	if !valid {
		return
	}

	progId = makeId()
	instId = makeId()

	s.lock.Lock()
	defer s.lock.Unlock()

	s.mustBeUniqueProgramId(progId)
	s.mustBeUniqueInstanceId(instId)

	prog.ownerCount++
	prog.instanceCount++
	s.programs[progId] = prog
	inst.program = prog
	s.instances[instId] = inst
	return
}

func (s *State) uploadAndInstantiateUnknown(wasm io.ReadCloser, clientHash string, inst *Instance,
) (instId string, prog *program, progId string, valid, loaded bool, err error) {
	prog, valid, err = loadProgram(wasm, clientHash, s.Runtime)
	if err != nil {
		return
	}

	progId = makeId()
	instId = makeId()

	s.lock.Lock()
	defer s.lock.Unlock()

	s.mustBeUniqueProgramId(progId)
	s.mustBeUniqueInstanceId(instId)

	if existing, found := s.programsByHash[prog.hash]; found {
		// Some other connection uploaded same program before we finished
		prog = existing
	} else {
		s.programsByHash[prog.hash] = prog
		loaded = true
	}
	prog.ownerCount++
	prog.instanceCount++
	s.programs[progId] = prog
	inst.program = prog
	s.instances[instId] = inst
	return
}

func (s *State) Instantiate(ctx context.Context, clientHash, progId string, originPipe *Pipe,
) (inst *Instance, instId string, valid, found bool, err error) {
	prog, found := s.getProgramForInstance(progId)
	if !found {
		return
	}

	unrefProg := true
	defer func() {
		if unrefProg {
			s.unrefProgramForInstance(prog)
		}
	}()

	valid = validateStringHash(prog.hash, clientHash)
	if !valid {
		return
	}

	inst, err = s.newInstance(ctx)
	if err != nil {
		return
	}

	killInst := true
	defer func() {
		if killInst {
			inst.kill(s)
		}
	}()

	inst.program = prog
	err = inst.populate(&prog.module, originPipe, s)
	if err != nil {
		return
	}

	instId = makeId()
	s.setInstance(instId, inst)

	killInst = false
	unrefProg = false

	s.MonitorEvent(&event.InstanceCreate{
		Context:    Context(ctx),
		ProgramId:  progId,
		InstanceId: instId,
	}, nil)
	return
}

func (s *State) AttachOrigin(ctx context.Context, instId string) (pipe *Pipe, found bool) {
	inst, found := s.getInstance(instId)
	if !found {
		return
	}

	pipe = inst.attachOrigin()
	if pipe == nil {
		return
	}

	s.MonitorEvent(&event.InstanceAttach{
		Context:    Context(ctx),
		InstanceId: instId,
	}, nil)
	return
}

func (s *State) Wait(ctx context.Context, instId string) (result *Result, found bool) {
	inst, found := s.getInstance(instId)
	if !found {
		return
	}

	result, found = s.WaitInstance(ctx, inst, instId)
	return
}

func (s *State) WaitInstance(ctx context.Context, inst *Instance, instId string) (result *Result, found bool) {
	result, found = <-inst.exit
	if !found {
		return
	}

	s.lock.Lock()
	delete(s.instances, instId)
	inst.program.instanceCount--
	s.lock.Unlock()

	s.MonitorEvent(&event.InstanceDelete{
		Context:    Context(ctx),
		InstanceId: instId,
	}, nil)
	return
}

type program struct {
	ownerCount    int
	instanceCount int
	module        wag.Module
	wasm          []byte
	hash          string
}

func loadProgram(body io.ReadCloser, clientHash string, rt *run.Runtime,
) (p *program, valid bool, err error) {
	var (
		wasm     bytes.Buffer
		realHash = newHash()
	)

	r := bufio.NewReader(io.TeeReader(io.TeeReader(body, &wasm), realHash))

	p = new(program)

	loadErr := run.Load(&p.module, r, rt, nil, nil)
	closeErr := body.Close()
	switch {
	case loadErr != nil:
		err = publicerror.Tag(loadErr)
		return

	case closeErr != nil:
		err = publicerror.Tag(closeErr)
		return
	}

	p.wasm = wasm.Bytes()

	realDigest := realHash.Sum(nil)
	p.hash = encoding.EncodeToString(realDigest)

	valid = validateHash(clientHash, realDigest)
	return
}

func validateHash(hash1 string, digest2 []byte) bool {
	digest1, err := encoding.DecodeString(hash1)
	if err == nil {
		return subtle.ConstantTimeCompare(digest1, digest2) == 1
	} else {
		return false
	}
}

func validateStringHash(hash1, hash2 string) bool {
	digest2, err := encoding.DecodeString(hash2)
	if err == nil {
		return validateHash(hash1, digest2)
	} else {
		return false
	}
}

func validateReadHash(hash1 string, r io.ReadCloser) (valid bool, err error) {
	hash2 := newHash()

	_, copyErr := io.Copy(hash2, r)
	closeErr := r.Close()
	switch {
	case copyErr != nil:
		err = publicerror.Tag(copyErr)
		return

	case closeErr != nil:
		err = publicerror.Tag(closeErr)
		return
	}

	valid = validateHash(hash1, hash2.Sum(nil))
	return
}

type Result struct {
	Status int
	Trap   trap.Id
	Err    error
}

type Instance struct {
	run        run.Instance
	exit       chan *Result
	originPipe *Pipe

	program *program // initialized and used only by State
}

func makeInstanceFactory(ctx context.Context, s *State) <-chan *Instance {
	channel := make(chan *Instance, s.PreforkProcs-1)

	go func() {
		defer func() {
			close(channel)

			for inst := range channel {
				inst.kill(s)
			}
		}()

		for {
			inst, err := newInstance(ctx, s)
			if err != nil {
				reportError(ctx, s, "instance allocation", "", 0, "", err)
				return
			}

			select {
			case channel <- inst:

			case <-ctx.Done():
				inst.kill(s)
				return
			}
		}
	}()

	return channel
}

func newInstance(ctx context.Context, s *State) (*Instance, error) {
	inst := new(Instance)

	if err := inst.run.Init(ctx, s.Runtime, s.Debug); err == nil {
		return inst, nil
	} else {
		return nil, err
	}
}

func (inst *Instance) kill(s *State) {
	inst.run.Kill(s.Runtime)
}

func (inst *Instance) populate(m *wag.Module, originPipe *Pipe, s *State) (err error) {
	_, memorySize := m.MemoryLimits()
	if memorySize > s.MemorySizeLimit {
		memorySize = s.MemorySizeLimit
	}

	err = inst.run.Populate(m, memorySize, s.StackSize)
	if err != nil {
		return
	}

	inst.exit = make(chan *Result, 1)
	inst.originPipe = originPipe
	return
}

func (inst *Instance) attachOrigin() (pipe *Pipe) {
	if inst.originPipe != nil && inst.originPipe.allocate() {
		pipe = inst.originPipe
	}
	return
}

func (inst *Instance) Run(ctx context.Context, progId string, instArg int32, instId string, r io.Reader, w io.Writer, s *State) {
	defer inst.kill(s)

	var (
		status int
		trapId trap.Id
		err    error
	)

	defer func() {
		var r *Result

		defer func() {
			defer close(inst.exit)
			inst.exit <- r
		}()

		if err != nil {
			return
		}

		r = new(Result)

		if trapId != 0 {
			r.Trap = trapId
		} else {
			r.Status = status
		}
	}()

	services := s.Services(&Server{
		Origin: Origin{
			R: r,
			W: w,
		},
	})

	inst.run.SetArg(instArg)

	status, trapId, err = inst.run.Run(ctx, s.Runtime, services)
	if err != nil {
		reportError(ctx, s, "run", progId, instArg, instId, err)
	}
}

type Pipe struct {
	lock     sync.Mutex
	in       *io.PipeWriter
	out      *io.PipeReader
	attached bool
}

func NewPipe() (inR *io.PipeReader, outW *io.PipeWriter, p *Pipe) {
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()

	p = &Pipe{
		in:  inW,
		out: outR,
	}
	return
}

func (p *Pipe) allocate() (ok bool) {
	p.lock.Lock()
	defer p.lock.Unlock()
	ok = !p.attached
	if ok {
		p.attached = true
	}
	return
}

func (p *Pipe) IO(in io.Reader, out io.Writer) {
	var (
		inDone  = make(chan struct{})
		outDone = make(chan struct{})
	)

	go func() {
		defer close(inDone)
		defer p.in.Close()
		io.Copy(p.in, in)
	}()

	go func() {
		defer close(outDone)
		io.Copy(out, p.out)
	}()

	<-inDone
	<-outDone
}

func (p *Pipe) DetachOrigin(ctx context.Context, instId string, s *State) {
	s.MonitorEvent(&event.InstanceDetach{
		Context:    Context(ctx),
		InstanceId: instId,
	}, nil)
}

func makeId() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		panic(err)
	}
	return encoding.EncodeToString(buf[:])
}

func reportError(ctx context.Context, s *State, subsystem, progId string, instArg int32, instId string, err error) {
	if puberr, ok := err.(publicerror.PublicError); ok {
		err = puberr.Cause()
		subsystem = puberr.Internal()
	}

	s.MonitorError(&detail.Position{
		Context:     Context(ctx),
		ProgramId:   progId,
		InstanceArg: instArg,
		InstanceId:  instId,
		Subsystem:   subsystem,
	}, err)
}
