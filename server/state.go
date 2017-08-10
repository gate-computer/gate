package server

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/binary"
	"io"
	"sync"

	"github.com/tsavola/gate/run"
	"github.com/tsavola/wag"
	"github.com/tsavola/wag/traps"
)

type Options struct {
	Env      *run.Environment
	Services func(io.Reader, io.Writer) run.ServiceRegistry
	Log      Logger
	Debug    io.Writer
}

type State struct {
	Settings
	Options

	instanceFactory <-chan *instance

	lock           sync.Mutex
	programsByHash map[[sha512.Size]byte]*program
	programs       map[uint64]*program
	instances      map[uint64]*instance
}

func NewState(ctx context.Context, settings Settings, opt Options) (s *State) {
	s = &State{
		Settings:       settings,
		Options:        opt,
		programsByHash: make(map[[sha512.Size]byte]*program),
		programs:       make(map[uint64]*program),
		instances:      make(map[uint64]*instance),
	}

	s.instanceFactory = makeInstanceFactory(ctx, s)
	return
}

func (s *State) getProgramByHash(hash []byte) (prog *program, found bool) {
	var key [sha512.Size]byte
	copy(key[:], hash)

	s.lock.Lock()
	prog, found = s.programsByHash[key]
	s.lock.Unlock()
	return
}

func (s *State) upload(wasm io.ReadCloser, clientHash []byte) (progId uint64, progHash []byte, valid bool, err error) {
	if len(clientHash) != sha512.Size {
		return
	}

	progId, progHash, valid, found, err := s.uploadKnown(wasm, clientHash)
	if err != nil {
		return
	}
	if found {
		return
	}

	progId, progHash, valid, err = s.uploadUnknown(wasm, clientHash)
	return
}

func (s *State) uploadKnown(wasm io.ReadCloser, clientHash []byte) (progId uint64, progHash []byte, valid, found bool, err error) {
	prog, found := s.getProgramByHash(clientHash)
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

	s.lock.Lock()
	prog.ownerCount++
	s.programs[progId] = prog
	s.lock.Unlock()

	progHash = prog.hash[:]
	return
}

func (s *State) uploadUnknown(wasm io.ReadCloser, clientHash []byte) (progId uint64, progHash []byte, valid bool, err error) {
	prog, valid, err := loadProgram(wasm, clientHash, s.Env)
	if err != nil {
		return
	}
	if !valid {
		return
	}

	progId = makeId()

	s.lock.Lock()
	if existing, found := s.programsByHash[prog.hash]; found {
		prog = existing
	} else {
		s.programsByHash[prog.hash] = prog
	}
	prog.ownerCount++
	s.programs[progId] = prog
	s.lock.Unlock()

	progHash = prog.hash[:]
	return
}

func (s *State) check(progId uint64, progHash []byte) (valid, found bool) {
	s.lock.Lock()
	prog, found := s.programs[progId]
	s.lock.Unlock()
	if !found {
		return
	}

	valid = validateHash(prog, progHash)
	return
}

func (s *State) uploadAndInstantiate(wasm io.ReadCloser, clientHash []byte, originPipe *pipe, cancel context.CancelFunc) (inst *instance, instId, progId uint64, progHash []byte, valid bool, err error) {
	if len(clientHash) != sha512.Size {
		return
	}

	inst = <-s.instanceFactory
	if inst == nil {
		err = context.Canceled
		return
	}

	instId, prog, progId, valid, found, err := s.uploadAndInstantiateKnown(wasm, clientHash, inst)
	if err != nil {
		inst.close()
		return
	}

	if !found {
		instId, prog, progId, valid, err = s.uploadAndInstantiateUnknown(wasm, clientHash, inst)
		if err != nil {
			inst.close()
			return
		}
	}

	err = inst.populate(&prog.module, &s.Settings, originPipe, cancel)
	if err != nil {
		inst.close()

		s.lock.Lock()
		delete(s.instances, instId)
		delete(s.programs, progId)
		prog.instanceCount--
		prog.ownerCount--
		s.lock.Unlock()
		return
	}

	progHash = prog.hash[:]
	return
}

func (s *State) uploadAndInstantiateKnown(wasm io.ReadCloser, clientHash []byte, inst *instance) (instId uint64, prog *program, progId uint64, valid, found bool, err error) {
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
	prog.ownerCount++
	prog.instanceCount++
	s.programs[progId] = prog
	inst.program = prog
	s.instances[instId] = inst
	s.lock.Unlock()
	return
}

func (s *State) uploadAndInstantiateUnknown(wasm io.ReadCloser, clientHash []byte, inst *instance) (instId uint64, prog *program, progId uint64, valid bool, err error) {
	prog, valid, err = loadProgram(wasm, clientHash, s.Env)
	if err != nil {
		return
	}

	progId = makeId()
	instId = makeId()

	s.lock.Lock()
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
	s.lock.Unlock()
	return
}

func (s *State) instantiate(progId uint64, progHash []byte, originPipe *pipe, cancel context.CancelFunc) (inst *instance, instId uint64, valid, found bool, err error) {
	s.lock.Lock()
	prog, found := s.programs[progId]
	if found {
		prog.instanceCount++
	}
	s.lock.Unlock()
	if !found {
		return
	}

	valid = validateHash(prog, progHash)
	if !valid {
		s.lock.Lock()
		prog.instanceCount--
		s.lock.Unlock()
		return
	}

	inst = <-s.instanceFactory
	if inst == nil {
		s.lock.Lock()
		prog.instanceCount--
		s.lock.Unlock()

		err = context.Canceled
		return
	}

	inst.program = prog

	err = inst.populate(&prog.module, &s.Settings, originPipe, cancel)
	if err != nil {
		inst.close()

		s.lock.Lock()
		prog.instanceCount--
		s.lock.Unlock()
		return
	}

	instId = makeId()

	s.lock.Lock()
	s.instances[instId] = inst
	s.lock.Unlock()
	return
}

func (s *State) attachOrigin(instId uint64) (pipe *pipe, found bool) {
	s.lock.Lock()
	inst, found := s.instances[instId]
	s.lock.Unlock()
	if !found {
		return
	}

	pipe = inst.attachOrigin()
	return
}

func (s *State) wait(instId uint64) (result *result, found bool) {
	s.lock.Lock()
	inst, found := s.instances[instId]
	s.lock.Unlock()
	if !found {
		return
	}

	result, found = s.waitInstance(inst, instId)
	return
}

func (s *State) waitInstance(inst *instance, instId uint64) (result *result, found bool) {
	result, found = <-inst.exit
	if !found {
		return
	}

	s.lock.Lock()
	delete(s.instances, instId)
	inst.program.instanceCount--
	s.lock.Unlock()
	return
}

func (s *State) Cancel() {
	s.lock.Lock()
	for _, inst := range s.instances {
		inst.cancel()
	}
	s.lock.Unlock()
}

type program struct {
	ownerCount    int
	instanceCount int
	module        wag.Module
	hash          [sha512.Size]byte
}

func loadProgram(body io.ReadCloser, clientHash []byte, env *run.Environment) (*program, bool, error) {
	hash := sha512.New()
	wasm := bufio.NewReader(io.TeeReader(body, hash))

	p := &program{
		module: wag.Module{
			MainSymbol: "main",
		},
	}

	loadErr := p.module.Load(wasm, env, new(bytes.Buffer), nil, run.RODataAddr, nil)
	closeErr := body.Close()
	switch {
	case loadErr != nil:
		return nil, false, loadErr

	case closeErr != nil:
		return nil, false, closeErr
	}

	hash.Sum(p.hash[:0])
	valid := validateHash(p, clientHash)
	return p, valid, nil
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

type result struct {
	status int
	trap   traps.Id
	err    error
}

type instance struct {
	payload    run.Payload
	process    run.Process
	exit       chan *result
	originPipe *pipe
	cancel     context.CancelFunc

	program *program // initialized and used only by State
}

func makeInstanceFactory(ctx context.Context, s *State) <-chan *instance {
	channel := make(chan *instance, s.preforkProcs()-1)

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

func newInstance(ctx context.Context, s *State) *instance {
	inst := new(instance)

	err := inst.payload.Init()
	if err != nil {
		s.Log.Printf("payload init: %v", err)
		return nil
	}

	err = inst.process.Init(ctx, s.Env, &inst.payload, s.Debug)
	if err != nil {
		s.Log.Printf("process init: %v", err)
		inst.payload.Close()
		return nil
	}

	return inst
}

func (inst *instance) close() {
	inst.payload.Close()
	inst.process.Close()
}

func (inst *instance) populate(m *wag.Module, s *Settings, originPipe *pipe, cancel context.CancelFunc) (err error) {
	_, memorySize := m.MemoryLimits()
	if limit := s.memorySizeLimit(); memorySize > limit {
		memorySize = limit
	}

	err = inst.payload.Populate(m, memorySize, s.stackSize())
	if err != nil {
		inst.close()
		return
	}

	inst.exit = make(chan *result, 1)
	inst.originPipe = originPipe
	inst.cancel = cancel
	return
}

func (inst *instance) attachOrigin() (pipe *pipe) {
	if inst.originPipe != nil && inst.originPipe.allocate() {
		pipe = inst.originPipe
	}
	return
}

func (inst *instance) run(ctx context.Context, opt *Options, r io.Reader, w io.Writer) {
	defer inst.close()

	var (
		status int
		trap   traps.Id
		err    error
	)

	defer func() {
		var r *result

		defer func() {
			defer close(inst.exit)
			inst.exit <- r
		}()

		if err != nil {
			return
		}

		r = new(result)

		if trap != 0 {
			r.trap = trap
		} else {
			r.status = status
		}
	}()

	status, trap, err = run.Run(ctx, opt.Env, &inst.process, &inst.payload, opt.Services(r, w))
	if err != nil {
		opt.Log.Printf("run error: %v", err)
	}
}

type pipe struct {
	lock     sync.Mutex
	in       *io.PipeWriter
	out      *io.PipeReader
	attached bool
}

func newPipe() (inR *io.PipeReader, outW *io.PipeWriter, p *pipe) {
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()

	p = &pipe{
		in:  inW,
		out: outR,
	}
	return
}

func (p *pipe) allocate() (ok bool) {
	p.lock.Lock()
	ok = !p.attached
	if ok {
		p.attached = true
	}
	p.lock.Unlock()
	return
}

func (p *pipe) io(in io.Reader, out io.Writer) {
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
