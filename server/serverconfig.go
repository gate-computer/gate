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
	"github.com/tsavola/gate/runtime"
	"github.com/tsavola/gate/server/detail"
)

const DefaultPreforkProcs = 1

type InstanceConnector interface {
	// Connect allocates a new I/O stream.  The returned function is used to
	// drive I/O between network connection and instance.  If it's non-nil, a
	// connection was made.
	Connect(context.Context) func(context.Context, io.Reader, io.Writer) error

	// Close disconnects remaining connections.  Currently blocked and future
	// Connect calls will return nil.
	Close() error
}

type InstanceServices interface {
	runtime.ServiceRegistry
	InstanceConnector
}

type instanceServices struct {
	runtime.ServiceRegistry
	InstanceConnector
}

func NewInstanceServices(r runtime.ServiceRegistry, c InstanceConnector) InstanceServices {
	return &instanceServices{r, c}
}

type Event interface {
	EventName() string
	EventType() int32
	proto.Message
}

type Config struct {
	ProgramStorage  image.ProgramStorage
	InstanceStorage image.InstanceStorage
	Executor        *runtime.Executor
	AccessPolicy    Authorizer
	PreforkProcs    int
	Monitor         func(Event, error)
}

func (c *Config) Configured() bool {
	return c.Executor != nil && c.AccessPolicy != nil
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
