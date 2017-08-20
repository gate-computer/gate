// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package peer

import (
	"sync"
	"sync/atomic"

	"github.com/tsavola/gate/packet"
	"github.com/tsavola/gate/service"
)

var (
	lastGroupId uint64 // atomic
)

type Group struct {
	Log Logger

	lock  sync.Mutex
	peers map[uint64]*peer
}

func (g *Group) New(code packet.Code, config *service.Config) service.Instance {
	return &peer{
		group: g,
		code:  code,
		id:    atomic.AddUint64(&lastGroupId, 1),
	}
}
