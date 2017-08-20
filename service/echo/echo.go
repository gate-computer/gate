// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package echo

import (
	"sync/atomic"

	"github.com/tsavola/gate/packet"
	"github.com/tsavola/gate/service"
)

const (
	Name    = "echo"
	Version = 0
)

type Logger interface {
	Printf(string, ...interface{})
}

var prevId uint64 // atomic

type Factory struct {
	Log Logger
}

func (f *Factory) New(packet.Code, *service.Config) service.Instance {
	return &echo{
		id:  atomic.AddUint64(&prevId, 1),
		log: f.Log,
	}
}

var Default = new(Factory)

func Register(r *service.Registry) {
	service.Register(r, Name, Version, Default)
}

type echo struct {
	id  uint64
	log Logger
}

func (e *echo) Handle(p packet.Buf, replies chan<- packet.Buf) {
	replies <- p

	if e.log != nil {
		e.log.Printf("instance %d: %#v", e.id, string(p.Content()))
	}
}

func (e *echo) Shutdown() {
	if e.log != nil {
		e.log.Printf("instance %d: shutdown", e.id)
	}
}
