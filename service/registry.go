// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"context"
	"encoding/binary"
	"errors"

	"github.com/tsavola/gate/packet"
	"github.com/tsavola/gate/run"
)

// Config of the runtime.
type Config struct {
	MaxContentSize int
}

// Instance of a service.
type Instance interface {
	Handle(ctx context.Context, op packet.Buf, evs chan<- packet.Buf)
	Shutdown()
}

// Factory creates instances of a particular service implementation.
type Factory interface {
	Instantiate(packet.Code, *Config) Instance
}

// FactoryFunc is almost like Factory.
type FactoryFunc func(packet.Code, *Config) Instance

func (f FactoryFunc) Instantiate(code packet.Code, config *Config) Instance {
	return f(code, config)
}

// Registry is the default run.ServiceRegistry implementation.  It multiplexes
// packets to service implementations.  If each program instance requires
// distinct configuration for a given service, a modified Registry with a
// distinct Factory instance must be used for each program instance.
type Registry struct {
	factories []Factory
	infos     map[string]run.ServiceInfo
}

// Defaults gets populated with the built-in services if the service/defaults
// package is imported.
var Defaults = new(Registry)

// Register a service implementation.  See
// https://github.com/tsavola/gate/blob/master/Service.md for service naming
// conventions.  The version parameter may be used to communicate changes in
// the service API.
func (r *Registry) Register(name string, version int32, f Factory) {
	if r.infos == nil {
		r.infos = make(map[string]run.ServiceInfo)
	}

	var code packet.Code

	if info, found := r.infos[name]; found {
		code = info.Code
		r.factories[code.Int()-1] = f
	} else {
		r.factories = append(r.factories, f)
		binary.LittleEndian.PutUint16(code[:], uint16(len(r.factories)))
	}

	r.infos[name] = run.ServiceInfo{Code: code, Version: version}
}

// RegisterFunc is almost like Register.
func (r *Registry) RegisterFunc(
	name string,
	version int32,
	f func(packet.Code, *Config) Instance,
) {
	r.Register(name, version, FactoryFunc(f))
}

// Clone the registry.  The new registry may be used to add or replace some
// service implementations.
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

// Info implements the run.ServiceRegistry interface function.
func (r *Registry) Info(name string) run.ServiceInfo {
	return r.infos[name]
}

// Serve implements the run.ServiceRegistry interface function.
func (r *Registry) Serve(ctx context.Context, ops <-chan packet.Buf, evs chan<- packet.Buf, maxContentSize int,
) (err error) {
	defer close(evs)

	config := Config{
		MaxContentSize: maxContentSize,
	}

	instances := make(map[packet.Code]Instance)
	defer shutdown(instances)
	defer flush(ops)

	for {
		select {
		case op, ok := <-ops:
			if !ok {
				return
			}

			var code packet.Code
			copy(code[:], op[packet.CodeOffset:])
			inst, found := instances[code]
			if !found {
				index := uint32(code.Int()) - 1 // underflow wraps around
				if index >= uint32(len(r.factories)) {
					err = errors.New("invalid service code")
					return
				}
				inst = r.factories[index].Instantiate(code, &config)
				instances[code] = inst
			}
			inst.Handle(ctx, op, evs)

		case <-ctx.Done():
			return
		}
	}
}

func flush(ops <-chan packet.Buf) {
	for range ops {
	}
}

func shutdown(instances map[packet.Code]Instance) {
	// Actual shutdown of a service should have began when the context was
	// canceled, so these Shutdown calls only need to wait for completion.
	for _, inst := range instances {
		inst.Shutdown()
	}
}
