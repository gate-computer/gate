// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"io"

	"gate.computer/gate/image"
	"gate.computer/gate/runtime"
	"gate.computer/gate/server/api"
	"gate.computer/gate/server/event"
	"gate.computer/gate/server/model"
	"gate.computer/gate/source"
	"gate.computer/gate/trace"
	"gate.computer/internal/serverapi"

	. "import.name/type/context"
)

type InstanceConnector interface {
	// Connect allocates a new I/O stream.  The returned function is to be used
	// to transfer data between a network connection and the instance.  If it's
	// non-nil, a connection was established.
	Connect(Context) func(Context, io.Reader, io.WriteCloser) error

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

type Config struct {
	UUID           string
	ImageStorage   image.Storage
	Inventory      model.Inventory
	ProcessFactory runtime.ProcessFactory
	AccessPolicy   Authorizer
	ModuleSources  map[string]source.Source
	SourceCache    model.SourceCache
	OpenDebugLog   func(string) io.WriteCloser

	// StartSpan within trace context, ending when endSpan is called.  Nil
	// links must be ignored.  [trace.ContextAutoLinks] must also be respected.
	StartSpan func(_ Context, op api.Op, links ...*trace.Link) (_ Context, endSpan func(Context))

	// AddEvent to the current trace span, or outside of trace but in relation
	// to [trace.ContextAutoLinks].
	AddEvent func(Context, *event.Event, error)
}

func (c *Config) Configured() bool {
	return c.UUID != "" && c.Inventory != nil && c.ProcessFactory != nil && c.AccessPolicy != nil
}

func (c *Config) openDebugLog(opt *api.InvokeOptions) io.WriteCloser {
	if c.OpenDebugLog != nil && opt != nil && opt.DebugLog != "" {
		return c.OpenDebugLog(opt.DebugLog)
	}
	return nil
}

func (c *Config) startOp(ctx Context, op api.Op, links ...*trace.Link) (Context, func(Context)) {
	ctx = serverapi.ContextWithOp(ctx, op)
	return c.StartSpan(ctx, op, links...)
}

func (c *Config) event(ctx Context, t event.Type) {
	c.AddEvent(ctx, &event.Event{
		Type: t,
	}, nil)
}

func (c *Config) eventFail(ctx Context, t event.Type, info *event.Fail, err error) {
	c.AddEvent(ctx, &event.Event{
		Type: t,
		Info: &event.EventFail{Fail: info},
	}, err)
}

func (c *Config) eventModule(ctx Context, t event.Type, info *event.Module) {
	c.AddEvent(ctx, &event.Event{
		Type: t,
		Info: &event.EventModule{Module: info},
	}, nil)
}

func (c *Config) eventInstance(ctx Context, t event.Type, info *event.Instance, err error) {
	c.AddEvent(ctx, &event.Event{
		Type: t,
		Info: &event.EventInstance{Instance: info},
	}, err)
}
