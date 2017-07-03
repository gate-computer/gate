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
