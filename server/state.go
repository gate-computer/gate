package server

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/binary"
	"io"
	"sync"

	"github.com/tsavola/wag"
	"github.com/tsavola/wag/traps"

	"github.com/tsavola/gate"
	"github.com/tsavola/gate/run"
)

func makeId() (id uint64) {
	if err := binary.Read(rand.Reader, binary.LittleEndian, &id); err != nil {
		panic(err)
	}
	return
}

type State struct {
	Settings

	lock           sync.Mutex
	programsByHash map[[sha512.Size]byte]*program
	programs       map[uint64]*program
	instances      map[uint64]*instance
}

func NewState(s Settings) *State {
	return &State{
		Settings:       s,
		programsByHash: make(map[[sha512.Size]byte]*program),
		programs:       make(map[uint64]*program),
		instances:      make(map[uint64]*instance),
	}
}

func (s *State) upload(wasm io.ReadCloser, clientHash []byte) (progId uint64, progHash []byte, valid bool, err error) {
	if clientHash != nil {
		var final bool

		progId, progHash, valid, final, err = s.uploadPossiblyKnown(wasm, clientHash)
		if err != nil {
			return
		}
		if final {
			return
		}
	}

	return s.uploadUnknown(wasm, clientHash)
}

func (s *State) uploadPossiblyKnown(wasm io.ReadCloser, clientHash []byte) (progId uint64, progHash []byte, valid, final bool, err error) {
	if len(clientHash) != sha512.Size {
		final = true // avoid processing impossible input
		return
	}

	var key [sha512.Size]byte
	copy(key[:], clientHash)

	s.lock.Lock()
	prog, final := s.programsByHash[key]
	s.lock.Unlock()
	if !final {
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

func (s *State) uploadAndInstantiate(wasm io.ReadCloser, exit chan *gate.Result, originPipe *pipe) (inst *instance, instId, progId uint64, progHash []byte, err error) {
	// TODO: clientHash support

	prog, _, err := loadProgram(wasm, nil, s.Env)
	if err != nil {
		return
	}

	progId = makeId()
	instId = makeId()
	inst = newInstance(exit, originPipe)

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

	progHash = prog.hash[:]
	return
}

func (s *State) instantiate(progId uint64, progHash []byte, exit chan *gate.Result, originPipe *pipe) (inst *instance, instId uint64, valid, found bool, err error) {
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
		prog.instanceCount-- // cancel
		s.lock.Unlock()
		return
	}

	instId = makeId()
	inst = newInstance(exit, originPipe)
	inst.program = prog

	s.lock.Lock()
	s.instances[instId] = inst
	s.lock.Unlock()
	return
}

func (s *State) cancel(inst *instance, instId uint64) {
	s.lock.Lock()
	delete(s.instances, instId)
	inst.program.instanceCount--
	s.lock.Unlock()

	inst.cancel()
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

func (s *State) wait(instId uint64) (result *gate.Result, found bool) {
	s.lock.Lock()
	inst, found := s.instances[instId]
	s.lock.Unlock()
	if !found {
		return
	}

	result, found = inst.wait()

	s.lock.Lock()
	delete(s.instances, instId)
	inst.program.instanceCount--
	s.lock.Unlock()
	return
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

	var valid bool
	if clientHash == nil {
		valid = true
	} else {
		valid = validateHash(p, clientHash)
	}

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

type instance struct {
	program    *program
	exit       chan *gate.Result
	originPipe *pipe
}

// newInstance does not set the program field; it must be initialized manually.
func newInstance(exit chan *gate.Result, originPipe *pipe) *instance {
	if exit == nil {
		exit = make(chan *gate.Result, 1)
	}

	return &instance{
		exit:       exit,
		originPipe: originPipe,
	}
}

func (inst *instance) cancel() {
	close(inst.exit)
}

func (inst *instance) attachOrigin() (pipe *pipe) {
	if inst.originPipe != nil && inst.originPipe.allocate() {
		pipe = inst.originPipe
	}
	return
}

func (inst *instance) wait() (result *gate.Result, found bool) {
	result, found = <-inst.exit
	return
}

func (inst *instance) run(s *Settings, r io.Reader, w io.Writer) {
	var (
		exit     int
		trap     traps.Id
		err      error
		internal bool
	)

	defer func() {
		var r *gate.Result

		defer func() {
			defer close(inst.exit)
			inst.exit <- r
		}()

		if err != nil && internal {
			return
		}

		r = new(gate.Result)

		switch {
		case err != nil:
			r.Error = err.Error()

		case trap != 0:
			r.Trap = trap.String()

		default:
			r.Exit = exit
		}
	}()

	_, memorySize := inst.program.module.MemoryLimits()
	if memorySize > s.MemorySizeLimit {
		memorySize = s.MemorySizeLimit
	}

	payload, err := run.NewPayload(&inst.program.module, memorySize, s.StackSize)
	if err != nil {
		return
	}
	defer payload.Close()

	internal = true

	exit, trap, err = run.Run(s.Env, payload, s.Services(r, w), s.Debug)
	if err != nil {
		s.Log.Printf("run error: %v", err)
		return
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
