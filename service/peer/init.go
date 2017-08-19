// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package peer

import (
	"github.com/tsavola/gate/service"
)

const (
	Name    = "peer"
	Version = 0
)

var Default = new(Group)

func Register(r *service.Registry) {
	service.Register(r, Name, Version, Default)
}
