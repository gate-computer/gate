// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"context"
	"fmt"
	"sync"
	"unicode"

	"gate.computer/gate/packet"
	"gate.computer/gate/runtime"
	"gate.computer/gate/snapshot"
)

const maxServiceStringLen = 127

// Service metadata.
type Service struct {
	Name     string `json:"name"`
	Revision string `json:"revision"`
}

// InstanceConfig for a service instance.
type InstanceConfig struct {
	packet.Service
}

// Instance of a service.  Corresponds to a program instance.
type Instance interface {
	Resume(ctx context.Context, send chan<- packet.Buf)
	Handle(ctx context.Context, send chan<- packet.Buf, received packet.Buf)
	Suspend(ctx context.Context) (snapshot []byte)
	Shutdown(ctx context.Context)
}

// Factory creates instances of a particular service implementation.
//
// See https://gate.computer/gate/blob/master/Service.md for service naming
// conventions.
type Factory interface {
	ServiceName() string
	ServiceRevision() string
	Discoverable(ctx context.Context) bool
	CreateInstance(ctx context.Context, config InstanceConfig) Instance
	RestoreInstance(ctx context.Context, config InstanceConfig, snapshot []byte) (Instance, error)
}

// Registry is a runtime.ServiceRegistry implementation.  It multiplexes
// packets to service implementations.
type Registry struct {
	factories map[string]Factory
	parent    *Registry
}

// Register a service implementation.
func (r *Registry) Register(f Factory) {
	checkString("name", f.ServiceName(), unicode.IsLetter, unicode.IsNumber, unicode.IsPunct)
	checkString("revision", f.ServiceRevision(), unicode.IsLetter, unicode.IsMark, unicode.IsNumber, unicode.IsPunct, unicode.IsSymbol)

	if r.factories == nil {
		r.factories = make(map[string]Factory)
	}
	r.factories[f.ServiceName()] = f
}

func checkString(what, s string, categories ...func(rune) bool) {
	if len(s) == 0 {
		panic(fmt.Sprintf("service %s string is empty", what))
	}
	if len(s) > maxServiceStringLen {
		panic(fmt.Sprintf("service %s string is too long", what))
	}

	for _, r := range s {
		for _, f := range categories {
			if f(r) {
				goto ok
			}
		}
		panic(fmt.Sprintf("service %s string contains invalid characters", what))
	ok:
	}
}

// Clone the registry shallowly.  The new registry may be used to add or
// replace services without affecting the original registry.
func (r *Registry) Clone() *Registry {
	return &Registry{
		parent: r,
	}
}

// Catalog of service metadata.  Only the services which are discoverable in
// this context are included.
func (r *Registry) Catalog(ctx context.Context) (services []Service) {
	m := make(map[string]string)
	r.catalog(ctx, m)

	for name, revision := range m {
		if revision != "" {
			services = append(services, Service{name, revision})
		}
	}

	return
}

func (r *Registry) catalog(ctx context.Context, m map[string]string) {
	if r.parent != nil {
		r.parent.catalog(ctx, m)
	}

	for name, f := range r.factories {
		if f.Discoverable(ctx) {
			m[name] = f.ServiceRevision()
		} else {
			m[name] = ""
		}
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
func (r *Registry) StartServing(ctx context.Context, serviceConfig runtime.ServiceConfig, initial []snapshot.Service, send chan<- packet.Buf, recv <-chan packet.Buf,
) (runtime.ServiceDiscoverer, []runtime.ServiceState, error) {
	d := &discoverer{
		registry:   r,
		discovered: make(map[Factory]struct{}),
		stopped:    make(chan map[packet.Code]Instance, 1),
		services:   make([]discoveredService, len(initial)),
	}

	for i, s := range initial {
		d.services[i].Service = s

		if len(d.services[i].Buffer) == 0 {
			d.services[i].Buffer = nil
		}

		if f := d.registry.lookup(s.Name); f != nil {
			if _, dupe := d.discovered[f]; dupe {
				return nil, nil, runtime.ErrDuplicateService
			}

			d.discovered[f] = struct{}{}
			d.services[i].factory = f
		}
	}

	instances := make(map[packet.Code]Instance)
	states := make([]runtime.ServiceState, len(d.services))

	for i, s := range d.services {
		var code = packet.Code(i)
		var inst Instance
		var err error

		if s.factory != nil {
			instConfig := InstanceConfig{packet.Service{
				MaxSendSize: serviceConfig.MaxSendSize,
				Code:        code,
			}}
			inst, err = s.factory.RestoreInstance(ctx, instConfig, s.Buffer)
			if err != nil {
				return nil, nil, err
			}
			s.Buffer = nil
			states[i].SetAvail()
		}

		instances[code] = inst
	}

	go serve(ctx, serviceConfig, r, d, instances, send, recv)

	return d, states, nil
}

func serve(ctx context.Context, serviceConfig runtime.ServiceConfig, r *Registry, d *discoverer, instances map[packet.Code]Instance, send chan<- packet.Buf, recv <-chan packet.Buf) {
	defer func() {
		d.stopped <- instances
		close(d.stopped)
	}()

	for _, inst := range instances {
		if inst != nil {
			inst.Resume(ctx, send)
		}
	}

	for op := range recv {
		code := op.Code()

		inst, found := instances[code]
		if !found {
			if s := d.getServices()[code]; s.factory != nil {
				instConfig := InstanceConfig{packet.Service{
					MaxSendSize: serviceConfig.MaxSendSize,
					Code:        code,
				}}
				inst = s.factory.CreateInstance(ctx, instConfig)
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

type discoveredService struct {
	snapshot.Service
	factory Factory
}

type discoverer struct {
	registry   *Registry
	discovered map[Factory]struct{}
	stopped    chan map[packet.Code]Instance
	mu         sync.Mutex
	services   []discoveredService
}

func (d *discoverer) getServices() []discoveredService {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.services
}

func (d *discoverer) setServices(services []discoveredService) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.services = services
}

func (d *discoverer) Discover(ctx context.Context, newNames []string) (states []runtime.ServiceState, err error) {
	oldCount := len(d.services)
	newCount := oldCount + len(newNames)

	newServices := make([]discoveredService, oldCount, newCount)
	copy(newServices, d.services)

	for _, name := range newNames {
		s := discoveredService{
			Service: snapshot.Service{
				Name: name,
			},
		}

		if f := d.registry.lookup(name); f != nil && f.Discoverable(ctx) {
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

// Suspend instances.
func (d *discoverer) Suspend(ctx context.Context) (final []snapshot.Service) {
	final = make([]snapshot.Service, len(d.services))

	for i, s := range d.services {
		final[i] = s.Service
	}

	instances := <-d.stopped

	for code, inst := range instances {
		if inst != nil {
			if b := inst.Suspend(ctx); len(b) > 0 {
				final[code].Buffer = b
			}
		}
	}

	return
}

// Shutdown instances.
func (d *discoverer) Shutdown(ctx context.Context) {
	instances := <-d.stopped

	for _, inst := range instances {
		if inst != nil {
			inst.Shutdown(ctx)
		}
	}
}
