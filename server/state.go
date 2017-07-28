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

	lock      sync.Mutex
	programs  map[uint64]*program
	instances map[uint64]*instance
}

func NewState(s Settings) *State {
	return &State{
		Settings:  s,
		programs:  make(map[uint64]*program),
		instances: make(map[uint64]*instance),
	}
}

func (s *State) upload(wasm io.ReadCloser) (progId uint64, progHash []byte, err error) {
	prog, err := loadProgram(wasm, s.Env)
	if err != nil {
		return
	}

	progHash = prog.hash[:]
	progId = makeId()

	s.lock.Lock()
	s.programs[progId] = prog
	s.lock.Unlock()
	return
}

func (s *State) check(progId uint64, progHash []byte) (found, valid bool) {
	s.lock.Lock()
	prog, found := s.programs[progId]
	s.lock.Unlock()
	if !found {
		return
	}

	valid = prog.checkHash(progHash)
	return
}

func (s *State) uploadAndInstantiate(wasm io.ReadCloser, exit chan *gate.Result, origin *pipe) (inst *instance, instId, progId uint64, progHash []byte, err error) {
	prog, err := loadProgram(wasm, s.Env)
	if err != nil {
		return
	}

	progHash = prog.hash[:]
	progId = makeId()
	instId = makeId()
	inst = newInstance(&prog.module, exit, origin)

	s.lock.Lock()
	s.programs[progId] = prog
	s.instances[instId] = inst
	s.lock.Unlock()
	return
}

func (s *State) instantiate(progId uint64, progHash []byte, exit chan *gate.Result, origin *pipe) (inst *instance, instId uint64, found, valid bool, err error) {
	s.lock.Lock()
	prog, found := s.programs[progId]
	s.lock.Unlock()
	if !found {
		return
	}

	valid = prog.checkHash(progHash)
	if !valid {
		return
	}

	instId = makeId()
	inst = newInstance(&prog.module, exit, origin)

	s.lock.Lock()
	s.instances[instId] = inst
	s.lock.Unlock()
	return
}

func (s *State) cancel(inst *instance, instId uint64) {
	s.lock.Lock()
	delete(s.instances, instId)
	s.lock.Unlock()

	inst.cancel()
}

func (s *State) attachOrigin(instId uint64, r io.Reader, w io.Writer) (found, ok bool) {
	s.lock.Lock()
	inst, found := s.instances[instId]
	s.lock.Unlock()
	if !found {
		return
	}

	ok = inst.attachOrigin(r, w, s.Log)
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
	s.lock.Unlock()
	return
}

type program struct {
	module wag.Module
	hash   [sha512.Size]byte
}

func loadProgram(body io.ReadCloser, env *run.Environment) (*program, error) {
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
		return nil, loadErr

	case closeErr != nil:
		return nil, closeErr
	}

	hash.Sum(p.hash[:0])
	return p, nil
}

func (p *program) checkHash(hash []byte) bool {
	return subtle.ConstantTimeCompare(hash, p.hash[:]) == 1
}

type instance struct {
	module     *wag.Module
	exit       chan *gate.Result
	originPipe *pipe
}

func newInstance(m *wag.Module, exit chan *gate.Result, originPipe *pipe) *instance {
	if exit == nil {
		exit = make(chan *gate.Result, 1)
	}

	return &instance{
		module:     m,
		exit:       exit,
		originPipe: originPipe,
	}
}

func (inst *instance) cancel() {
	close(inst.exit)
}

func (inst *instance) attachOrigin(r io.Reader, w io.Writer, log Logger) (ok bool) {
	if inst.originPipe != nil {
		ok = inst.originPipe.attach(r, w, log)
	}
	return
}

func (inst *instance) wait() (result *gate.Result, found bool) {
	result, found = <-inst.exit
	return
}

func (inst *instance) run(s *Settings, r io.Reader, w io.WriteCloser) {
	s.Log.Printf("instance run...")
	defer s.Log.Printf("instance runned")

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

	defer w.Close()

	_, memorySize := inst.module.MemoryLimits()
	if memorySize > s.MemorySizeLimit {
		memorySize = s.MemorySizeLimit
	}

	payload, err := run.NewPayload(inst.module, memorySize, s.StackSize)
	if err != nil {
		s.Log.Printf("instance run: payload: %v", err)
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

func newPipe() (io.Reader, io.WriteCloser, *pipe) {
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()

	return inR, outW, &pipe{
		in:  inW,
		out: outR,
	}
}

func (socket *pipe) attach(in io.Reader, out io.Writer, log Logger) (ok bool) {
	socket.lock.Lock()
	ok = !socket.attached
	if ok {
		socket.attached = true
	}
	socket.lock.Unlock()
	if !ok {
		return
	}

	var (
		inDone  = make(chan struct{})
		outDone = make(chan struct{})
	)

	go func() {
		defer close(inDone)
		defer socket.in.Close()

		if n, err := io.Copy(socket.in, in); err != nil {
			log.Printf("origin input: %v", err)
		} else {
			log.Printf("origin input: %d bytes", n)
		}
	}()

	go func() {
		defer close(outDone)

		if n, err := io.Copy(out, socket.out); err != nil {
			log.Printf("origin output: %v", err)
		} else {
			log.Printf("origin output: %d bytes", n)
		}
	}()

	<-inDone
	<-outDone
	return
}
