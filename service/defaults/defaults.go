// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package defaults defines the list of built-in services.
package defaults

import (
	"github.com/tsavola/gate/service"
	"github.com/tsavola/gate/service/echo"
	"github.com/tsavola/gate/service/origin"
	"github.com/tsavola/gate/service/peer"
)

// Register the built-in services.  The registry may be nil.
func Register(r *service.Registry) *service.Registry {
	if r == nil {
		r = new(service.Registry)
	}

	echo.Default.Register(r)
	origin.Default.Register(r)
	peer.Default.Register(r)

	return r
}
