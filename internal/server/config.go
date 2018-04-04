// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"io"

	"github.com/tsavola/gate/run"
	"github.com/tsavola/gate/server/detail"
	"github.com/tsavola/wag/wasm"
)

const (
	DefaultMemorySizeLimit = 16777216
	DefaultStackSize       = 65536
	DefaultPreforkProcs    = 1
)

type Origin struct {
	R io.Reader
	W io.Writer
}

type Server struct {
	Origin Origin
}

type Event interface {
	EventName() string
	EventType() int32
	ProtoMessage()
	Reset()
	String() string
}

type Monitor struct {
	MonitorError func(*detail.Position, error)
	MonitorEvent func(Event, error)
}

type Config struct {
	Runtime  *run.Runtime
	Services func(*Server) run.ServiceRegistry
	Monitor
	Debug io.Writer

	MemorySizeLimit wasm.MemorySize
	StackSize       int32
	PreforkProcs    int
}
