// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"
	"fmt"

	internal "github.com/tsavola/gate/internal/server"
	"github.com/tsavola/gate/server/detail"
)

const (
	DefaultMaxProgramSize  = internal.DefaultMaxProgramSize
	DefaultMemorySizeLimit = internal.DefaultMemorySizeLimit
	DefaultStackSize       = internal.DefaultStackSize
	DefaultPreforkProcs    = internal.DefaultPreforkProcs
)

type Origin = internal.Origin
type Server = internal.Server
type Event = internal.Event
type Monitor = internal.Monitor
type Config = internal.Config

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
		panic(fmt.Errorf("Iface %d (%s) already exists with different name: %s", value, name, n))
	}
	if v, found := detail.Iface_value[name]; found && v != value {
		panic(fmt.Errorf("Iface %s (%d) already exists with different value: %d", name, value, v))
	}
	detail.Iface_name[value] = name
	detail.Iface_value[name] = value
}

func WithIface(ctx context.Context, value detail.Iface) context.Context {
	return internal.WithIface(ctx, value)
}

func WithClient(ctx context.Context, value string) context.Context {
	return internal.WithClient(ctx, value)
}

func WithCall(ctx context.Context, value string) context.Context {
	return internal.WithCall(ctx, value)
}

func Context(ctx context.Context) detail.Context {
	return internal.Context(ctx)
}

type State struct {
	Internal internal.State
}

// NewState retains significant resources until the context is canceled.
func NewState(ctx context.Context, config *Config) *State {
	s := new(State)
	s.Internal.Init(ctx, *config)
	return s
}
