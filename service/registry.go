package service

import (
	"encoding/binary"
	"errors"
	"sync"

	"github.com/tsavola/gate/run"
)

type Instance interface {
	Handle(op []byte, evs chan<- []byte)
	Shutdown()
}

type Factory interface {
	New() Instance
}

type FactoryFunc func() Instance

func (f FactoryFunc) New() Instance {
	return f()
}

type Registry struct {
	factories []Factory
	infos     map[string]run.ServiceInfo
}

func (r *Registry) Register(name string, version uint32, f Factory) {
	if r.infos == nil {
		r.infos = make(map[string]run.ServiceInfo)
	}

	r.factories = append(r.factories, f)
	atom := uint32(len(r.factories))
	r.infos[name] = run.ServiceInfo{Atom: atom, Version: version}
}

func (r *Registry) RegisterFunc(name string, version uint32, f func() Instance) {
	r.Register(name, version, FactoryFunc(f))
}

func (r *Registry) Info(name string) run.ServiceInfo {
	return r.infos[name]
}

func (r *Registry) Serve(ops <-chan []byte, evs chan<- []byte) (err error) {
	defer close(evs)

	instances := make(map[uint32]Instance)
	defer shutdown(instances)

	for op := range ops {
		atom := binary.LittleEndian.Uint32(op[8:])
		inst, found := instances[atom]
		if !found {
			index := atom - 1 // underflow wraps around
			if index >= uint32(len(r.factories)) {
				err = errors.New("invalid service atom")
				return
			}
			inst = r.factories[index].New()
			instances[atom] = inst
		}
		inst.Handle(op, evs)
	}

	return
}

func shutdown(instances map[uint32]Instance) {
	var wg sync.WaitGroup
	defer wg.Wait()

	shutdown := func(inst Instance) {
		defer wg.Done()
		inst.Shutdown()
	}

	for _, inst := range instances {
		wg.Add(1)
		go shutdown(inst)
	}
}
