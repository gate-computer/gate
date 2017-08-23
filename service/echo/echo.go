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

var prevId uint64 // atomic

type Config struct {
	Log Logger
}

var Default = new(Config)

func (c *Config) Register(r *service.Registry) {
	r.Register(Name, Version, c)
}

func (c *Config) Instantiate(packet.Code, *service.Config) service.Instance {
	return &echo{
		Config: *c,
		id:     atomic.AddUint64(&prevId, 1),
	}
}

type echo struct {
	Config
	id uint64
}

func (e *echo) Handle(p packet.Buf, replies chan<- packet.Buf) {
	replies <- p

	if e.Log != nil {
		e.Log.Printf("instance %d: %#v", e.id, string(p.Content()))
	}
}

func (e *echo) Shutdown() {
	if e.Log != nil {
		e.Log.Printf("instance %d: shutdown", e.id)
	}
}
