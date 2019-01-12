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
}

// Instance of a service.  Corresponds to a program instance.
type Instance interface {
	Handle(ctx context.Context, send chan<- packet.Buf, in packet.Buf)
	Shutdown(ctx context.Context)
}

// Factory creates instances of a particular service implementation.
//
// See https://github.com/tsavola/gate/blob/master/Service.md for service
// naming conventions.
type Factory interface {
	ServiceName() string
	Instantiate(*InstanceConfig, packet.Code) Instance
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
func (r *Registry) StartServing(ctx context.Context, serviceConfig *runtime.ServiceConfig, send chan<- packet.Buf, recv <-chan packet.Buf) runtime.ServiceDiscoverer {
	d := &discoverer{
		registry:   r,
		discovered: make(map[Factory]struct{}),
	}
	go serve(ctx, serviceConfig, r, d, send, recv)
	return d
}

func serve(ctx context.Context, serviceConfig *runtime.ServiceConfig, r *Registry, d *discoverer, send chan<- packet.Buf, recv <-chan packet.Buf) {
	config := &InstanceConfig{
		ServiceConfig: *serviceConfig,
	}

	instances := make(map[packet.Code]Instance)
	defer shutdown(ctx, instances)

	for op := range recv {
		code := op.Code()

		inst, found := instances[code]
		if !found {
			if f := d.getFactories()[code]; f != nil {
				inst = f.Instantiate(config, code)
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

func shutdown(ctx context.Context, instances map[packet.Code]Instance) {
	// Actual shutdown of a service should have began when the context was
	// canceled, so these Shutdown calls only need to wait for completion.
	for _, inst := range instances {
		if inst != nil {
			inst.Shutdown(ctx)
		}
	}
}

type discoverer struct {
	registry    *Registry
	discovered  map[Factory]struct{}
	services    []runtime.ServiceState
	factoryLock sync.Mutex
	factories   []Factory
}

func (d *discoverer) Discover(newNames []string) []runtime.ServiceState {
	oldCount := len(d.services)
	newCount := oldCount + len(newNames)

	if cap(d.services) < newCount {
		newServices := make([]runtime.ServiceState, oldCount, newCount)
		copy(newServices, d.services)
		d.services = newServices
	}

	newFactories := make([]Factory, oldCount, newCount)
	copy(newFactories, d.factories)

	for _, name := range newNames {
		var s runtime.ServiceState

		f := d.registry.lookup(name)
		if f != nil {
			if _, dupe := d.discovered[f]; dupe {
				f = nil
			} else {
				d.discovered[f] = struct{}{}
				s.SetAvail()
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
