// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"
	"fmt"
	"io"

	"github.com/gogo/protobuf/proto"
	"github.com/tsavola/gate/image"
	"github.com/tsavola/gate/principal"
	"github.com/tsavola/gate/runtime"
	"github.com/tsavola/gate/server/detail"
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

	// TODO: remove this after there is some kind of ownership database
	XXX_Owner *principal.ID
}

func (c *Config) Configured() bool {
	return c.ProcessFactory != nil && c.AccessPolicy != nil
}

func (c *Config) monitor(e Event) {
	c.Monitor(e, nil)
}

func AllocateIface(name string) detail.Iface {
	value, found := detail.Iface_value[name]
	if !found {
		value = int32(len(detail.Iface_name))
		detail.Iface_name[value] = name
		detail.Iface_value[name] = value
	}
	return detail.Iface(value)
}

func RegisterIface(value int32, name string) {
	if n, found := detail.Iface_name[value]; found && n != name {
		panic(fmt.Errorf("iface %d (%s) already exists with different name: %s", value, name, n))
	}
	if v, found := detail.Iface_value[name]; found && v != value {
		panic(fmt.Errorf("iface %s (%d) already exists with different value: %d", name, value, v))
	}
	detail.Iface_name[value] = name
	detail.Iface_value[name] = value
}
