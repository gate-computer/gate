// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"
	"io"

	"gate.computer/gate/image"
	"gate.computer/gate/runtime"
	"google.golang.org/protobuf/proto"
)

type InstanceConnector interface {
	// Connect allocates a new I/O stream.  The returned function is to be used
	// to transfer data between a network connection and the instance.  If it's
	// non-nil, a connection was established.
	Connect(context.Context) func(context.Context, io.Reader, io.Writer) error

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

type Event interface {
	EventName() string
	EventType() int32
	proto.Message
}

type Config struct {
	ImageStorage   image.Storage
	ProcessFactory runtime.ProcessFactory
	AccessPolicy   Authorizer
	Monitor        func(Event, error)
}

func (c *Config) Configured() bool {
	return c.ProcessFactory != nil && c.AccessPolicy != nil
}

func (c *Config) monitor(e Event) {
	c.Monitor(e, nil)
}
