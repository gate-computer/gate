package peer

import (
	"sync"
	"sync/atomic"

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

func (g *Group) New() service.Instance {
	return &peer{
		group: g,
		id:    atomic.AddUint64(&lastGroupId, 1),
	}
}
