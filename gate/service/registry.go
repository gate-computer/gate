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
	"import.name/lock"
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

// Properties of a service implementation.
type Properties struct {
	Service
	Streams bool // Should Instance.Handle() receive flow and data packets?
}

// InstanceConfig for a service instance.
type InstanceConfig struct {
	packet.Service
}

// Instance of a service.  Corresponds to a program instance.
type Instance interface {
	Ready(ctx context.Context) error
	Start(ctx context.Context, send chan<- packet.Thunk, abort func(error)) error
	Handle(ctx context.Context, send chan<- packet.Thunk, received packet.Buf) (packet.Buf, error)
	Shutdown(ctx context.Context, suspend bool) (snapshot []byte, err error)
}

// InstanceBase provides default implementations for some Instance methods.
type InstanceBase struct{}

func (InstanceBase) Ready(context.Context) error                                   { return nil }
func (InstanceBase) Start(context.Context, chan<- packet.Thunk, func(error)) error { return nil }
func (InstanceBase) Shutdown(context.Context, bool) ([]byte, error)                { return nil, nil }

// Factory creates instances of a particular service implementation.
//
// See https://github.com/gate-computer/gate/blob/master/Service.md for service
// naming conventions.
type Factory interface {
	Properties() Properties
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
	service := f.Properties().Service

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
			m[name] = f.Properties().Service.Revision
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

func (r *Registry) CreateServer(ctx context.Context, serviceConfig runtime.ServiceConfig, initial []snapshot.Service, send chan<- packet.Thunk) (runtime.InstanceServer, []runtime.ServiceState, <-chan error, error) {
	done := make(chan error, 1)
	d := &discoverer{
		registry:  r,
		config:    serviceConfig,
		aborter:   aborter{c: done},
		factories: make(map[Factory]struct{}),
		services:  make([]serverService, len(initial)),
	}

	for i, s := range initial {
		d.services[i].Service = s

		if len(d.services[i].Buffer) == 0 {
			d.services[i].Buffer = nil
		}

		if f := d.registry.lookup(s.Name); f != nil {
			if _, dupe := d.factories[f]; dupe {
				return nil, nil, nil, runtime.ErrDuplicateService
			}

			d.factories[f] = struct{}{}
			d.services[i].factory = f
		}
	}

	states := make([]runtime.ServiceState, len(d.services))

	for i, s := range d.services {
		if s.factory != nil {
			instConfig := InstanceConfig{packet.Service{
				MaxSendSize: serviceConfig.MaxSendSize,
				Code:        packet.Code(i),
			}}

			x, err := s.factory.CreateInstance(ctx, instConfig, s.Buffer)
			if err != nil {
				return nil, nil, nil, err
			}

			s.Buffer = nil
			s.instance = x
			states[i].SetAvail()
		}
	}

	for _, s := range d.services {
		if s.instance != nil {
			if err := s.instance.Ready(ctx); err != nil {
				return nil, nil, nil, err
			}
		}
	}

	return d, states, done, nil
}

type aborter struct {
	mu sync.Mutex
	c  chan<- error
}

func (a *aborter) abort(err error) {
	if err == nil {
		err = errors.New("service aborted with unspecified error")
	}

	var c chan<- error
	lock.Guard(&a.mu, func() {
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

type serverService struct {
	snapshot.Service
	factory   Factory
	instance  Instance
	maxDomain packet.Domain
}

type discoverer struct {
	registry *Registry
	config   runtime.ServiceConfig
	aborter
	factories map[Factory]struct{}
	services  []serverService
}

func (d *discoverer) Start(ctx context.Context, send chan<- packet.Thunk) error {
	for _, s := range d.services {
		if s.instance != nil {
			if err := s.instance.Start(ctx, send, d.abort); err != nil {
				return err
			}
		}
	}

	return nil
}

func (d *discoverer) Discover(ctx context.Context, newNames []string) (states []runtime.ServiceState, err error) {
	for _, name := range newNames {
		s := serverService{
			Service: snapshot.Service{
				Name: name,
			},
		}

		if f := d.registry.lookup(name); f != nil && f.Discoverable(ctx) {
			if _, dupe := d.factories[f]; dupe {
				err = runtime.ErrDuplicateService
				return
			}

			d.factories[f] = struct{}{}
			s.factory = f
		}

		d.services = append(d.services, s)
	}

	states = make([]runtime.ServiceState, len(d.services))
	for i, s := range d.services {
		if s.factory != nil {
			states[i].SetAvail()
		}
	}
	return
}

func (d *discoverer) Handle(ctx context.Context, send chan<- packet.Thunk, p packet.Buf) (packet.Buf, error) {
	code := p.Code()
	s := d.services[code]

	if s.instance == nil {
		if s.factory == nil {
			// TODO: service unavailable: buffer up to max packet size
			return nil, nil
		}

		instConfig := InstanceConfig{packet.Service{
			MaxSendSize: d.config.MaxSendSize,
			Code:        code,
		}}

		inst, err := s.factory.CreateInstance(ctx, instConfig, nil)
		if err != nil {
			return nil, err
		}

		s.instance = inst
		s.maxDomain = packet.DomainInfo
		if s.factory.Properties().Streams {
			s.maxDomain = packet.DomainData
		}
		d.services[code] = s

		if err := s.instance.Ready(ctx); err != nil {
			return nil, err
		}

		if err := s.instance.Start(ctx, send, d.abort); err != nil {
			return nil, err
		}
	}

	if dom := p.Domain(); dom > s.maxDomain {
		return nil, fmt.Errorf("%s received packet with unexpected domain: %s", code, dom)
	}

	return s.instance.Handle(ctx, send, p)
}

// Shutdown instances.
func (d *discoverer) Shutdown(ctx context.Context, suspend bool) ([]snapshot.Service, error) {
	if suspend {
		return d.suspend(ctx)
	}
	return nil, d.shutdown(ctx)
}

func (d *discoverer) shutdown(ctx context.Context) (err error) {
	for _, s := range d.services {
		if s.instance != nil {
			if _, e := s.instance.Shutdown(ctx, false); err == nil {
				err = e
			}
		}
	}

	return
}

func (d *discoverer) suspend(ctx context.Context) (final []snapshot.Service, err error) {
	final = make([]snapshot.Service, len(d.services))

	for code, s := range d.services {
		final[code] = s.Service

		if s.instance != nil {
			b, e := s.instance.Shutdown(ctx, true)
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
