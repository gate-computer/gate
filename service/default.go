// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"github.com/tsavola/gate/packet"
)

// Defaults gets populated with the built-in services if the service/defaults
// package is imported.
var Defaults = new(Registry)

// Register registers a default service if r is nil.
func Register(r *Registry, name string, version int32, f Factory) {
	if r == nil {
		r = Defaults
	}
	r.Register(name, version, f)
}

// RegisterFunc is almost like Register.
func RegisterFunc(r *Registry, name string, version int32, f func(packet.Code, *Config) Instance) {
	Register(r, name, version, FactoryFunc(f))
}

// Clone the default registry if r is nil.
func Clone(r *Registry) *Registry {
	if r == nil {
		r = Defaults
	}
	return r.Clone()
}
