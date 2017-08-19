// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

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

func (r *Registry) Register(name string, version int32, f Factory) {
	if r.infos == nil {
		r.infos = make(map[string]run.ServiceInfo)
	}

	var code uint16

	if info, found := r.infos[name]; found {
		code = info.Code
		r.factories[code-1] = f
	} else {
		r.factories = append(r.factories, f)
		code = uint16(len(r.factories))
	}

	r.infos[name] = run.ServiceInfo{Code: code, Version: version}
}

func (r *Registry) RegisterFunc(name string, version int32, f func() Instance) {
	r.Register(name, version, FactoryFunc(f))
}

func (r *Registry) Clone() *Registry {
	clone := new(Registry)

	clone.factories = make([]Factory, len(r.factories))
	copy(clone.factories, r.factories)

	clone.infos = make(map[string]run.ServiceInfo)
	for k, v := range r.infos {
		clone.infos[k] = v
	}

	return clone
}

func (r *Registry) Info(name string) run.ServiceInfo {
	return r.infos[name]
}

func (r *Registry) Serve(ops <-chan []byte, evs chan<- []byte) (err error) {
	defer close(evs)

	instances := make(map[uint16]Instance)
	defer shutdown(instances)

	for op := range ops {
		code := binary.LittleEndian.Uint16(op[6:])
		inst, found := instances[code]
		if !found {
			index := uint32(code) - 1 // underflow wraps around
			if index >= uint32(len(r.factories)) {
				err = errors.New("invalid service code")
				return
			}
			inst = r.factories[index].New()
			instances[code] = inst
		}
		inst.Handle(op, evs)
	}

	return
}

func shutdown(instances map[uint16]Instance) {
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
