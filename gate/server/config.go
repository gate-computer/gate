// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"
	"io"

	"gate.computer/gate/image"
	"gate.computer/gate/runtime"
	"gate.computer/gate/server/api"
	"gate.computer/gate/server/event"
)

type InstanceConnector interface {
	// Connect allocates a new I/O stream.  The returned function is to be used
	// to transfer data between a network connection and the instance.  If it's
	// non-nil, a connection was established.
	Connect(context.Context) func(context.Context, io.Reader, io.WriteCloser) error

	// Close causes currently blocked and future Connect calls to return nil.
	// Established connections will not be closed.
	Close() error
}

type InstanceServices interface {
	InstanceConnector
	runtime.ServiceRegistry
}

func NewInstanceServices(c InstanceConnector, r runtime.ServiceRegistry) InstanceServices {
	return &struct {
		InstanceConnector
		runtime.ServiceRegistry
	}{c, r}
}

type Inventory interface {
	GetSourceModule(ctx context.Context, source string) (module string, err error)
	AddModuleSource(ctx context.Context, module, source string) error
}

type Config struct {
	ImageStorage   image.Storage
	Inventory      Inventory
	ProcessFactory runtime.ProcessFactory
	AccessPolicy   Authorizer
	ModuleSources  map[string]Source
	Monitor        func(*event.Event, error)
	OpenDebugLog   func(string) io.WriteCloser
}

func (c *Config) Configured() bool {
	return c.ProcessFactory != nil && c.AccessPolicy != nil
}

func (c *Config) monitor(ctx context.Context, t event.Type) {
	c.Monitor(&event.Event{
		Type: t,
		Meta: api.ContextMeta(ctx),
	}, nil)
}

func (c *Config) monitorFail(ctx context.Context, t event.Type, info *event.Fail, err error) {
	c.Monitor(&event.Event{
		Type: t,
		Meta: api.ContextMeta(ctx),
		Info: &event.EventFail{Fail: info},
	}, err)
}

func (c *Config) monitorModule(ctx context.Context, t event.Type, info *event.Module) {
	c.Monitor(&event.Event{
		Type: t,
		Meta: api.ContextMeta(ctx),
		Info: &event.EventModule{Module: info},
	}, nil)
}

func (c *Config) monitorInstance(ctx context.Context, t event.Type, info *event.Instance) {
	c.Monitor(&event.Event{
		Type: t,
		Meta: api.ContextMeta(ctx),
		Info: &event.EventInstance{Instance: info},
	}, nil)
}

func (c *Config) openDebugLog(opt *api.InvokeOptions) io.WriteCloser {
	if c.OpenDebugLog != nil && opt != nil && opt.DebugLog != "" {
		return c.OpenDebugLog(opt.DebugLog)
	}
	return nil
}
