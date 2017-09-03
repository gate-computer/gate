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
	"encoding/binary"
	"fmt"
	"io"
	"strconv"
	"sync"

	"github.com/tsavola/gate/run"
	config "github.com/tsavola/gate/server/serverconfig"
	"github.com/tsavola/wag"
	"github.com/tsavola/wag/traps"
	"github.com/tsavola/wag/wasm"
)

const (
	maxStackSize = 0x40000000 // crazy but valid
)

func FormatId(id uint64) (hex string) {
	hex = fmt.Sprintf("%016x", id)
	return
}

func ParseId(hex string) (id uint64, ok bool) {
	id, err := strconv.ParseUint(hex, 16, 64)
	ok = (err == nil)
	return
}

type State struct {
	config.Config

	instanceFactory <-chan *Instance

	lock           sync.Mutex
	programsByHash map[[sha512.Size]byte]*program
	programs       map[uint64]*program
	instances      map[uint64]*Instance
}

func (s *State) Init(ctx context.Context, conf config.Config) {
	if conf.MemorySizeLimit > 0 {
		conf.MemorySizeLimit = (conf.MemorySizeLimit + int(wasm.Page-1)) &^ int(wasm.Page-1)
	} else {
		conf.MemorySizeLimit = config.DefaultMemorySizeLimit
	}

	if conf.StackSize > maxStackSize {
		conf.StackSize = maxStackSize
	} else if conf.StackSize <= 0 {
		conf.StackSize = config.DefaultStackSize
	}

	if conf.PreforkProcs <= 0 {
		conf.PreforkProcs = config.DefaultPreforkProcs
	}

	s.Config = conf
	s.instanceFactory = makeInstanceFactory(ctx, s)
	s.programsByHash = make(map[[sha512.Size]byte]*program)
	s.programs = make(map[uint64]*program)
	s.instances = make(map[uint64]*Instance)
}

func (s *State) newInstance(ctx context.Context) (inst *Instance, err error) {
	select {
	case inst = <-s.instanceFactory:
		if inst == nil {
			err = context.Canceled
		}

	case <-ctx.Done():
		err = ctx.Err()
	}
	return
}

func (s *State) getProgram(progId uint64) (prog *program, found bool) {
	s.lock.Lock()
	defer s.lock.Unlock()
	prog, found = s.programs[progId]
	return
}

func (s *State) setProgramForOwner(progId uint64, prog *program) {
	s.lock.Lock()
	defer s.lock.Unlock()
	prog.ownerCount++
	s.programs[progId] = prog
}

func (s *State) getProgramForInstance(progId uint64) (prog *program, found bool) {
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

func (s *State) getProgramByHash(hash []byte) (prog *program, found bool) {
	var key [sha512.Size]byte
	copy(key[:], hash)

	s.lock.Lock()
	defer s.lock.Unlock()
	prog, found = s.programsByHash[key]
	return
}

func (s *State) getInstance(instId uint64) (inst *Instance, found bool) {
	s.lock.Lock()
	defer s.lock.Unlock()
	inst, found = s.instances[instId]
	return
}

func (s *State) setInstance(instId uint64, inst *Instance) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.instances[instId] = inst
}

func (s *State) Upload(wasm io.ReadCloser, clientHash []byte) (progId uint64, progHash []byte, valid bool, err error) {
	if len(clientHash) != sha512.Size {
		return
	}

	prog, progId, valid, found, err := s.uploadKnown(wasm, clientHash)
	if err != nil {
		return
	}

	if !found {
		prog, progId, valid, err = s.uploadUnknown(wasm, clientHash)
		if err != nil {
			return
		}
	}

	progHash = prog.hash[:]
	return
}

func (s *State) uploadKnown(wasm io.ReadCloser, clientHash []byte) (prog *program, progId uint64, valid, found bool, err error) {
	prog, found = s.getProgramByHash(clientHash)
	if !found {
		return
	}

	valid, err = validateReadHash(prog, wasm)
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

func (s *State) uploadUnknown(wasm io.ReadCloser, clientHash []byte) (prog *program, progId uint64, valid bool, err error) {
	prog, valid, err = loadProgram(wasm, clientHash, s.Runtime)
	if err != nil {
		return
	}
	if !valid {
		return
	}

	progId = makeId()

	s.lock.Lock()
	defer s.lock.Unlock()
	if existing, found := s.programsByHash[prog.hash]; found {
		prog = existing
	} else {
		s.programsByHash[prog.hash] = prog
	}
	prog.ownerCount++
	s.programs[progId] = prog
	return
}

func (s *State) Check(progId uint64, progHash []byte) (valid, found bool) {
	prog, found := s.getProgram(progId)
	if !found {
		return
	}

	valid = validateHash(prog, progHash)
	return
}

func (s *State) UploadAndInstantiate(ctx context.Context, wasm io.ReadCloser, clientHash []byte, originPipe *Pipe) (inst *Instance, instId, progId uint64, progHash []byte, valid bool, err error) {
	if len(clientHash) != sha512.Size {
		return
	}

	inst, err = s.newInstance(ctx)
	if err != nil {
		return
	}

	closeInst := true
	defer func() {
		if closeInst {
			inst.close()
		}
	}()

	instId, prog, progId, valid, found, err := s.uploadAndInstantiateKnown(wasm, clientHash, inst)
	if err != nil {
		return
	}

	if !found {
		instId, prog, progId, valid, err = s.uploadAndInstantiateUnknown(wasm, clientHash, inst)
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

	progHash = prog.hash[:]

	removeProgAndInst = false
	closeInst = false
	return
}

func (s *State) uploadAndInstantiateKnown(wasm io.ReadCloser, clientHash []byte, inst *Instance) (instId uint64, prog *program, progId uint64, valid, found bool, err error) {
	prog, found = s.getProgramByHash(clientHash)
	if !found {
		return
	}

	valid, err = validateReadHash(prog, wasm)
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
	prog.ownerCount++
	prog.instanceCount++
	s.programs[progId] = prog
	inst.program = prog
	s.instances[instId] = inst
	return
}

func (s *State) uploadAndInstantiateUnknown(wasm io.ReadCloser, clientHash []byte, inst *Instance) (instId uint64, prog *program, progId uint64, valid bool, err error) {
	prog, valid, err = loadProgram(wasm, clientHash, s.Runtime)
	if err != nil {
		return
	}

	progId = makeId()
	instId = makeId()

	s.lock.Lock()
	defer s.lock.Unlock()
	if existing, found := s.programsByHash[prog.hash]; found {
		prog = existing
	} else {
		s.programsByHash[prog.hash] = prog
	}
	prog.ownerCount++
	prog.instanceCount++
	s.programs[progId] = prog
	inst.program = prog
	s.instances[instId] = inst
	return
}

func (s *State) Instantiate(ctx context.Context, progId uint64, progHash []byte, originPipe *Pipe) (inst *Instance, instId uint64, valid, found bool, err error) {
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

	valid = validateHash(prog, progHash)
	if !valid {
		return
	}

	inst, err = s.newInstance(ctx)
	if err != nil {
		return
	}

	closeInst := true
	defer func() {
		if closeInst {
			inst.close()
		}
	}()

	inst.program = prog
	err = inst.populate(&prog.module, originPipe, s)
	if err != nil {
		return
	}

	instId = makeId()
	s.setInstance(instId, inst)

	closeInst = false
	unrefProg = false
	return
}

func (s *State) AttachOrigin(instId uint64) (pipe *Pipe, found bool) {
	inst, found := s.getInstance(instId)
	if !found {
		return
	}

	pipe = inst.attachOrigin()
	return
}

func (s *State) Wait(instId uint64) (result *Result, found bool) {
	inst, found := s.getInstance(instId)
	if !found {
		return
	}

	result, found = s.WaitInstance(inst, instId)
	return
}

func (s *State) WaitInstance(inst *Instance, instId uint64) (result *Result, found bool) {
	result, found = <-inst.exit
	if !found {
		return
	}

	s.lock.Lock()
	defer s.lock.Unlock()
	delete(s.instances, instId)
	inst.program.instanceCount--
	return
}

type program struct {
	ownerCount    int
	instanceCount int
	module        wag.Module
	wasm          []byte
	hash          [sha512.Size]byte
}

func loadProgram(body io.ReadCloser, clientHash []byte, rt *run.Runtime) (p *program, valid bool, err error) {
	var (
		wasm bytes.Buffer
		hash = sha512.New()
	)

	r := bufio.NewReader(io.TeeReader(io.TeeReader(body, &wasm), hash))

	p = &program{
		module: wag.Module{
			MainSymbol: "main",
		},
	}

	loadErr := p.module.Load(r, rt, new(bytes.Buffer), nil, run.RODataAddr, nil)
	closeErr := body.Close()
	switch {
	case loadErr != nil:
		err = loadErr
		return

	case closeErr != nil:
		err = closeErr
		return
	}

	p.wasm = wasm.Bytes()

	hash.Sum(p.hash[:0])
	valid = validateHash(p, clientHash)
	return
}

func validateHash(p *program, hash []byte) bool {
	return subtle.ConstantTimeCompare(hash, p.hash[:]) == 1
}

func validateReadHash(p *program, r io.ReadCloser) (valid bool, err error) {
	h := sha512.New()

	_, err = io.Copy(h, r)
	r.Close()
	if err != nil {
		return
	}

	valid = validateHash(p, h.Sum(nil))
	return
}

type Result struct {
	Status int
	Trap   traps.Id
	Err    error
}

type Instance struct {
	payload    run.Payload
	process    run.Process
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
				inst.close()
			}
		}()

		for {
			inst := newInstance(ctx, s)
			if inst == nil {
				return
			}

			select {
			case channel <- inst:

			case <-ctx.Done():
				inst.close()
				return
			}
		}
	}()

	return channel
}

func newInstance(ctx context.Context, s *State) *Instance {
	inst := new(Instance)

	if err := inst.payload.Init(); err != nil {
		s.Log.Printf("payload init: %v", err)
		return nil
	}

	closePayload := true
	defer func() {
		if closePayload {
			inst.payload.Close()
		}
	}()

	if err := inst.process.Init(ctx, s.Runtime, &inst.payload, s.Debug); err != nil {
		s.Log.Printf("process init: %v", err)
		return nil
	}

	closePayload = false
	return inst
}

func (inst *Instance) close() {
	inst.payload.Close()
	inst.process.Close()
}

func (inst *Instance) populate(m *wag.Module, originPipe *Pipe, s *State) (err error) {
	_, memorySize := m.MemoryLimits()
	if limit := wasm.MemorySize(s.MemorySizeLimit); memorySize > limit {
		memorySize = limit
	}

	err = inst.payload.Populate(m, memorySize, int32(s.StackSize))
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

func (inst *Instance) Run(ctx context.Context, s *State, arg int32, r io.Reader, w io.Writer) {
	defer inst.close()

	var (
		status int
		trap   traps.Id
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

		if trap != 0 {
			r.Trap = trap
		} else {
			r.Status = status
		}
	}()

	services := s.Services(&config.Server{
		Origin: config.Origin{
			R: r,
			W: w,
		},
	})

	inst.payload.SetArg(arg)

	status, trap, err = run.Run(ctx, s.Runtime, &inst.process, &inst.payload, services)
	if err != nil {
		s.Log.Printf("run error: %v", err)
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

func makeId() (id uint64) {
	if err := binary.Read(rand.Reader, binary.LittleEndian, &id); err != nil {
		panic(err)
	}
	return
}
