// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"context"
	"sync"

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

type impl struct {
	factory Factory
	version int32
}

// Registry is the default run.ServiceRegistry implementation.  It multiplexes
// packets to service implementations.  If each program instance requires
// distinct configuration for a given service, a modified Registry with a
// distinct Factory instance must be used for each program instance.
type Registry struct {
	impls  map[string]impl
	parent *Registry
}

// Defaults gets populated with the built-in services if the service/defaults
// package is imported.
var Defaults = new(Registry)

// Register a service implementation.  See
// https://github.com/tsavola/gate/blob/master/Service.md for service naming
// conventions.  The version parameter may be used to communicate changes in
// the service API.
func (r *Registry) Register(name string, version int32, f Factory) {
	if r.impls == nil {
		r.impls = make(map[string]impl)
	}
	r.impls[name] = impl{f, version}
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
	return &Registry{
		parent: r,
	}
}

func (r *Registry) lookup(name string) (result impl) {
	for {
		var found bool

		result, found = r.impls[name]
		if found {
			return
		}

		r = r.parent
		if r == nil {
			return
		}
	}
}

// StartServing implements the run.ServiceRegistry interface function.
func (r *Registry) StartServing(ctx context.Context, ops <-chan packet.Buf, evs chan<- packet.Buf, maxContentSize int,
) run.ServiceDiscoverer {
	d := &discoverer{
		registry:   r,
		discovered: make(map[Factory]struct{}),
	}

	go serve(ctx, r, d, ops, evs, maxContentSize)

	return d
}

func serve(ctx context.Context, r *Registry, d *discoverer, ops <-chan packet.Buf, evs chan<- packet.Buf, maxContentSize int) {
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
				if f := d.getFactories()[code.Int16()]; f != nil {
					inst = f.Instantiate(code, &config)
				}
				instances[code] = inst
			}

			if inst != nil {
				inst.Handle(ctx, op, evs)
			}
			// TODO: packets to unknown services should be handled somehow

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
		if inst != nil {
			inst.Shutdown()
		}
	}
}

type discoverer struct {
	registry    *Registry
	discovered  map[Factory]struct{}
	services    []run.Service
	factoryLock sync.Mutex
	factories   []Factory
}

func (d *discoverer) Discover(newNames []string) []run.Service {
	oldCount := len(d.services)
	newCount := oldCount + len(newNames)

	if cap(d.services) < newCount {
		newServices := make([]run.Service, oldCount, newCount)
		copy(newServices, d.services)
		d.services = newServices
	}

	newFactories := make([]Factory, oldCount, newCount)
	copy(newFactories, d.factories)

	for _, name := range newNames {
		var (
			f Factory
			s run.Service
		)

		if impl := d.registry.lookup(name); impl.factory != nil {
			if _, dupe := d.discovered[impl.factory]; !dupe {
				d.discovered[impl.factory] = struct{}{}
				f = impl.factory
				s.SetAvailable(impl.version)
			}
		}

		newFactories = append(newFactories, f)
		d.services = append(d.services, s)
	}

	d.setFactories(newFactories)

	return d.services
}

func (d *discoverer) NumServices() int {
	return len(d.services)
}

func (d *discoverer) getFactories() []Factory {
	d.factoryLock.Lock()
	defer d.factoryLock.Unlock()

	return d.factories
}

func (d *discoverer) setFactories(factories []Factory) {
	d.factoryLock.Lock()
	defer d.factoryLock.Unlock()

	d.factories = factories
}
