// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"bytes"
	"context"
	"sync"

	"github.com/tsavola/gate/packet"
	"github.com/tsavola/gate/runtime"
)

const maxServiceNameLen = 127

// Config for service initialization.
type Config struct {
	Registry *Registry
}

// InstanceConfig for a service instance.
type InstanceConfig struct {
	runtime.ServiceConfig
	Code packet.Code
}

// Instance of a service.  Corresponds to a program instance.
type Instance interface {
	Handle(ctx context.Context, send chan<- packet.Buf, in packet.Buf)
	Shutdown(ctx context.Context) (finalState []byte)
}

// Factory creates instances of a particular service implementation.
//
// See https://github.com/tsavola/gate/blob/master/Service.md for service
// naming conventions.
type Factory interface {
	ServiceName() string
	Instantiate(config InstanceConfig, initialState []byte) Instance
}

// Registry is a runtime.ServiceRegistry implementation.  It multiplexes
// packets to service implementations.  If each program instance requires
// distinct configuration for a given service, a cloned registry with distinct
// Factory must be used.
type Registry struct {
	factories map[string]Factory
	parent    *Registry
}

// Register a service implementation.
func (r *Registry) Register(f Factory) {
	name := f.ServiceName()
	if len(name) > maxServiceNameLen {
		panic("service name is too long")
	}
	if bytes.Contains([]byte(name), []byte{0}) {
		panic("service name contains nul byte")
	}

	if r.factories == nil {
		r.factories = make(map[string]Factory)
	}
	r.factories[name] = f
}

// Clone the registry shallowly.  The new registry may be used to add or
// replace services without affecting the original registry.
func (r *Registry) Clone() *Registry {
	return &Registry{
		parent: r,
	}
}

func (r *Registry) lookup(name string) (result Factory) {
	for {
		var found bool

		result, found = r.factories[name]
		if found {
			return
		}

		r = r.parent
		if r == nil {
			return
		}
	}
}

// StartServing implements the runtime.ServiceRegistry interface function.
func (r *Registry) StartServing(ctx context.Context, serviceConfig runtime.ServiceConfig, initial []runtime.SuspendedService, send chan<- packet.Buf, recv <-chan packet.Buf,
) (runtime.ServiceDiscoverer, []runtime.ServiceState, error) {
	d := &discoverer{
		registry:   r,
		discovered: make(map[Factory]struct{}),
		stopped:    make(chan []runtime.SuspendedService, 1),
		services:   make([]serviceState, len(initial)),
	}

	for i, is := range initial {
		d.services[i].SuspendedService = is

		if f := d.registry.lookup(is.Name); f != nil {
			if _, dupe := d.discovered[f]; dupe {
				return nil, nil, runtime.ErrDuplicateService
			}

			d.discovered[f] = struct{}{}
			d.services[i].factory = f
		}
	}

	instances := make(map[packet.Code]Instance)
	states := make([]runtime.ServiceState, len(initial))

	for i, s := range d.services {
		if s.factory != nil {
			code := packet.Code(i)
			instances[code] = s.factory.Instantiate(InstanceConfig{serviceConfig, code}, s.Buffer)
			s.Buffer = nil

			states[i].SetAvail()
		}
	}

	go serve(ctx, serviceConfig, r, d, instances, send, recv)

	return d, states, nil
}

func serve(ctx context.Context, serviceConfig runtime.ServiceConfig, r *Registry, d *discoverer, instances map[packet.Code]Instance, send chan<- packet.Buf, recv <-chan packet.Buf) {
	defer d.shutdown(ctx, instances)

	for op := range recv {
		code := op.Code()

		inst, found := instances[code]
		if !found {
			if s := d.getServices()[code]; s.factory != nil {
				inst = s.factory.Instantiate(InstanceConfig{serviceConfig, code}, nil)
			}
			instances[code] = inst
		}

		if inst != nil {
			inst.Handle(ctx, send, op)
		} else {
			// TODO: service unavailable: buffer up to max packet size
		}
	}
}

type serviceState struct {
	runtime.SuspendedService
	factory Factory
}

type discoverer struct {
	registry   *Registry
	discovered map[Factory]struct{}
	stopped    chan []runtime.SuspendedService
	lock       sync.Mutex
	services   []serviceState
}

func (d *discoverer) getServices() []serviceState {
	d.lock.Lock()
	defer d.lock.Unlock()

	return d.services
}

func (d *discoverer) setServices(services []serviceState) {
	d.lock.Lock()
	defer d.lock.Unlock()

	d.services = services
}

func (d *discoverer) Discover(newNames []string) (states []runtime.ServiceState, err error) {
	oldCount := len(d.services)
	newCount := oldCount + len(newNames)

	newServices := make([]serviceState, oldCount, newCount)
	copy(newServices, d.services)

	for _, name := range newNames {
		var s serviceState

		if f := d.registry.lookup(name); f != nil {
			if _, dupe := d.discovered[f]; dupe {
				err = runtime.ErrDuplicateService
				return
			}

			d.discovered[f] = struct{}{}
			s.factory = f
		}

		newServices = append(newServices, s)
	}

	d.setServices(newServices)

	states = make([]runtime.ServiceState, len(newServices))
	for i, s := range newServices {
		if s.factory != nil {
			states[i].SetAvail()
		}
	}
	return
}

func (d *discoverer) NumServices() int {
	return len(d.services)
}

func (d *discoverer) shutdown(ctx context.Context, instances map[packet.Code]Instance) {
	// Actual shutdown of a service should have began when the context was
	// canceled, so these Shutdown calls only need to wait for completion.

	final := make([]runtime.SuspendedService, len(d.services))

	for i, s := range d.services {
		final[i] = s.SuspendedService
	}

	for code, inst := range instances {
		if inst != nil {
			if b := inst.Shutdown(ctx); len(b) > 0 {
				final[code].Buffer = b
			}
		}
	}

	d.stopped <- final
}

func (d *discoverer) Suspend() []runtime.SuspendedService {
	return <-d.stopped
}

func (d *discoverer) Close() error {
	return nil
}
