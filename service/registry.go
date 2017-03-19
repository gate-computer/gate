package service

import (
	"encoding/binary"
	"sync"

	"github.com/tsavola/gate/run"
)

type Instance interface {
	Message(op []byte)
	Shutdown()
}

type Registry struct {
	factories []func(chan<- []byte) Instance
	infos     map[string]run.ServiceInfo
}

func (r *Registry) Register(name string, version uint32, f func(evs chan<- []byte) Instance) {
	if r.infos == nil {
		r.infos = make(map[string]run.ServiceInfo)
	}

	r.factories = append(r.factories, f)
	atom := uint32(len(r.factories))
	r.infos[name] = run.ServiceInfo{Atom: atom, Version: version}
}

func (r *Registry) Info(name string) run.ServiceInfo {
	return r.infos[name]
}

func (r *Registry) Messenger(evs chan<- []byte) run.Messenger {
	return &messenger{
		factories: r.factories,
		instances: make(map[uint32]Instance),
		evs:       evs,
	}
}

type messenger struct {
	factories []func(chan<- []byte) Instance
	instances map[uint32]Instance
	evs       chan<- []byte
}

func (mr *messenger) Message(op []byte) (ok bool) {
	atom := binary.LittleEndian.Uint32(op[8:])
	inst, found := mr.instances[atom]
	if !found {
		index := atom - 1 // underflow wraps around
		if index >= uint32(len(mr.factories)) {
			return
		}
		inst = mr.factories[index](mr.evs)
	}
	inst.Message(op)
	ok = true
	return
}

func (mr *messenger) Shutdown() {
	var wg sync.WaitGroup

	shutdown := func(inst Instance) {
		defer wg.Done()
		inst.Shutdown()
	}

	for _, inst := range mr.instances {
		wg.Add(1)
		go shutdown(inst)
	}

	wg.Wait()
	close(mr.evs)
}
