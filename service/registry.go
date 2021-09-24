// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"unicode"

	"gate.computer/gate/packet"
	"gate.computer/gate/runtime"
	"gate.computer/gate/snapshot"
	"github.com/tsavola/mu"
)

const maxServiceStringLen = 127

type existenceError string

func (e existenceError) Error() string { return string(e) }
func (e existenceError) Unwrap() error { return os.ErrExist }

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
	Ready(ctx context.Context) error
	Start(ctx context.Context, send chan<- packet.Buf, abort func(error)) error
	Handle(ctx context.Context, send chan<- packet.Buf, received packet.Buf) error
	Shutdown(ctx context.Context) error
	Suspend(ctx context.Context) (snapshot []byte, err error)
}

// InstanceBase provides default implementations for some Instance methods.
type InstanceBase struct{}

func (InstanceBase) Ready(context.Context) error                                 { return nil }
func (InstanceBase) Start(context.Context, chan<- packet.Buf, func(error)) error { return nil }
func (InstanceBase) Shutdown(context.Context) error                              { return nil }

// Factory creates instances of a particular service implementation.
//
// See https://github.com/gate-computer/gate/blob/master/Service.md for service
// naming conventions.
type Factory interface {
	Service() Service
	Discoverable(ctx context.Context) bool
	CreateInstance(ctx context.Context, config InstanceConfig, snapshot []byte) (Instance, error)
}

// Registry is a runtime.ServiceRegistry implementation.  It multiplexes
// packets to service implementations.
type Registry struct {
	factories map[string]Factory
	parent    *Registry
}

// Register a service implementation.  Doesn't replace an existing service.
func (r *Registry) Register(f Factory) error {
	return r.register(f, false)
}

// MustRegister a service implementation.  May replace an existing service.
// Panicks if service information is invalid.
func (r *Registry) MustRegister(f Factory) {
	if err := r.register(f, true); err != nil {
		panic(err)
	}
}

func (r *Registry) register(f Factory, replace bool) error {
	service := f.Service()

	if err := checkString("name", service.Name, unicode.IsLetter, unicode.IsNumber, unicode.IsPunct); err != nil {
		return err
	}

	if err := checkString("revision", service.Revision, unicode.IsLetter, unicode.IsMark, unicode.IsNumber, unicode.IsPunct, unicode.IsSymbol); err != nil {
		return err
	}

	if r.factories == nil {
		r.factories = make(map[string]Factory)
	}
	if !replace {
		if _, found := r.factories[service.Name]; found {
			return existenceError(fmt.Sprintf("service %q already registered", service.Name))
		}
	}
	r.factories[service.Name] = f
	return nil
}

func checkString(what, s string, categories ...func(rune) bool) error {
	if len(s) == 0 {
		return fmt.Errorf("service %s string is empty", what)
	}
	if len(s) > maxServiceStringLen {
		return fmt.Errorf("service %s string is too long", what)
	}

	for _, r := range s {
		for _, f := range categories {
			if f(r) {
				goto ok
			}
		}
		return fmt.Errorf("service %s string contains invalid characters", what)
	ok:
	}

	return nil
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
			m[name] = f.Service().Revision
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
func (r *Registry) StartServing(ctx context.Context, serviceConfig runtime.ServiceConfig, initial []snapshot.Service, send chan<- packet.Buf, recv <-chan packet.Buf) (runtime.ServiceDiscoverer, []runtime.ServiceState, <-chan error, error) {
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
				return nil, nil, nil, runtime.ErrDuplicateService
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

		if s.factory != nil {
			instConfig := InstanceConfig{packet.Service{
				MaxSendSize: serviceConfig.MaxSendSize,
				Code:        code,
			}}

			var err error
			inst, err = s.factory.CreateInstance(ctx, instConfig, s.Buffer)
			if err != nil {
				return nil, nil, nil, err
			}

			s.Buffer = nil
			states[i].SetAvail()
		}

		instances[code] = inst
	}

	for _, inst := range instances {
		if inst != nil {
			if err := inst.Ready(ctx); err != nil {
				return nil, nil, nil, err
			}
		}
	}

	done := make(chan error, 1)

	go func() {
		err := errors.New("service panicked")

		a := aborter{c: done}
		defer func() {
			a.close(err)
		}()

		err = serve(ctx, serviceConfig, r, d, instances, send, recv, a.abort)
	}()

	return d, states, done, nil
}

type aborter struct {
	mu mu.Mutex
	c  chan<- error
}

func (a *aborter) abort(err error) {
	if err == nil {
		err = errors.New("service aborted with unspecified error")
	}
	a.close(err)
}

func (a *aborter) close(err error) {
	var c chan<- error
	a.mu.Guard(func() {
		c = a.c
		a.c = nil
	})
	if c == nil {
		return
	}

	if err != nil {
		c <- err
	}
	close(c)
}

func serve(outerCtx context.Context, serviceConfig runtime.ServiceConfig, r *Registry, d *discoverer, instances map[packet.Code]Instance, send chan<- packet.Buf, recv <-chan packet.Buf, abort func(error)) error {
	defer func() {
		d.stopped <- instances
		close(d.stopped)
	}()

	innerCtx, cancel := context.WithCancel(outerCtx)
	defer cancel()

	for _, inst := range instances {
		if inst != nil {
			if err := inst.Start(innerCtx, send, abort); err != nil {
				return err
			}
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

				var err error
				inst, err = s.factory.CreateInstance(outerCtx, instConfig, nil)
				if err != nil {
					return err
				}
			}

			instances[code] = inst

			if inst != nil {
				if err := inst.Ready(outerCtx); err != nil {
					return err
				}
				if err := inst.Start(innerCtx, send, abort); err != nil {
					return err
				}
			}
		}

		if inst != nil {
			if err := inst.Handle(innerCtx, send, op); err != nil {
				return err
			}
		} else {
			// TODO: service unavailable: buffer up to max packet size
		}
	}

	return nil
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

// Shutdown instances.
func (d *discoverer) Shutdown(ctx context.Context) (err error) {
	instances := <-d.stopped

	for _, inst := range instances {
		if inst != nil {
			if e := inst.Shutdown(ctx); err == nil {
				err = e
			}
		}
	}

	return
}

// Suspend instances.
func (d *discoverer) Suspend(ctx context.Context) (final []snapshot.Service, err error) {
	final = make([]snapshot.Service, len(d.services))

	for i, s := range d.services {
		final[i] = s.Service
	}

	instances := <-d.stopped

	for code, inst := range instances {
		if inst != nil {
			b, e := inst.Suspend(ctx)
			if len(b) > 0 {
				final[code].Buffer = b
			}
			if err == nil {
				err = e
			}
		}
	}

	return
}
